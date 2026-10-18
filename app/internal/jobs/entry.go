package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"halid.org/engine/internal/content"
	"halid.org/engine/internal/i18n"
	"halid.org/engine/internal/render"
	"halid.org/engine/internal/render/og"
	"halid.org/engine/internal/search"
	"halid.org/engine/internal/store"
)

// TypeEntryChanged is the job every entry write enqueues: publish, save of a
// published entry, unpublish, delete.
const TypeEntryChanged = "entry.changed"

// EntryChanged is its payload. Prev is the entry as it was before the
// write (from RefOf), so old URLs and old tag pages are purged too.
type EntryChanged struct {
	EntryID string `json:"entry_id"`
	Prev    *Ref   `json:"prev,omitempty"`
}

// EntryJobs re-renders and re-indexes entries.
type EntryJobs struct {
	db     store.DB
	search store.DB
	blob   store.Blob
	warm   *Warmer
	origin string   // PUBLIC_ORIGIN: absolute links on it count as internal
	icons  Favicons // nil: links keep the initial instead of a favicon
}

// NewEntryJobs wires the handler; register it with jobs.Handle. blob is
// where OG cards go (R2).
func NewEntryJobs(db, searchDB store.DB, blob store.Blob, warm *Warmer) *EntryJobs {
	return &EntryJobs{db: db, search: searchDB, blob: blob, warm: warm,
		origin: strings.TrimRight(os.Getenv("PUBLIC_ORIGIN"), "/")}
}

// Register adds the entry handlers to j.
func (e *EntryJobs) Register(j *Jobs) {
	j.Handle(TypeEntryChanged, e.changed)
	j.Handle(TypePagesChanged, e.pages)
}

// Enqueue is what write paths call after their batch commits.
func Enqueue(ctx context.Context, j *Jobs, entryID string, prev *Ref) error {
	_, err := j.Enqueue(ctx, TypeEntryChanged, EntryChanged{EntryID: entryID, Prev: prev})
	return err
}

type entryRow struct {
	ID         string  `json:"id"`
	Kind       string  `json:"kind"`
	Lang       string  `json:"lang"`
	Slug       string  `json:"slug"`
	Title      string  `json:"title"`
	Summary    string  `json:"summary"`
	Status     string  `json:"status"`
	Visibility string  `json:"visibility"`
	Group      string  `json:"translation_group_id"`
	BodyMD     *string `json:"body_md"`
	DataJSON   *string `json:"data_json"`
	Extra      *string `json:"extra"`
	Tags       *string `json:"tags"`
	Categories *string `json:"categories"`
	Homelab    *int    `json:"homelab"`
	Parent     *string `json:"parent"`
	Logs       *string `json:"logs"`
	Published  *int64  `json:"published_at"`
	Minutes    *int    `json:"reading_minutes"`
	Version    *string `json:"version_day"`
}

// load returns the entry with the body search should see: the published
// revision for published entries, the current one otherwise (admin search
// finds drafts by their latest text). nil when the entry is gone.
func load(ctx context.Context, db store.DB, id string) (*entryRow, error) {
	res, err := db.Query(ctx, `SELECT e.id, e.kind, e.lang, e.slug, e.title, e.summary, e.status, e.visibility,
  e.translation_group_id, r.body_md, r.data_json, e.published_at,
  -- kind fields worth finding: a link's domain and URL, a fav's author
  CASE e.kind WHEN 'link' THEN (SELECT l.domain || ' ' || l.url FROM links l WHERE l.entry_id = e.id)
              WHEN 'fav' THEN (SELECT coalesce(f.author, '') || ' ' || f.fav_type FROM favs f WHERE f.entry_id = e.id)
              -- a project's tagline and stack, a build log's project
              WHEN 'project' THEN (SELECT p.tagline || ' ' || p.kind_line || coalesce(' ' || (SELECT group_concat(s.name, ' ')
                FROM project_stack s WHERE s.project_id = p.entry_id), '') FROM projects p WHERE p.entry_id = e.id)
              WHEN 'buildlog' THEN (SELECT pe.title FROM buildlogs b JOIN entries pe ON pe.id = b.project_entry_id WHERE b.entry_id = e.id) END AS extra,
  (SELECT p.project_type = 'homelab' FROM projects p WHERE p.entry_id = e.id) AS homelab,
  (SELECT pe.slug FROM buildlogs b JOIN entries pe ON pe.id = b.project_entry_id WHERE b.entry_id = e.id) AS parent,
  (SELECT group_concat(le.slug, ' ') FROM buildlogs b JOIN entries le ON le.id = b.entry_id WHERE b.project_entry_id = e.id) AS logs,
  (SELECT reading_minutes FROM posts WHERE entry_id = e.id) AS reading_minutes,
  (SELECT strftime('%Y-%m-%d', max(published_at), 'unixepoch') FROM page_versions WHERE entry_id = e.id) AS version_day,
  -- a link's category and its parent: both awesome pages list it
  (SELECT c.slug || coalesce(' ' || p.slug, '') FROM links l JOIN categories c ON c.id = l.category_id
     LEFT JOIN categories p ON p.id = c.parent_id WHERE l.entry_id = e.id) AS categories,
  (SELECT group_concat(t.slug, ' ') FROM entry_tags et JOIN tags t ON t.id = et.tag_id WHERE et.entry_id = e.id) AS tags
FROM entries e
LEFT JOIN revisions r ON r.id = CASE WHEN e.status = 'published' THEN e.published_revision_id ELSE e.current_revision_id END
WHERE e.id = ?`, id)
	if err != nil {
		return nil, err
	}
	rows, err := store.Scan[entryRow](res)
	if err != nil || len(rows) == 0 {
		return nil, err
	}
	return &rows[0], nil
}

func (r *entryRow) tags() []string {
	if r.Tags == nil || *r.Tags == "" {
		return nil
	}
	return strings.Fields(*r.Tags)
}

func (r *entryRow) ref() Ref {
	ref := Ref{Kind: r.Kind, Lang: r.Lang, Slug: r.Slug, Tags: r.tags(), Title: r.Title,
		Listed: r.Status == content.Published && r.Visibility == content.Public}
	if r.Categories != nil {
		ref.Categories = strings.Fields(*r.Categories)
	}
	ref.Homelab = r.Homelab != nil && *r.Homelab == 1
	if r.Published != nil {
		ref.Month = time.Unix(*r.Published, 0).UTC().Format("2006-01")
	}
	if r.Version != nil {
		ref.Version = *r.Version
	}
	if r.Parent != nil {
		ref.Parent = *r.Parent
	}
	if r.Logs != nil {
		ref.Logs = strings.Fields(*r.Logs)
	}
	if r.Kind == "snippet" && r.DataJSON != nil {
		for _, f := range snippetFiles(*r.DataJSON) {
			ref.Files = append(ref.Files, f.Filename)
		}
	}
	return ref
}

// RefOf snapshots an entry before a write, for EntryChanged.Prev.
func RefOf(ctx context.Context, db store.DB, id string) (*Ref, error) {
	row, err := load(ctx, db, id)
	if err != nil || row == nil {
		return nil, err
	}
	r := row.ref()
	return &r, nil
}

func (e *EntryJobs) changed(ctx context.Context, raw json.RawMessage) error {
	var p EntryChanged
	if err := json.Unmarshal(raw, &p); err != nil || p.EntryID == "" {
		return Permanent{fmt.Errorf("entry.changed: bad payload: %s", raw)}
	}
	row, err := load(ctx, e.db, p.EntryID)
	if err != nil {
		return err
	}
	c := Change{Prev: p.Prev}
	if row != nil {
		r := row.ref()
		c.Now = &r
		sibs, err := siblings(ctx, e.db, row)
		if err != nil {
			return err
		}
		c.Siblings = sibs
		linked, err := e.links(ctx, row, c.Now.Listed)
		if err != nil {
			return err
		}
		c.Linked = linked
	}
	if PeersChanged(c) {
		if c.Peers, err = peers(ctx, e.db, c); err != nil {
			return err
		}
	}
	// Favicon and cards first: the pages warmed next show them.
	if err := e.favicon(ctx, row); err != nil {
		return err
	}
	if err := e.cards(ctx, c, row); err != nil {
		return err
	}
	if _, err := e.warm.Warm(ctx, Paths(c)); err != nil {
		return err
	}
	return e.index(ctx, p.EntryID, row)
}

// cards keeps OG images in step with the entry: rendered while it is listed,
// removed from its old URL when it moves, is unlisted or is deleted. The
// site cards are written the first time a job sees their design Version.
func (e *EntryJobs) cards(ctx context.Context, c Change, row *entryRow) error {
	if p := c.Prev; p != nil {
		if key := cardKey(*p); key != "" && (c.Now == nil || !c.Now.Listed || cardKey(*c.Now) != key) {
			if err := e.blob.Delete(ctx, key); err != nil {
				return err
			}
		}
	}
	if c.Now != nil && c.Now.Listed && cardKey(*c.Now) != "" && row.Published != nil {
		var card og.Card
		if row.Kind == "buildlog" {
			card = og.BuildLog(row.Lang, row.Title, row.Slug, *row.Published)
		} else if row.Kind == "post" {
			minutes := 0
			if row.Minutes != nil {
				minutes = *row.Minutes
			}
			card = og.Post(row.Lang, row.Title, *row.Published, minutes)
		} else {
			card = og.Entry(row.Kind, row.Lang, row.Title, row.Summary, row.Slug, *row.Published)
		}
		png, err := og.Render(card)
		if err != nil {
			return err
		}
		if err := e.blob.Put(ctx, cardKey(*c.Now), png, "image/png"); err != nil {
			return err
		}
	}
	return e.siteCards(ctx)
}

func cardKey(r Ref) string {
	if r.Kind == "buildlog" && r.Slug != "" && i18n.IsSupported(r.Lang) {
		return og.EntryKey(r.Lang, og.LogSection, r.Slug)
	}
	seg, ok := kindPath[r.Kind]
	if !ok || r.Slug == "" || !i18n.IsSupported(r.Lang) {
		return ""
	}
	return og.EntryKey(r.Lang, seg, r.Slug)
}

func (e *EntryJobs) siteCards(ctx context.Context) error {
	for _, l := range i18n.Supported {
		key := og.SiteKey(l)
		if _, err := e.blob.Get(ctx, key); err == nil {
			continue
		} else if !errors.Is(err, store.ErrNotFound) {
			return err
		}
		png, err := og.Render(og.Site(l))
		if err != nil {
			return err
		}
		if err := e.blob.Put(ctx, key, png, "image/png"); err != nil {
			return err
		}
	}
	return nil
}

// links rebuilds the entry's outgoing rows in links_graph from its published
// HTML and returns the entries it linked to before or links to now: their
// backlink lists change, so their pages are re-rendered too.
func (e *EntryJobs) links(ctx context.Context, row *entryRow, listed bool) ([]Ref, error) {
	before, err := content.LinkTargets(ctx, e.db, row.ID)
	if err != nil {
		return nil, err
	}
	var found []content.Link
	if listed {
		res, err := e.db.Query(ctx, `SELECT r.body_html FROM entries e JOIN revisions r ON r.id = e.published_revision_id WHERE e.id = ?`, row.ID)
		if err != nil {
			return nil, err
		}
		bodies, err := store.Scan[struct {
			HTML string `json:"body_html"`
		}](res)
		if err != nil {
			return nil, err
		}
		if len(bodies) == 1 {
			found = content.ExtractLinks(bodies[0].HTML, e.origin)
		}
	}
	if _, err := e.db.Batch(ctx, content.SetLinksStmts(row.ID, found)...); err != nil {
		return nil, err
	}
	after, err := content.LinkTargets(ctx, e.db, row.ID)
	if err != nil {
		return nil, err
	}
	ids := append(before, after...)
	if len(ids) == 0 {
		return nil, nil
	}
	q := `SELECT kind, lang, slug FROM entries WHERE id IN (?` + strings.Repeat(", ?", len(ids)-1) + `)`
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	res, err := e.db.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	return store.Scan[Ref](res)
}

type snippetFile struct {
	Filename string `json:"filename"`
	Code     string `json:"code"`
}

// snippetFiles reads the files a snippet revision keeps in data_json
// (content/snippet.FilesOf). Kept here so jobs does not import kind packages,
// which import jobs for their admin handlers.
func snippetFiles(dataJSON string) []snippetFile {
	var d struct {
		Files []snippetFile `json:"files"`
	}
	_ = json.Unmarshal([]byte(dataJSON), &d)
	return d.Files
}

// peers lists the other listed snippets in the languages of c, newest
// first, as many as the snippet list shows (public.snippetList).
func peers(ctx context.Context, db store.DB, c Change) ([]Ref, error) {
	langs := map[string]bool{}
	for _, r := range []*Ref{c.Now, c.Prev} {
		if r != nil {
			langs[r.Lang] = true
		}
	}
	var out []Ref
	for l := range langs {
		res, err := db.Query(ctx, `SELECT kind, lang, slug FROM entries
WHERE kind = 'snippet' AND lang = ? AND status = 'published' AND visibility = 'public'
ORDER BY published_at DESC, id DESC LIMIT 50`, l)
		if err != nil {
			return nil, err
		}
		rows, err := store.Scan[Ref](res)
		if err != nil {
			return nil, err
		}
		out = append(out, rows...)
	}
	return out, nil
}

func siblings(ctx context.Context, db store.DB, row *entryRow) ([]Ref, error) {
	res, err := db.Query(ctx, `SELECT kind, lang, slug FROM entries WHERE translation_group_id = ? AND id <> ?`, row.Group, row.ID)
	if err != nil {
		return nil, err
	}
	return store.Scan[Ref](res)
}

// index keeps the SEARCH database in step: private and deleted entries are
// removed; everything else is indexed with its status so public queries can
// filter drafts out (search.Filter).
func (e *EntryJobs) index(ctx context.Context, id string, row *entryRow) error {
	if row == nil || row.Visibility == content.Private {
		del := search.Delete(id)
		_, err := e.search.Query(ctx, del.SQL, del.Args...)
		return err
	}
	text := ""
	if row.BodyMD != nil {
		out, err := render.Markdown([]byte(*row.BodyMD), render.Options{Lang: row.Lang})
		if err != nil {
			return err
		}
		text = out.Text
	}
	// Kind fields: a snippet's file names and code (from the same revision
	// as the body), a link's domain and URL, a fav's author and type.
	if row.Kind == "snippet" && row.DataJSON != nil {
		for _, f := range snippetFiles(*row.DataJSON) {
			text += "\n" + f.Filename + "\n" + f.Code
		}
	}
	if row.Extra != nil {
		text += "\n" + *row.Extra
	}
	// Homelab builds are projects, indexed under their own kind so the
	// palette's h: and pr: prefixes tell them apart.
	kind := row.Kind
	if row.Homelab != nil && *row.Homelab == 1 {
		kind = "homelab"
	}
	doc := search.Doc{EntryID: row.ID, Kind: kind, Lang: row.Lang, Status: row.Status, Visibility: row.Visibility,
		Slug: row.Slug, Title: row.Title, Summary: row.Summary, Body: text, Tags: row.tags()}
	_, err := e.search.Batch(ctx, doc.Upsert()...)
	return err
}
