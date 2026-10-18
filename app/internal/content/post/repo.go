package post

import (
	"context"
	"strings"

	"halid.org/engine/internal/content"
	"halid.org/engine/internal/render"
	"halid.org/engine/internal/store"
)

// Repo reads and writes posts through store.DB (the Worker bridge in the
// container, SQLite in tests).
type Repo struct{ db store.DB }

// NewRepo returns a repository over db.
func NewRepo(db store.DB) *Repo { return &Repo{db: db} }

const selectPost = `SELECT ` + content.EntryColumns + `,
  p.reading_minutes, p.canonical_url, p.mermaid, p.comments_enabled
FROM entries e JOIN posts p ON p.entry_id = e.id`

// Draft is the input for Create and Save: entry fields, Markdown and tags.
type Draft struct {
	Post
	Markdown string
	Tags     []string
}

// rendered renders the Markdown and fills the derived post fields.
func (d *Draft) rendered() (content.Body, error) {
	out, err := render.Markdown([]byte(d.Markdown), render.Options{Lang: d.Lang})
	if err != nil {
		return content.Body{}, err
	}
	d.ReadingMinutes = ReadingMinutes(out.Words)
	d.Mermaid = out.Mermaid
	return content.Body{MD: d.Markdown, HTML: out.HTML, TOCJSON: out.TOCJSON()}, nil
}

func postRow(p Post) store.Stmt {
	return store.Stmt{
		SQL: `INSERT INTO posts (entry_id, reading_minutes, canonical_url, mermaid, comments_enabled) VALUES (?, ?, ?, ?, ?)
ON CONFLICT (entry_id) DO UPDATE SET reading_minutes = excluded.reading_minutes,
  canonical_url = excluded.canonical_url, mermaid = excluded.mermaid, comments_enabled = excluded.comments_enabled`,
		Args: []any{p.ID, p.ReadingMinutes, p.CanonicalURL, p.Mermaid, p.CommentsEnabled},
	}
}

// Create validates, renders and inserts a new post with its first revision
// and tags, in one transaction. New posts start as drafts unless d says
// otherwise; publishing goes through Publish.
func (r *Repo) Create(ctx context.Context, d Draft) (Post, error) {
	d.Kind = Kind
	if d.Status == "" {
		d.Status = content.Draft
	}
	if d.Visibility == "" {
		d.Visibility = content.Public
	}
	if d.Status == content.Published {
		// The published pointer only moves in Publish.
		d.Status = content.Draft
	}
	if err := Validate(d.Post); err != nil {
		return Post{}, err
	}
	body, err := d.rendered()
	if err != nil {
		return Post{}, err
	}
	stmts := content.InsertEntry(&d.Entry, body, content.OriginManual)
	stmts = append(stmts, postRow(d.Post))
	stmts = append(stmts, content.SetTagsStmts(d.ID, d.Tags)...)
	if _, err := r.db.Batch(ctx, stmts...); err != nil {
		if content.IsConstraint(err) {
			return Post{}, content.ValidationError{"slug": "already used by another post in this language"}
		}
		return Post{}, err
	}
	return d.Post, nil
}

// Save writes the editable fields and a revision. origin is autosave or
// manual; autosaves coalesce (content.SaveRevision). Status changes to
// published are refused here: use Publish.
func (r *Repo) Save(ctx context.Context, d Draft, origin string) (Post, string, error) {
	d.Kind = Kind
	cur, err := r.Get(ctx, d.ID)
	if err != nil {
		return Post{}, "", err
	}
	if d.Status == content.Published && cur.Status != content.Published {
		d.Status = cur.Status
	}
	if err := Validate(d.Post); err != nil {
		return Post{}, "", err
	}
	body, err := d.rendered()
	if err != nil {
		return Post{}, "", err
	}
	stmts := []store.Stmt{content.UpdateEntry(&d.Entry), postRow(d.Post)}
	stmts = append(stmts, content.SetTagsStmts(d.ID, d.Tags)...)
	if _, err := r.db.Batch(ctx, stmts...); err != nil {
		if content.IsConstraint(err) {
			return Post{}, "", content.ValidationError{"slug": "already used by another post in this language"}
		}
		return Post{}, "", err
	}
	rev, err := content.SaveRevision(ctx, r.db, d.ID, body, origin)
	if err != nil {
		return Post{}, "", err
	}
	d.CurrentRevisionID = &rev
	return d.Post, rev, nil
}

// Publish points published_revision_id at the current revision and sets
// the status. Rendering into KV and indexing happen in jobs (Phase 1.3).
func (r *Repo) Publish(ctx context.Context, id string) (Post, error) {
	p, err := r.Get(ctx, id)
	if err != nil {
		return Post{}, err
	}
	now := content.Clock().Unix()
	p.Status = content.Published
	if p.PublishedAt == nil {
		p.PublishedAt = &now
	}
	if err := Validate(p); err != nil {
		return Post{}, err
	}
	if p.CurrentRevisionID == nil {
		return Post{}, content.ValidationError{"body": "nothing to publish"}
	}
	if _, err := r.db.Batch(ctx, content.PublishStmts(id, *p.CurrentRevisionID, now)...); err != nil {
		return Post{}, err
	}
	return r.Get(ctx, id)
}

func (r *Repo) one(ctx context.Context, where string, args ...any) (Post, error) {
	res, err := r.db.Query(ctx, selectPost+" WHERE "+where+" LIMIT 1", args...)
	if err != nil {
		return Post{}, err
	}
	posts, err := scan(res)
	if err != nil {
		return Post{}, err
	}
	if len(posts) == 0 {
		return Post{}, content.NotFound(Kind, strings.TrimSpace(where))
	}
	return posts[0], nil
}

// Get loads a post by id, any status.
func (r *Repo) Get(ctx context.Context, id string) (Post, error) {
	return r.one(ctx, "e.id = ?", id)
}

// GetPublished loads a public post by language and slug, for public pages.
func (r *Repo) GetPublished(ctx context.Context, lang, slug string) (Post, error) {
	return r.one(ctx, "e.lang = ? AND e.slug = ? AND e.status = 'published' AND e.visibility <> 'private'", lang, slug)
}

// ListFilter selects posts for lists.
type ListFilter struct {
	Lang   string
	Status string // empty for every status (admin)
	Tag    string // tag slug, optional
	Limit  int    // default 20, max 100
	Before *int64 // published_at cursor, exclusive
	Admin  bool   // every visibility, newest change first
}

// List returns posts newest first.
func (r *Repo) List(ctx context.Context, f ListFilter) ([]Post, error) {
	var where []string
	var args []any
	if f.Lang != "" {
		where = append(where, "e.lang = ?")
		args = append(args, f.Lang)
	}
	if f.Status != "" {
		where = append(where, "e.status = ?")
		args = append(args, f.Status)
		if f.Status == content.Published && !f.Admin {
			where = append(where, "e.visibility = 'public'")
		}
	}
	if f.Tag != "" {
		where = append(where, "EXISTS (SELECT 1 FROM entry_tags et JOIN tags t ON t.id = et.tag_id WHERE et.entry_id = e.id AND t.slug = ?)")
		args = append(args, f.Tag)
	}
	if f.Before != nil {
		where = append(where, "e.published_at < ?")
		args = append(args, *f.Before)
	}
	limit := f.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	q := selectPost
	if len(where) > 0 {
		q += " WHERE " + strings.Join(where, " AND ")
	}
	if f.Admin {
		q += " ORDER BY e.updated_at DESC, e.id DESC LIMIT ?"
	} else {
		q += " ORDER BY COALESCE(e.published_at, e.updated_at) DESC, e.id DESC LIMIT ?"
	}
	args = append(args, limit)
	res, err := r.db.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	return scan(res)
}

// Archive takes a post off the site but keeps it: lists, feeds and its URL
// stop showing it (the URL answers 404 until it is published again).
func (r *Repo) Archive(ctx context.Context, id string) error {
	res, err := r.db.Query(ctx, `UPDATE entries SET status = 'archived', updated_at = ? WHERE id = ? AND kind = 'post'`, content.Clock().Unix(), id)
	if err != nil {
		return err
	}
	if res.Changes == 0 {
		return content.NotFound(Kind, id)
	}
	return nil
}

// Delete removes a post; revisions, tags and the posts row cascade.
// Backlink checks and redirects come with the editor (Phase 1.6).
func (r *Repo) Delete(ctx context.Context, id string) error {
	res, err := r.db.Query(ctx, `DELETE FROM entries WHERE id = ? AND kind = 'post'`, id)
	if err != nil {
		return err
	}
	if res.Changes == 0 {
		return content.NotFound(Kind, id)
	}
	return nil
}

// row mirrors a post row; D1 returns booleans as 0/1.
type row struct {
	content.Entry
	ReadingMinutes  int    `json:"reading_minutes"`
	CanonicalURL    string `json:"canonical_url"`
	Mermaid         int    `json:"mermaid"`
	CommentsEnabled int    `json:"comments_enabled"`
}

func scan(res store.Result) ([]Post, error) {
	rows, err := store.Scan[row](res)
	if err != nil {
		return nil, err
	}
	out := make([]Post, len(rows))
	for i, r := range rows {
		out[i], _ = scanOne(r)
	}
	return out, nil
}

func scanOne(r row) (Post, error) {
	return Post{Entry: r.Entry, ReadingMinutes: r.ReadingMinutes, CanonicalURL: r.CanonicalURL,
		Mermaid: r.Mermaid == 1, CommentsEnabled: r.CommentsEnabled == 1}, nil
}

// Published is a public post with its published body, for feeds.
type Published struct {
	Post
	HTML string
	Tags []string
}

type publishedRow struct {
	row
	BodyHTML string  `json:"body_html"`
	TagList  *string `json:"tag_list"`
}

// Feed returns the newest public posts in lang with the published body,
// optionally only those tagged tag.
func (r *Repo) Feed(ctx context.Context, lang, tag string, limit int) ([]Published, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	q := `SELECT ` + content.EntryColumns + `,
  p.reading_minutes, p.canonical_url, p.mermaid, p.comments_enabled, rv.body_html,
  (SELECT group_concat(t.slug, ' ') FROM entry_tags et JOIN tags t ON t.id = et.tag_id WHERE et.entry_id = e.id) AS tag_list
FROM entries e JOIN posts p ON p.entry_id = e.id JOIN revisions rv ON rv.id = e.published_revision_id
WHERE e.lang = ? AND e.status = 'published' AND e.visibility = 'public'`
	args := []any{lang}
	if tag != "" {
		q += ` AND EXISTS (SELECT 1 FROM entry_tags et JOIN tags t ON t.id = et.tag_id WHERE et.entry_id = e.id AND t.slug = ?)`
		args = append(args, tag)
	}
	q += ` ORDER BY e.published_at DESC, e.id DESC LIMIT ?`
	args = append(args, limit)
	res, err := r.db.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	rows, err := store.Scan[publishedRow](res)
	if err != nil {
		return nil, err
	}
	out := make([]Published, len(rows))
	for i, x := range rows {
		ps, _ := scanOne(x.row)
		out[i] = Published{Post: ps, HTML: x.BodyHTML}
		if x.TagList != nil && *x.TagList != "" {
			out[i].Tags = strings.Fields(*x.TagList)
		}
	}
	return out, nil
}

// Translation is a published sibling of a post in another language.
type Translation struct {
	Lang string `json:"lang"`
	Slug string `json:"slug"`
}

// Translations lists the published, non-private siblings of p.
func (r *Repo) Translations(ctx context.Context, p Post) ([]Translation, error) {
	res, err := r.db.Query(ctx, `SELECT lang, slug FROM entries
WHERE translation_group_id = ? AND id <> ? AND kind = 'post' AND status = 'published' AND visibility <> 'private'
ORDER BY lang`, p.TranslationGroupID, p.ID)
	if err != nil {
		return nil, err
	}
	return store.Scan[Translation](res)
}

// SeriesNav is a post's place in its series, in the post's language.
type SeriesNav struct {
	Slug  string
	Title string
	Index int // 1-based
	Total int
	Prev  *Post
	Next  *Post
}

// Series returns p's series navigation, or nil when p is not in a series.
func (r *Repo) Series(ctx context.Context, p Post) (*SeriesNav, error) {
	if p.SeriesID == nil {
		return nil, nil
	}
	res, err := r.db.Query(ctx, `SELECT slug, title_tr, title_en FROM series WHERE id = ?`, *p.SeriesID)
	if err != nil {
		return nil, err
	}
	meta, err := store.Scan[struct {
		Slug    string `json:"slug"`
		TitleTR string `json:"title_tr"`
		TitleEN string `json:"title_en"`
	}](res)
	if err != nil || len(meta) == 0 {
		return nil, err
	}
	res, err = r.db.Query(ctx, selectPost+` WHERE e.series_id = ? AND e.lang = ? AND e.status = 'published' AND e.visibility = 'public'
ORDER BY COALESCE(e.series_order, 1000000), e.published_at`, *p.SeriesID, p.Lang)
	if err != nil {
		return nil, err
	}
	parts, err := scan(res)
	if err != nil {
		return nil, err
	}
	nav := &SeriesNav{Slug: meta[0].Slug, Title: meta[0].TitleEN, Total: len(parts)}
	if p.Lang == "tr" && meta[0].TitleTR != "" {
		nav.Title = meta[0].TitleTR
	}
	for i := range parts {
		if parts[i].ID != p.ID {
			continue
		}
		nav.Index = i + 1
		if i > 0 {
			nav.Prev = &parts[i-1]
		}
		if i+1 < len(parts) {
			nav.Next = &parts[i+1]
		}
	}
	if nav.Index == 0 {
		return nil, nil
	}
	return nav, nil
}

// PublishedBody returns the HTML and TOC of p's published revision.
func (r *Repo) PublishedBody(ctx context.Context, p Post) (content.Revision, error) {
	if p.PublishedRevisionID == nil {
		return content.Revision{}, content.NotFound("revision", p.ID)
	}
	return content.GetRevision(ctx, r.db, *p.PublishedRevisionID)
}

// TagNames maps tag slugs to their display names in lang.
func (r *Repo) TagNames(ctx context.Context, lang string, slugs []string) (map[string]string, error) {
	out := map[string]string{}
	if len(slugs) == 0 {
		return out, nil
	}
	ph := strings.TrimSuffix(strings.Repeat("?,", len(slugs)), ",")
	args := make([]any, len(slugs))
	for i, s := range slugs {
		args[i] = s
	}
	res, err := r.db.Query(ctx, `SELECT id, slug, name_tr, name_en, tone FROM tags WHERE slug IN (`+ph+`)`, args...)
	if err != nil {
		return nil, err
	}
	tags, err := store.Scan[content.Tag](res)
	if err != nil {
		return nil, err
	}
	for _, t := range tags {
		out[t.Slug] = t.Name(lang)
	}
	return out, nil
}
