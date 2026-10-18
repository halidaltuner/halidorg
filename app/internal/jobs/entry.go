package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"halid.org/engine/internal/content"
	"halid.org/engine/internal/render"
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
	warm   *Warmer
}

// NewEntryJobs wires the handler; register it with jobs.Handle.
func NewEntryJobs(db, searchDB store.DB, warm *Warmer) *EntryJobs {
	return &EntryJobs{db: db, search: searchDB, warm: warm}
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
}

// load returns the entry with the body search should see: the published
// revision for published entries, the current one otherwise (admin search
// finds drafts by their latest text). nil when the entry is gone.
func load(ctx context.Context, db store.DB, id string) (*entryRow, error) {
	res, err := db.Query(ctx, `SELECT e.id, e.kind, e.lang, e.slug, e.title, e.summary, e.status, e.visibility,
  e.translation_group_id, r.body_md,
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
	if _, err := e.warm.Warm(ctx, Paths(c)); err != nil {
		return err
	}
	return e.index(ctx, p.EntryID, row)
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
