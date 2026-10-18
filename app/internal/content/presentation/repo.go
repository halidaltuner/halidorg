package presentation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"

	"halid.org/engine/internal/content"
	"halid.org/engine/internal/render"
	"halid.org/engine/internal/store"
)

// Repo reads and writes presentations.
type Repo struct{ db store.DB }

// NewRepo returns a repository over db.
func NewRepo(db store.DB) *Repo { return &Repo{db: db} }

const selectPresentation = `SELECT ` + content.EntryColumns + `, p.pres_type, p.event, p.city, p.held_on, p.talk_lang,
  p.slide_count, p.deck_r2_key, p.deck_version, p.recording_url, p.recording_minutes, p.outline_json
FROM entries e JOIN presentations p ON p.entry_id = e.id`

type row struct {
	Presentation
	OutlineJSON string `json:"outline_json"`
}

func (r row) presentation() Presentation {
	p := r.Presentation
	_ = json.Unmarshal([]byte(r.OutlineJSON), &p.Outline)
	return p
}

// normalize trims the fields, drops empty rows, sorts the outline by slide
// and numbers the links in order.
func normalize(d *Draft) {
	d.Kind = Kind
	d.Event, d.City = strings.TrimSpace(d.Event), strings.TrimSpace(d.City)
	if d.Type == "" {
		d.Type = "talk"
	}
	if d.TalkLang == "" {
		d.TalkLang = d.Lang
	}
	if d.RecordingURL != nil {
		if u := strings.TrimSpace(*d.RecordingURL); u == "" {
			d.RecordingURL = nil
		} else {
			d.RecordingURL = &u
		}
	}
	outline := d.Outline[:0:0]
	for _, s := range d.Outline {
		s.Title = strings.TrimSpace(s.Title)
		if s.Title == "" && s.N == 0 {
			continue
		}
		outline = append(outline, s)
	}
	sort.SliceStable(outline, func(i, j int) bool { return outline[i].N < outline[j].N })
	d.Outline = outline
	links := d.Links[:0:0]
	for _, l := range d.Links {
		l.Label, l.URL = strings.TrimSpace(l.Label), strings.TrimSpace(l.URL)
		if l.Label == "" && l.URL == "" {
			continue
		}
		l.Sort = len(links)
		links = append(links, l)
	}
	d.Links = links
}

// ownRows upsert the presentation row and replace its links. The deck
// columns belong to SetDeck and are kept.
func ownRows(id string, p Presentation) []store.Stmt {
	outline, _ := json.Marshal(nonNil(p.Outline))
	stmts := []store.Stmt{
		{SQL: `INSERT INTO presentations (entry_id, pres_type, event, city, held_on, talk_lang, slide_count, recording_url,
  recording_minutes, outline_json) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (entry_id) DO UPDATE SET pres_type = excluded.pres_type, event = excluded.event, city = excluded.city,
  held_on = excluded.held_on, talk_lang = excluded.talk_lang, slide_count = excluded.slide_count,
  recording_url = excluded.recording_url, recording_minutes = excluded.recording_minutes, outline_json = excluded.outline_json`,
			Args: []any{id, p.Type, p.Event, p.City, p.HeldOn, p.TalkLang, p.SlideCount, p.RecordingURL, p.RecordingMinutes, string(outline)}},
		{SQL: `DELETE FROM presentation_links WHERE presentation_id = ?`, Args: []any{id}},
	}
	for _, l := range p.Links {
		stmts = append(stmts, store.Stmt{SQL: `INSERT INTO presentation_links (presentation_id, label, url, sort) VALUES (?, ?, ?, ?)`,
			Args: []any{id, l.Label, l.URL, l.Sort}})
	}
	return stmts
}

func nonNil(s []Slide) []Slide {
	if s == nil {
		return []Slide{}
	}
	return s
}

// data is what a revision keeps of the rows, so a restore brings them back
// with the text.
type data struct {
	Outline []Slide `json:"outline"`
	Links   []Link  `json:"links"`
}

func body(d Draft) (content.Body, error) {
	out, err := render.Markdown([]byte(d.Markdown), render.Options{Lang: d.Lang})
	if err != nil {
		return content.Body{}, err
	}
	raw, err := json.Marshal(data{Outline: d.Outline, Links: d.Links})
	if err != nil {
		return content.Body{}, err
	}
	return content.Body{MD: d.Markdown, HTML: out.HTML, TOCJSON: out.TOCJSON(), DataJSON: string(raw)}, nil
}

// RowsOf fills p's rows from a revision's data_json.
func RowsOf(p *Presentation, dataJSON string) {
	var d data
	_ = json.Unmarshal([]byte(dataJSON), &d)
	p.Outline, p.Links = d.Outline, d.Links
}

// check validates p and its cover: an image from media. Its alt text is the
// media item's, else the presentation title.
func (r *Repo) check(ctx context.Context, p Presentation) error {
	if err := Validate(p); err != nil {
		return err
	}
	if p.CoverMediaID == nil {
		return nil
	}
	res, err := r.db.Query(ctx, `SELECT COUNT(*) AS n FROM media WHERE id = ? AND mime LIKE 'image/%'`,
		*p.CoverMediaID)
	if err != nil {
		return err
	}
	n, err := store.Scan[struct {
		N int `json:"n"`
	}](res)
	if err != nil {
		return err
	}
	if len(n) == 0 || n[0].N == 0 {
		return content.ValidationError{"cover": "an image from media"}
	}
	return nil
}

// Create validates and inserts a draft presentation with its rows.
func (r *Repo) Create(ctx context.Context, d Draft) (Presentation, error) {
	normalize(&d)
	if d.Status == "" || d.Status == content.Published {
		d.Status = content.Draft
	}
	if d.Visibility == "" {
		d.Visibility = content.Public
	}
	if err := r.check(ctx, d.Presentation); err != nil {
		return Presentation{}, err
	}
	b, err := body(d)
	if err != nil {
		return Presentation{}, err
	}
	err = content.CreateEntry(ctx, r.db, &d.Entry, b, d.Tags, func(id string) []store.Stmt { return ownRows(id, d.Presentation) })
	return d.Presentation, err
}

// Save writes the fields, replaces the rows and adds a revision.
func (r *Repo) Save(ctx context.Context, d Draft, origin string) (Presentation, error) {
	normalize(&d)
	cur, err := r.Get(ctx, d.ID)
	if err != nil {
		return Presentation{}, err
	}
	if err := r.check(ctx, d.Presentation); err != nil {
		return Presentation{}, err
	}
	b, err := body(d)
	if err != nil {
		return Presentation{}, err
	}
	if _, err := content.SaveEntry(ctx, r.db, cur.Entry, &d.Entry, b, origin, d.Tags, ownRows(d.ID, d.Presentation)); err != nil {
		return Presentation{}, err
	}
	return r.Get(ctx, d.ID)
}

// Publish points the published revision at the current one.
func (r *Repo) Publish(ctx context.Context, id string) (Presentation, error) {
	p, err := r.Get(ctx, id)
	if err != nil {
		return Presentation{}, err
	}
	err = content.PublishEntry(ctx, r.db, p.Entry, func(e content.Entry) error {
		x := p
		x.Entry = e
		return Validate(x)
	})
	if err != nil {
		return Presentation{}, err
	}
	return r.Get(ctx, id)
}

// SetDeck stores a deck in R2 (replacing the last one) and records it with
// its slide count. An empty outline is filled from the deck's headings;
// one already written is kept, so the deck does not overwrite edits.
func (r *Repo) SetDeck(ctx context.Context, blob store.Blob, id string, deck []byte) (Presentation, error) {
	p, err := r.Get(ctx, id)
	if err != nil {
		return Presentation{}, err
	}
	if err := CheckDeck(deck); err != nil {
		return Presentation{}, err
	}
	key := DeckKey(id)
	if err := blob.Put(ctx, key, deck, "text/html; charset=utf-8"); err != nil {
		return Presentation{}, err
	}
	sum := sha256.Sum256(deck)
	version := hex.EncodeToString(sum[:])[:12]
	count, outline := Outline(deck)
	if count == 0 {
		count = p.SlideCount
	}
	if len(p.Outline) > 0 {
		outline = p.Outline
	}
	raw, _ := json.Marshal(nonNil(outline))
	_, err = r.db.Query(ctx, `UPDATE presentations SET deck_r2_key = ?, deck_version = ?, slide_count = ?, outline_json = ? WHERE entry_id = ?`,
		key, version, count, string(raw), id)
	if err != nil {
		return Presentation{}, err
	}
	return r.Get(ctx, id)
}

// Get loads a presentation with its current rows, any status.
func (r *Repo) Get(ctx context.Context, id string) (Presentation, error) {
	p, err := r.one(ctx, "e.id = ?", id)
	if err != nil {
		return p, err
	}
	return p, r.links(ctx, &p)
}

// GetPublished loads a public presentation by language and slug, with the
// body, outline and links of its published revision. The deck and its
// slide count are the latest upload.
func (r *Repo) GetPublished(ctx context.Context, lang, slug string) (Presentation, error) {
	p, err := r.one(ctx, "e.lang = ? AND e.slug = ? AND e.status = 'published' AND e.visibility <> 'private'", lang, slug)
	if err != nil || p.PublishedRevisionID == nil {
		return p, err
	}
	rev, err := content.GetRevision(ctx, r.db, *p.PublishedRevisionID)
	if err != nil {
		return Presentation{}, err
	}
	RowsOf(&p, rev.DataJSON)
	p.HTML = rev.BodyHTML
	return p, nil
}

// ListFilter selects presentations for List.
type ListFilter struct {
	Lang  string
	Admin bool // every status; otherwise published and public only
}

// List returns presentations without their links, newest first by the day
// they were given.
func (r *Repo) List(ctx context.Context, f ListFilter) ([]Presentation, error) {
	where, args := []string{"1 = 1"}, []any{}
	if f.Lang != "" {
		where, args = append(where, "e.lang = ?"), append(args, f.Lang)
	}
	if !f.Admin {
		where = append(where, "e.status = 'published' AND e.visibility = 'public'")
	}
	res, err := r.db.Query(ctx, selectPresentation+" WHERE "+strings.Join(where, " AND ")+
		" ORDER BY coalesce(p.held_on, e.published_at, e.created_at) DESC, e.id DESC LIMIT 500", args...)
	if err != nil {
		return nil, err
	}
	rows, err := store.Scan[row](res)
	if err != nil {
		return nil, err
	}
	out := make([]Presentation, len(rows))
	for i, x := range rows {
		out[i] = x.presentation()
	}
	return out, nil
}

func (r *Repo) one(ctx context.Context, where string, args ...any) (Presentation, error) {
	res, err := r.db.Query(ctx, selectPresentation+" WHERE "+where+" LIMIT 1", args...)
	if err != nil {
		return Presentation{}, err
	}
	rows, err := store.Scan[row](res)
	if err != nil {
		return Presentation{}, err
	}
	if len(rows) == 0 {
		return Presentation{}, content.NotFound(Kind, where)
	}
	return rows[0].presentation(), nil
}

func (r *Repo) links(ctx context.Context, p *Presentation) error {
	res, err := r.db.Query(ctx, `SELECT label, url, sort FROM presentation_links WHERE presentation_id = ? ORDER BY sort`, p.ID)
	if err != nil {
		return err
	}
	p.Links, err = store.Scan[Link](res)
	return err
}

// DeleteStmts remove a presentation; its rows cascade. The deck object is
// left in R2: DeleteDeck removes it.
func DeleteStmts(id string) []store.Stmt {
	return []store.Stmt{{SQL: `DELETE FROM entries WHERE id = ? AND kind = 'presentation'`, Args: []any{id}}}
}

// DeleteDeck removes an entry's deck from R2; a missing one is no error.
func DeleteDeck(ctx context.Context, blob store.Blob, id string) error {
	return blob.Delete(ctx, DeckKey(id))
}
