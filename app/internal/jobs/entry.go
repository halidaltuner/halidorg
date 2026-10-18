package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

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
}

// NewEntryJobs wires the handler; register it with jobs.Handle. blob is
// where OG cards go (R2).
func NewEntryJobs(db, searchDB store.DB, blob store.Blob, warm *Warmer) *EntryJobs {
	return &EntryJobs{db: db, search: searchDB, blob: blob, warm: warm}
}

// Register adds the entry handlers to j.
func (e *EntryJobs) Register(j *Jobs) { j.Handle(TypeEntryChanged, e.changed) }

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
	Tags       *string `json:"tags"`
	Published  *int64  `json:"published_at"`
	Minutes    *int    `json:"reading_minutes"`
}

// load returns the entry with the body search should see: the published
// revision for published entries, the current one otherwise (admin search
// finds drafts by their latest text). nil when the entry is gone.
func load(ctx context.Context, db store.DB, id string) (*entryRow, error) {
	res, err := db.Query(ctx, `SELECT e.id, e.kind, e.lang, e.slug, e.title, e.summary, e.status, e.visibility,
  e.translation_group_id, r.body_md, e.published_at,
  (SELECT reading_minutes FROM posts WHERE entry_id = e.id) AS reading_minutes,
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
	return Ref{Kind: r.Kind, Lang: r.Lang, Slug: r.Slug, Tags: r.tags(),
		Listed: r.Status == content.Published && r.Visibility == content.Public}
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
	}
	// Cards first: the pages warmed next point og:image at them.
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
	if c.Now != nil && c.Now.Listed && row.Kind == "post" && row.Published != nil {
		minutes := 0
		if row.Minutes != nil {
			minutes = *row.Minutes
		}
		png, err := og.Render(og.Post(row.Lang, row.Title, *row.Published, minutes))
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
	doc := search.Doc{EntryID: row.ID, Kind: row.Kind, Lang: row.Lang, Status: row.Status, Visibility: row.Visibility,
		Slug: row.Slug, Title: row.Title, Summary: row.Summary, Body: text, Tags: row.tags()}
	_, err := e.search.Batch(ctx, doc.Upsert()...)
	return err
}
