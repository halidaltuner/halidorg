// Package export writes the weekly full export (Phase 9.1): every entry as
// Markdown with front matter, every table of the main database as JSON
// (galleries, radar, settings and the rest), and a media manifest, zipped
// into the backups bucket at exports/{date}.zip. The nightly SQL dump of
// the same database comes from .github/workflows/backup.yml; this one is
// for reading and moving content, not for restoring the database.
package export

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"halid.org/engine/internal/content"
	"halid.org/engine/internal/jobs"
	"halid.org/engine/internal/store"
)

// TypeWeekly is the job the Worker's Sunday cron queues (worker/src/jobs.ts).
const TypeWeekly = "export.weekly"

// Prefix is where exports go in the backups bucket.
const Prefix = "exports/"

// page is how many rows one query reads.
const page = 500

// Exporter builds and stores the export.
type Exporter struct {
	db      store.DB
	backups store.Blob
	origin  string // PUBLIC_ORIGIN, for the media manifest's URLs
}

// New returns an exporter reading db and writing to backups.
func New(db store.DB, backups store.Blob) *Exporter {
	return &Exporter{db: db, backups: backups, origin: strings.TrimRight(os.Getenv("PUBLIC_ORIGIN"), "/")}
}

// Register adds the weekly export handler to j.
func (e *Exporter) Register(j *jobs.Jobs) {
	j.Handle(TypeWeekly, func(ctx context.Context, _ json.RawMessage) error {
		_, err := e.Run(ctx)
		return err
	})
}

// Run writes today's export and returns its key. Running it twice on one
// day replaces the file.
func (e *Exporter) Run(ctx context.Context) (string, error) {
	if e.backups == nil {
		return "", jobs.Permanent{Err: fmt.Errorf("export: no backups bucket")}
	}
	data, err := e.Build(ctx)
	if err != nil {
		return "", err
	}
	key := Prefix + content.Clock().UTC().Format("2006-01-02") + ".zip"
	return key, e.backups.Put(ctx, key, data, "application/zip")
}

// Build returns the zip.
func (e *Exporter) Build(ctx context.Context) ([]byte, error) {
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	now := content.Clock().UTC()
	add := func(name string, body []byte) error {
		w, err := z.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate, Modified: now})
		if err != nil {
			return err
		}
		_, err = w.Write(body)
		return err
	}
	if err := add("README.txt", []byte(readme(now))); err != nil {
		return nil, err
	}
	tables, err := e.tables(ctx)
	if err != nil {
		return nil, err
	}
	for _, t := range tables {
		rows, err := e.rows(ctx, t)
		if err != nil {
			return nil, fmt.Errorf("export: %s: %w", t, err)
		}
		b, err := json.MarshalIndent(rows, "", "  ")
		if err != nil {
			return nil, err
		}
		if err := add("tables/"+t+".json", b); err != nil {
			return nil, err
		}
	}
	if err := e.entries(ctx, add); err != nil {
		return nil, err
	}
	if err := e.media(ctx, add); err != nil {
		return nil, err
	}
	if err := z.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func readme(now time.Time) string {
	return "halid.org export, " + now.Format(time.RFC3339) + `

entries/{lang}/{kind}/{slug}.md  every entry: front matter (JSON values, valid YAML), then its
                                 newest Markdown (the published text may be older: see
                                 published_revision_id and tables/revisions.json)
tables/{table}.json              every table of the main database, all rows
media/manifest.json              media library: R2 key, public URL, type, size, alt texts

Images and PDFs are not included; they stay in the media bucket under the R2 keys listed.
The database itself is restored from the nightly SQL dump (docs/runbook-restore.md).
`
}

// tables are the main database's own tables, by name.
func (e *Exporter) tables(ctx context.Context) ([]string, error) {
	res, err := e.db.Query(ctx, `SELECT name FROM sqlite_master WHERE type = 'table'
AND name NOT LIKE 'sqlite_%' AND name NOT LIKE '_cf_%' AND name <> 'd1_migrations' ORDER BY name`)
	if err != nil {
		return nil, err
	}
	rows, err := store.Scan[struct {
		Name string `json:"name"`
	}](res)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.Name)
	}
	return out, nil
}

// rows reads a whole table, page by page in rowid order.
func (e *Exporter) rows(ctx context.Context, table string) ([]map[string]any, error) {
	if strings.ContainsAny(table, `"`) {
		return nil, fmt.Errorf("bad table name")
	}
	out := []map[string]any{}
	for off := 0; ; off += page {
		res, err := e.db.Query(ctx, `SELECT * FROM "`+table+`" ORDER BY rowid LIMIT ? OFFSET ?`, page, off)
		if err != nil {
			return nil, err
		}
		rows, err := store.Scan[map[string]any](res)
		if err != nil {
			return nil, err
		}
		out = append(out, rows...)
		if len(rows) < page {
			return out, nil
		}
	}
}

type entryRow struct {
	ID          string  `json:"id"`
	Kind        string  `json:"kind"`
	Lang        string  `json:"lang"`
	Slug        string  `json:"slug"`
	Title       string  `json:"title"`
	Summary     string  `json:"summary"`
	Status      string  `json:"status"`
	Visibility  string  `json:"visibility"`
	Group       string  `json:"translation_group_id"`
	PublishedAt *int64  `json:"published_at"`
	CreatedAt   int64   `json:"created_at"`
	UpdatedAt   int64   `json:"updated_at"`
	Current     *string `json:"current_revision_id"`
	Published   *string `json:"published_revision_id"`
	Cover       *string `json:"cover_media_id"`
	Series      *string `json:"series"`
	SeriesOrder *int    `json:"series_order"`
	Tags        *string `json:"tags"`
	Markdown    *string `json:"body_md"`
	Data        *string `json:"data_json"`
}

func (e *Exporter) entries(ctx context.Context, add func(string, []byte) error) error {
	for off := 0; ; off += page {
		res, err := e.db.Query(ctx, `SELECT e.id, e.kind, e.lang, e.slug, e.title, e.summary, e.status, e.visibility,
  e.translation_group_id, e.published_at, e.created_at, e.updated_at, e.current_revision_id, e.published_revision_id,
  e.cover_media_id, e.series_order, (SELECT s.slug FROM series s WHERE s.id = e.series_id) AS series,
  (SELECT group_concat(t.slug, ' ') FROM entry_tags et JOIN tags t ON t.id = et.tag_id WHERE et.entry_id = e.id) AS tags,
  r.body_md, r.data_json
FROM entries e LEFT JOIN revisions r ON r.id = e.current_revision_id
ORDER BY e.rowid LIMIT ? OFFSET ?`, page, off)
		if err != nil {
			return err
		}
		rows, err := store.Scan[entryRow](res)
		if err != nil {
			return err
		}
		for _, r := range rows {
			if err := add("entries/"+r.Lang+"/"+r.Kind+"/"+r.Slug+".md", markdown(r)); err != nil {
				return err
			}
		}
		if len(rows) < page {
			return nil
		}
	}
}

// markdown is one entry: front matter with JSON-encoded values (valid
// YAML), then the body.
func markdown(r entryRow) []byte {
	var b bytes.Buffer
	b.WriteString("---\n")
	field := func(k string, v any) {
		j, _ := json.Marshal(v)
		fmt.Fprintf(&b, "%s: %s\n", k, j)
	}
	field("id", r.ID)
	field("kind", r.Kind)
	field("lang", r.Lang)
	field("slug", r.Slug)
	field("title", r.Title)
	field("summary", r.Summary)
	field("status", r.Status)
	field("visibility", r.Visibility)
	field("translation_group_id", r.Group)
	field("published_at", unix(r.PublishedAt))
	field("created_at", unix(&r.CreatedAt))
	field("updated_at", unix(&r.UpdatedAt))
	tags := []string{}
	if r.Tags != nil && *r.Tags != "" {
		tags = strings.Fields(*r.Tags)
	}
	field("tags", tags)
	if r.Series != nil {
		field("series", *r.Series)
		field("series_order", r.SeriesOrder)
	}
	if r.Cover != nil {
		field("cover_media_id", *r.Cover)
	}
	field("current_revision_id", r.Current)
	field("published_revision_id", r.Published)
	if r.Data != nil && *r.Data != "" && *r.Data != "{}" {
		var v any
		if json.Unmarshal([]byte(*r.Data), &v) == nil {
			field("data", v)
		}
	}
	b.WriteString("---\n")
	if r.Markdown != nil {
		b.WriteString(*r.Markdown)
		if !strings.HasSuffix(*r.Markdown, "\n") {
			b.WriteByte('\n')
		}
	}
	return b.Bytes()
}

// unix formats seconds as RFC 3339 in UTC, or nil.
func unix(t *int64) any {
	if t == nil {
		return nil
	}
	return time.Unix(*t, 0).UTC().Format(time.RFC3339)
}

func (e *Exporter) media(ctx context.Context, add func(string, []byte) error) error {
	type item struct {
		ID     string  `json:"id"`
		Key    string  `json:"r2_key"`
		URL    string  `json:"url"`
		MIME   string  `json:"mime"`
		Name   *string `json:"name"`
		Bytes  *int64  `json:"bytes"`
		Width  *int    `json:"width"`
		Height *int    `json:"height"`
		AltTR  *string `json:"alt_tr"`
		AltEN  *string `json:"alt_en"`
	}
	out := []item{}
	for off := 0; ; off += page {
		res, err := e.db.Query(ctx, `SELECT id, r2_key, mime, name, bytes, width, height, alt_tr, alt_en FROM media ORDER BY rowid LIMIT ? OFFSET ?`, page, off)
		if err != nil {
			return err
		}
		rows, err := store.Scan[item](res)
		if err != nil {
			return err
		}
		for _, m := range rows {
			m.URL = e.origin + "/" + m.Key
			out = append(out, m)
		}
		if len(rows) < page {
			break
		}
	}
	b, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	return add("media/manifest.json", b)
}
