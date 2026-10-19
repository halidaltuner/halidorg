// Package series is the series of long-form entries (CONTENT_MODEL.md §1,
// Phase 5.5): a slug, a title and summary per language, a status. Entries
// join one with entries.series_id and series_order in the post editor.
package series

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"halid.org/engine/internal/content"
	"halid.org/engine/internal/store"
)

// Statuses are a series' states: active (still growing), done, archived
// (hidden from the site and the editor's list).
var Statuses = []string{"active", "done", "archived"}

// Series is one series.
type Series struct {
	ID        string `json:"id"`
	Slug      string `json:"slug"`
	TitleTR   string `json:"title_tr"`
	TitleEN   string `json:"title_en"`
	SummaryTR string `json:"summary_tr"`
	SummaryEN string `json:"summary_en"`
	Status    string `json:"status"`
	Sort      int    `json:"sort"`
}

// Title is the title in lang, else the other language's, else the slug.
func (s Series) Title(lang string) string {
	return pick(lang, s.TitleTR, s.TitleEN, s.Slug)
}

// Summary is the summary in lang, else the other language's.
func (s Series) Summary(lang string) string { return pick(lang, s.SummaryTR, s.SummaryEN, "") }

func pick(lang, tr, en, fallback string) string {
	a, b := en, tr
	if lang == "tr" {
		a, b = tr, en
	}
	switch {
	case a != "":
		return a
	case b != "":
		return b
	}
	return fallback
}

// Part is a published entry in a series.
type Part struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	Lang        string `json:"lang"`
	Slug        string `json:"slug"`
	Title       string `json:"title"`
	Summary     string `json:"summary"`
	Status      string `json:"status"`
	Order       *int64 `json:"series_order"`
	PublishedAt *int64 `json:"published_at"`
}

// Repo reads and writes series.
type Repo struct{ db store.DB }

// NewRepo returns a repository over db.
func NewRepo(db store.DB) *Repo { return &Repo{db: db} }

const columns = `id, slug, title_tr, title_en, summary_tr, summary_en, status, sort`

// All lists every series in order.
func (r *Repo) All(ctx context.Context) ([]Series, error) {
	res, err := r.db.Query(ctx, `SELECT `+columns+` FROM series ORDER BY sort, slug`)
	if err != nil {
		return nil, err
	}
	return store.Scan[Series](res)
}

// Get loads a series by id.
func (r *Repo) Get(ctx context.Context, id string) (Series, error) {
	return r.one(ctx, "id = ?", id)
}

// BySlug loads a series by slug.
func (r *Repo) BySlug(ctx context.Context, slug string) (Series, error) {
	return r.one(ctx, "slug = ?", slug)
}

func (r *Repo) one(ctx context.Context, where string, arg string) (Series, error) {
	res, err := r.db.Query(ctx, `SELECT `+columns+` FROM series WHERE `+where, arg)
	if err != nil {
		return Series{}, err
	}
	ss, err := store.Scan[Series](res)
	if err != nil {
		return Series{}, err
	}
	if len(ss) == 0 {
		return Series{}, content.NotFound("series", arg)
	}
	return ss[0], nil
}

// Validate checks a series' fields.
func Validate(s Series) error {
	v := content.ValidationError{}
	if !content.ValidSlug(s.Slug) {
		v["slug"] = "a-z, 0-9 and hyphens"
	}
	if s.TitleTR == "" && s.TitleEN == "" {
		v["title"] = "a title in at least one language"
	}
	for f, x := range map[string]struct {
		val string
		max int
	}{"title_tr": {s.TitleTR, 120}, "title_en": {s.TitleEN, 120}, "summary_tr": {s.SummaryTR, 300}, "summary_en": {s.SummaryEN, 300}} {
		if utf8.RuneCountInString(x.val) > x.max {
			v[f] = fmt.Sprintf("at most %d characters", x.max)
		}
	}
	if !slices.Contains(Statuses, s.Status) {
		v["status"] = "active, done or archived"
	}
	if len(v) > 0 {
		return v
	}
	return nil
}

// Save inserts (no id) or updates a series. An empty slug comes from the
// English title, else the Turkish one.
func (r *Repo) Save(ctx context.Context, s Series) (Series, error) {
	for _, f := range []*string{&s.Slug, &s.TitleTR, &s.TitleEN, &s.SummaryTR, &s.SummaryEN} {
		*f = strings.TrimSpace(*f)
	}
	if s.Slug == "" {
		s.Slug = content.Slugify(pick("en", s.TitleTR, s.TitleEN, ""))
	}
	if s.Status == "" {
		s.Status = Statuses[0]
	}
	if err := Validate(s); err != nil {
		return s, err
	}
	if s.ID == "" {
		s.ID = content.NewID()
	}
	_, err := r.db.Query(ctx, `INSERT INTO series (`+columns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (id) DO UPDATE SET slug = excluded.slug, title_tr = excluded.title_tr, title_en = excluded.title_en,
  summary_tr = excluded.summary_tr, summary_en = excluded.summary_en, status = excluded.status, sort = excluded.sort`,
		s.ID, s.Slug, s.TitleTR, s.TitleEN, s.SummaryTR, s.SummaryEN, s.Status, s.Sort)
	if content.IsConstraint(err) {
		return s, content.ValidationError{"slug": "already used by another series"}
	}
	return s, err
}

// Delete removes a series; its entries stay and leave it (series_id is set
// to NULL by the foreign key).
func (r *Repo) Delete(ctx context.Context, id string) error {
	if _, err := r.Get(ctx, id); err != nil {
		return err
	}
	_, err := r.db.Batch(ctx,
		store.Stmt{SQL: `UPDATE entries SET series_id = NULL, series_order = NULL WHERE series_id = ?`, Args: []any{id}},
		store.Stmt{SQL: `DELETE FROM series WHERE id = ?`, Args: []any{id}})
	return err
}

// Parts lists the entries in a series, in order: every status for the admin
// (lang ""), published and public ones in lang for the site.
func (r *Repo) Parts(ctx context.Context, id, lang string) ([]Part, error) {
	q := `SELECT id, kind, lang, slug, title, summary, status, series_order, published_at FROM entries WHERE series_id = ?`
	args := []any{id}
	if lang != "" {
		q += ` AND lang = ? AND status = 'published' AND visibility = 'public'`
		args = append(args, lang)
	}
	q += ` ORDER BY coalesce(series_order, 1000000), coalesce(published_at, created_at), id`
	res, err := r.db.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	return store.Scan[Part](res)
}

// Counts are the published, public parts per series in lang.
func (r *Repo) Counts(ctx context.Context, lang string) (map[string]int, error) {
	res, err := r.db.Query(ctx, `SELECT series_id, count(*) AS n FROM entries WHERE series_id IS NOT NULL AND lang = ?
  AND status = 'published' AND visibility = 'public' GROUP BY series_id`, lang)
	if err != nil {
		return nil, err
	}
	rows, err := store.Scan[struct {
		ID string `json:"series_id"`
		N  int    `json:"n"`
	}](res)
	if err != nil {
		return nil, err
	}
	out := map[string]int{}
	for _, x := range rows {
		out[x.ID] = x.N
	}
	return out, nil
}
