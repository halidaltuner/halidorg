// Package radar is the tech radar (CONTENT_MODEL.md §2 radar_item, Phase 6).
//
// The admin edits a working copy (radar_items), one row per item with both
// languages' notes. Nothing in it is public: freezing an edition copies
// every item on the radar into radar_history and publishes the item's two
// entries (kind radar_item), whose title is the name and summary the note.
// The public radar shows frozen editions only; an item is new in an edition
// it was not on before, and moved when its ring differs from the previous
// edition's.
package radar

import (
	"context"
	"errors"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"halid.org/engine/internal/content"
	"halid.org/engine/internal/i18n"
	"halid.org/engine/internal/store"
)

// Kind is the entries.kind value of an item's entries.
const Kind = "radar_item"

// Quadrants in drawing order: languages top right, techniques top left,
// tools bottom left, platforms bottom right (design/Radar.dc.html).
var Quadrants = []string{"languages", "techniques", "tools", "platforms"}

// Rings from the center out.
var Rings = []string{"adopt", "trial", "assess", "hold"}

// RingIndex is the ring's position from the center, -1 when unknown.
func RingIndex(ring string) int { return index(Rings, ring) }

// QuadrantIndex is the quadrant's drawing position, -1 when unknown.
func QuadrantIndex(q string) int { return index(Quadrants, q) }

func index(list []string, v string) int {
	for i, x := range list {
		if x == v {
			return i
		}
	}
	return -1
}

// Item is one row of the working copy.
type Item struct {
	Group     string `json:"group_id"`
	Slug      string `json:"slug"`
	Name      string `json:"name"`
	Quadrant  string `json:"quadrant"`
	Ring      string `json:"ring"`
	NoteTR    string `json:"note_tr"`
	NoteEN    string `json:"note_en"`
	Off       bool   `json:"-"`
	OffInt    int    `json:"off"`
	CreatedAt int64  `json:"created_at"`
	UpdatedAt int64  `json:"updated_at"`
}

// Note is the note in lang.
func (i Item) Note(lang string) string {
	if lang == "tr" {
		return i.NoteTR
	}
	return i.NoteEN
}

// Edition is a frozen radar.
type Edition struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	PublishedAt int64  `json:"published_at"`
}

// Blip is an item as an edition (or the working copy) shows it, compared
// with the edition before: Prev is its ring there, "" when it was not on it.
type Blip struct {
	Group    string `json:"group_id"`
	Slug     string `json:"slug"`
	Name     string `json:"name"`
	Quadrant string `json:"quadrant"`
	Ring     string `json:"ring"`
	NoteTR   string `json:"note_tr"`
	NoteEN   string `json:"note_en"`
	Prev     string `json:"-"`
	New      bool   `json:"-"`
	Moved    bool   `json:"-"`
}

// Note is the note in lang.
func (b Blip) Note(lang string) string {
	if lang == "tr" {
		return b.NoteTR
	}
	return b.NoteEN
}

// Compare marks each blip of cur against prev, the edition before it: new
// when it was not on prev (and prev exists: the first edition has nothing
// new), moved when its ring differs. dropped lists the blips of prev that
// cur leaves out. cur keeps its order.
func Compare(prev, cur []Blip, first bool) (out, dropped []Blip) {
	was := map[string]Blip{}
	for _, b := range prev {
		was[b.Group] = b
	}
	on := map[string]bool{}
	out = make([]Blip, len(cur))
	for i, b := range cur {
		on[b.Group] = true
		b.Prev, b.New, b.Moved = "", false, false
		if p, ok := was[b.Group]; ok {
			b.Prev = p.Ring
			b.Moved = p.Ring != b.Ring
		} else {
			b.New = !first
		}
		out[i] = b
	}
	for _, b := range prev {
		if !on[b.Group] {
			dropped = append(dropped, b)
		}
	}
	return out, dropped
}

// Sort orders blips as the radar lists them: by ring, then quadrant, then
// name.
func Sort(bs []Blip) {
	sort.SliceStable(bs, func(i, j int) bool {
		a, b := bs[i], bs[j]
		if ra, rb := RingIndex(a.Ring), RingIndex(b.Ring); ra != rb {
			return ra < rb
		}
		if qa, qb := QuadrantIndex(a.Quadrant), QuadrantIndex(b.Quadrant); qa != qb {
			return qa < qb
		}
		return strings.ToLower(a.Name) < strings.ToLower(b.Name)
	})
}

var labelRE = regexp.MustCompile(`^[0-9]{4}-(0[1-9]|1[0-2])$`)

// ValidLabel reports whether s is an edition label (2026-10).
func ValidLabel(s string) bool { return labelRE.MatchString(s) }

// Validate checks an item.
func Validate(i Item) error {
	v := content.ValidationError{}
	if n := utf8.RuneCountInString(strings.TrimSpace(i.Name)); n == 0 || n > 60 {
		v["name"] = "1 to 60 characters"
	}
	if QuadrantIndex(i.Quadrant) < 0 {
		v["quadrant"] = "unknown quadrant"
	}
	if RingIndex(i.Ring) < 0 {
		v["ring"] = "unknown ring"
	}
	// The notes become the entries' summaries (the meta description).
	if utf8.RuneCountInString(i.NoteTR) > 160 {
		v["note_tr"] = "at most 160 characters"
	}
	if utf8.RuneCountInString(i.NoteEN) > 160 {
		v["note_en"] = "at most 160 characters"
	}
	if len(v) == 0 {
		return nil
	}
	return v
}

// Repo reads and writes the radar.
type Repo struct{ db store.DB }

// NewRepo returns a repository over db.
func NewRepo(db store.DB) *Repo { return &Repo{db: db} }

const itemCols = `group_id, slug, name, quadrant, ring, note_tr, note_en, off, created_at, updated_at`

func scanItems(res store.Result, err error) ([]Item, error) {
	if err != nil {
		return nil, err
	}
	rows, err := store.Scan[Item](res)
	for i := range rows {
		rows[i].Off = rows[i].OffInt != 0
	}
	return rows, err
}

// Items lists the working copy by ring, quadrant and name.
func (r *Repo) Items(ctx context.Context) ([]Item, error) {
	items, err := scanItems(r.db.Query(ctx, `SELECT `+itemCols+` FROM radar_items`))
	if err != nil {
		return nil, err
	}
	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i], items[j]
		if a.Off != b.Off {
			return !a.Off
		}
		if ra, rb := RingIndex(a.Ring), RingIndex(b.Ring); ra != rb {
			return ra < rb
		}
		if qa, qb := QuadrantIndex(a.Quadrant), QuadrantIndex(b.Quadrant); qa != qb {
			return qa < qb
		}
		return strings.ToLower(a.Name) < strings.ToLower(b.Name)
	})
	return items, nil
}

// Get loads one item.
func (r *Repo) Get(ctx context.Context, group string) (Item, error) {
	items, err := scanItems(r.db.Query(ctx, `SELECT `+itemCols+` FROM radar_items WHERE group_id = ?`, group))
	if err != nil {
		return Item{}, err
	}
	if len(items) == 0 {
		return Item{}, content.NotFound(Kind, group)
	}
	return items[0], nil
}

func normalize(i *Item) {
	i.Name = strings.TrimSpace(i.Name)
	i.NoteTR = strings.TrimSpace(i.NoteTR)
	i.NoteEN = strings.TrimSpace(i.NoteEN)
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// Create adds an item to the working copy. Its slug comes from the name and
// stays when the item is renamed.
func (r *Repo) Create(ctx context.Context, i Item) (Item, error) {
	normalize(&i)
	if err := Validate(i); err != nil {
		return Item{}, err
	}
	base := content.Slugify(i.Name)
	if base == "" || ValidLabel(base) {
		// Edition pages live at /{lang}/radar/{YYYY-MM}.
		base = strings.TrimSuffix("item-"+base, "-")
	}
	slug := base
	for n := 2; ; n++ {
		res, err := r.db.Query(ctx, `SELECT slug FROM radar_items WHERE slug = ?
UNION SELECT slug FROM entries WHERE kind = ? AND slug = ?`, slug, Kind, slug)
		if err != nil {
			return Item{}, err
		}
		if len(res.Rows) == 0 {
			break
		}
		slug = base + "-" + strconv.Itoa(n)
		if n > 50 {
			return Item{}, errors.New("radar: no free slug for " + base)
		}
	}
	now := content.Clock().Unix()
	i.Group, i.Slug, i.CreatedAt, i.UpdatedAt = content.NewID(), slug, now, now
	_, err := r.db.Query(ctx, `INSERT INTO radar_items (`+itemCols+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		i.Group, i.Slug, i.Name, i.Quadrant, i.Ring, i.NoteTR, i.NoteEN, boolInt(i.Off), now, now)
	return i, err
}

// Save writes an item's name, quadrant, ring, notes and whether it is off
// the next edition. Nothing public changes until the next freeze.
func (r *Repo) Save(ctx context.Context, i Item) (Item, error) {
	normalize(&i)
	if err := Validate(i); err != nil {
		return Item{}, err
	}
	res, err := r.db.Query(ctx, `UPDATE radar_items SET name = ?, quadrant = ?, ring = ?, note_tr = ?, note_en = ?, off = ?, updated_at = ?
WHERE group_id = ?`, i.Name, i.Quadrant, i.Ring, i.NoteTR, i.NoteEN, boolInt(i.Off), content.Clock().Unix(), i.Group)
	if err != nil {
		return Item{}, err
	}
	if res.Changes == 0 {
		return Item{}, content.NotFound(Kind, i.Group)
	}
	return r.Get(ctx, i.Group)
}

// EntryIDs are the ids of the item's published entries, by language.
func (r *Repo) EntryIDs(ctx context.Context, group string) (map[string]string, error) {
	res, err := r.db.Query(ctx, `SELECT id, lang FROM entries WHERE kind = ? AND translation_group_id = ?`, Kind, group)
	if err != nil {
		return nil, err
	}
	rows, err := store.Scan[struct {
		ID   string `json:"id"`
		Lang string `json:"lang"`
	}](res)
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, x := range rows {
		out[x.Lang] = x.ID
	}
	return out, nil
}

// Delete removes an item and its entries. Its rows in frozen editions stay:
// old editions keep showing it.
func (r *Repo) Delete(ctx context.Context, group string) error {
	res, err := r.db.Batch(ctx,
		store.Stmt{SQL: `DELETE FROM radar_items WHERE group_id = ?`, Args: []any{group}},
		store.Stmt{SQL: `DELETE FROM entries WHERE kind = ? AND translation_group_id = ?`, Args: []any{Kind, group}})
	if err != nil {
		return err
	}
	if len(res) == 0 || res[0].Changes == 0 {
		return content.NotFound(Kind, group)
	}
	return nil
}

// Editions lists the frozen editions, newest first.
func (r *Repo) Editions(ctx context.Context) ([]Edition, error) {
	res, err := r.db.Query(ctx, `SELECT id, label, published_at FROM radar_editions ORDER BY label DESC`)
	if err != nil {
		return nil, err
	}
	return store.Scan[Edition](res)
}

// Snapshot is what edition showed, sorted.
func (r *Repo) Snapshot(ctx context.Context, editionID string) ([]Blip, error) {
	res, err := r.db.Query(ctx, `SELECT group_id, slug, name, quadrant, ring, note_tr, note_en FROM radar_history WHERE edition_id = ?`, editionID)
	if err != nil {
		return nil, err
	}
	bs, err := store.Scan[Blip](res)
	Sort(bs)
	return bs, err
}

// View is one edition as the public radar shows it.
type View struct {
	Edition  Edition
	Editions []Edition // all, newest first
	Blips    []Blip
	Dropped  []Blip
}

// ErrNoEdition: nothing frozen yet, or no edition with that label.
var ErrNoEdition = errors.New("radar: no such edition")

// Edition loads the edition labelled label ("" is the latest), compared
// with the one before it.
func (r *Repo) Edition(ctx context.Context, label string) (View, error) {
	eds, err := r.Editions(ctx)
	if err != nil {
		return View{}, err
	}
	at := -1
	for i, e := range eds {
		if label == "" || e.Label == label {
			at = i
			break
		}
	}
	if at < 0 {
		return View{Editions: eds}, ErrNoEdition
	}
	cur, err := r.Snapshot(ctx, eds[at].ID)
	if err != nil {
		return View{}, err
	}
	var prev []Blip
	first := at == len(eds)-1
	if !first {
		if prev, err = r.Snapshot(ctx, eds[at+1].ID); err != nil {
			return View{}, err
		}
	}
	v := View{Edition: eds[at], Editions: eds}
	v.Blips, v.Dropped = Compare(prev, cur, first)
	return v, nil
}

// Pending compares the working copy with the latest edition: what freezing
// now would publish.
func (r *Repo) Pending(ctx context.Context) (View, error) {
	items, err := r.Items(ctx)
	if err != nil {
		return View{}, err
	}
	var cur []Blip
	for _, i := range items {
		if !i.Off {
			cur = append(cur, blipOf(i))
		}
	}
	Sort(cur)
	eds, err := r.Editions(ctx)
	if err != nil {
		return View{}, err
	}
	v := View{Editions: eds}
	var prev []Blip
	if len(eds) > 0 {
		if prev, err = r.Snapshot(ctx, eds[0].ID); err != nil {
			return View{}, err
		}
	}
	v.Blips, v.Dropped = Compare(prev, cur, len(eds) == 0)
	return v, nil
}

func blipOf(i Item) Blip {
	return Blip{Group: i.Group, Slug: i.Slug, Name: i.Name, Quadrant: i.Quadrant, Ring: i.Ring, NoteTR: i.NoteTR, NoteEN: i.NoteEN}
}

// History is an item's rows across editions, newest first.
type History struct {
	Label string `json:"label"`
	Ring  string `json:"ring"`
	Note  string `json:"-"`
	Prev  string `json:"-"` // the ring in the edition before, "" when not on it
	// Off is set on an edition the item is not on, after one it was on.
	Off    bool   `json:"-"`
	NoteTR string `json:"note_tr"`
	NoteEN string `json:"note_en"`
}

// HistoryOf lists every edition from the item's first, with its ring in
// each (Off where it was left out), newest first.
func (r *Repo) HistoryOf(ctx context.Context, group, lang string) ([]History, error) {
	eds, err := r.Editions(ctx)
	if err != nil {
		return nil, err
	}
	res, err := r.db.Query(ctx, `SELECT e.label, h.ring, h.note_tr, h.note_en FROM radar_history h
JOIN radar_editions e ON e.id = h.edition_id WHERE h.group_id = ?`, group)
	if err != nil {
		return nil, err
	}
	rows, err := store.Scan[History](res)
	if err != nil {
		return nil, err
	}
	by := map[string]History{}
	for _, h := range rows {
		by[h.Label] = h
	}
	var out []History
	prev := ""
	started := false
	// Oldest first, so each row knows the ring before it.
	for i := len(eds) - 1; i >= 0; i-- {
		h, ok := by[eds[i].Label]
		if !ok {
			if started {
				out = append(out, History{Label: eds[i].Label, Off: true, Prev: prev})
				prev = ""
			}
			continue
		}
		started = true
		h.Prev = prev
		h.Note = h.NoteEN
		if lang == "tr" {
			h.Note = h.NoteTR
		}
		out = append(out, h)
		prev = h.Ring
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, nil
}

// Frozen is what Freeze did: the edition and every radar entry it wrote,
// with the state before (nil for new entries), for the render jobs.
type Frozen struct {
	Edition Edition
	Entries []string
}

// Freeze snapshots every item on the radar into a new edition labelled
// label and publishes the items' entries (title the name, summary the note
// in that language). Labels must grow: 2026-10 after 2026-04.
func (r *Repo) Freeze(ctx context.Context, label string) (Frozen, error) {
	if !ValidLabel(label) {
		return Frozen{}, content.ValidationError{"label": "YYYY-MM"}
	}
	eds, err := r.Editions(ctx)
	if err != nil {
		return Frozen{}, err
	}
	if len(eds) > 0 && label <= eds[0].Label {
		return Frozen{}, content.ValidationError{"label": "after " + eds[0].Label}
	}
	items, err := r.Items(ctx)
	if err != nil {
		return Frozen{}, err
	}
	res, err := r.db.Query(ctx, `SELECT id, lang, translation_group_id, current_revision_id FROM entries WHERE kind = ?`, Kind)
	if err != nil {
		return Frozen{}, err
	}
	existing, err := store.Scan[struct {
		ID    string  `json:"id"`
		Lang  string  `json:"lang"`
		Group string  `json:"translation_group_id"`
		Rev   *string `json:"current_revision_id"`
	}](res)
	if err != nil {
		return Frozen{}, err
	}
	have := map[string]string{} // group/lang -> entry id
	out := Frozen{Edition: Edition{ID: content.NewID(), Label: label, PublishedAt: content.Clock().Unix()}}
	for _, e := range existing {
		have[e.Group+"/"+e.Lang] = e.ID
		out.Entries = append(out.Entries, e.ID)
	}
	now := out.Edition.PublishedAt
	stmts := []store.Stmt{{SQL: `INSERT INTO radar_editions (id, label, published_at) VALUES (?, ?, ?)`, Args: []any{out.Edition.ID, label, now}}}
	on := 0
	for _, it := range items {
		if it.Off {
			continue
		}
		on++
		stmts = append(stmts, store.Stmt{SQL: `INSERT INTO radar_history (group_id, edition_id, slug, name, quadrant, ring, note_tr, note_en, changed_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, Args: []any{it.Group, out.Edition.ID, it.Slug, it.Name, it.Quadrant, it.Ring, it.NoteTR, it.NoteEN, now}})
		for _, lang := range i18n.Supported {
			if id, ok := have[it.Group+"/"+lang]; ok {
				stmts = append(stmts, store.Stmt{SQL: `UPDATE entries SET slug = ?, title = ?, summary = ?, status = 'published', visibility = 'public',
  published_revision_id = COALESCE(published_revision_id, current_revision_id), published_at = COALESCE(published_at, ?), updated_at = ? WHERE id = ?`,
					Args: []any{it.Slug, it.Name, it.Note(lang), now, now, id}})
				continue
			}
			e := content.Entry{Kind: Kind, Slug: it.Slug, Lang: lang, TranslationGroupID: it.Group, Title: it.Name, Summary: it.Note(lang),
				Status: content.Draft, Visibility: content.Public}
			stmts = append(stmts, content.InsertEntry(&e, content.Body{}, content.OriginManual)...)
			stmts = append(stmts, content.PublishStmts(e.ID, *e.CurrentRevisionID, now)...)
			out.Entries = append(out.Entries, e.ID)
		}
	}
	if on == 0 {
		return Frozen{}, content.ValidationError{"items": "nothing on the radar"}
	}
	if _, err := r.db.Batch(ctx, stmts...); err != nil {
		return Frozen{}, err
	}
	return out, nil
}
