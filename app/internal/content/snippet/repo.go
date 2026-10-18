package snippet

import (
	"context"
	"encoding/json"
	"path"
	"strings"

	"halid.org/engine/internal/content"
	"halid.org/engine/internal/render"
	"halid.org/engine/internal/store"
)

// Repo reads and writes snippets.
type Repo struct{ db store.DB }

// NewRepo returns a repository over db.
func NewRepo(db store.DB) *Repo { return &Repo{db: db} }

const selectSnippet = `SELECT ` + content.EntryColumns + `, s.category_id, s.gist_url
FROM entries e JOIN snippets s ON s.entry_id = e.id`

type row struct {
	content.Entry
	CategoryID *string `json:"category_id"`
	GistURL    *string `json:"gist_url"`
}

// languages maps file extensions to the chroma lexer names the renderer uses.
var languages = map[string]string{
	".go": "go", ".ts": "typescript", ".js": "javascript", ".py": "python", ".rs": "rust", ".rb": "ruby",
	".sh": "bash", ".bash": "bash", ".zsh": "bash", ".sql": "sql", ".html": "html", ".css": "css",
	".json": "json", ".yaml": "yaml", ".yml": "yaml", ".toml": "toml", ".md": "markdown", ".c": "c",
	".h": "c", ".cpp": "cpp", ".tf": "hcl", ".hcl": "hcl", ".lua": "lua", ".nix": "nix", ".java": "java",
	".kt": "kotlin", ".swift": "swift", ".php": "php", ".dockerfile": "docker", ".xml": "xml",
}

// Language guesses a file's language from its name when none is given.
func Language(filename string) string {
	if strings.EqualFold(filename, "Dockerfile") {
		return "docker"
	}
	if strings.EqualFold(filename, "Makefile") {
		return "make"
	}
	return languages[strings.ToLower(path.Ext(filename))]
}

// normalize trims names, fills languages and numbers the files in order.
func normalize(files []File) []File {
	out := make([]File, len(files))
	for i, f := range files {
		f.Filename = strings.TrimSpace(f.Filename)
		f.Code = strings.ReplaceAll(f.Code, "\r\n", "\n")
		if f.Language == "" {
			f.Language = Language(f.Filename)
		}
		f.Sort = i
		out[i] = f
	}
	return out
}

func ownRows(id string, s Snippet) []store.Stmt {
	stmts := []store.Stmt{
		{SQL: `INSERT INTO snippets (entry_id, category_id, gist_url) VALUES (?, ?, ?)
ON CONFLICT (entry_id) DO UPDATE SET category_id = excluded.category_id, gist_url = excluded.gist_url`,
			Args: []any{id, s.CategoryID, content.NullIfEmpty(s.GistURL)}},
		{SQL: `DELETE FROM snippet_files WHERE snippet_id = ?`, Args: []any{id}},
	}
	for _, f := range s.Files {
		stmts = append(stmts, store.Stmt{
			SQL:  `INSERT INTO snippet_files (snippet_id, filename, language, code, sort) VALUES (?, ?, ?, ?, ?)`,
			Args: []any{id, f.Filename, f.Language, f.Code, f.Sort},
		})
	}
	return stmts
}

type data struct {
	Files []File `json:"files"`
}

// body renders the description and snapshots the files into data_json, so
// a revision restores files and description together.
func body(d Draft) (content.Body, error) {
	out, err := render.Markdown([]byte(d.Markdown), render.Options{Lang: d.Lang})
	if err != nil {
		return content.Body{}, err
	}
	raw, err := json.Marshal(data{Files: d.Files})
	if err != nil {
		return content.Body{}, err
	}
	return content.Body{MD: d.Markdown, HTML: out.HTML, TOCJSON: out.TOCJSON(), DataJSON: string(raw)}, nil
}

// FilesOf reads the files a revision's data_json holds.
func FilesOf(dataJSON string) []File {
	var d data
	_ = json.Unmarshal([]byte(dataJSON), &d)
	return d.Files
}

func (r *Repo) check(ctx context.Context, s Snippet) error {
	if err := Validate(s); err != nil {
		return err
	}
	return content.CheckCategory(ctx, r.db, s.CategoryID, Kind)
}

// Create validates and inserts a draft snippet with its files.
func (r *Repo) Create(ctx context.Context, d Draft) (Snippet, error) {
	d.Kind = Kind
	if d.Status == "" || d.Status == content.Published {
		d.Status = content.Draft
	}
	if d.Visibility == "" {
		d.Visibility = content.Public
	}
	d.Files = normalize(d.Files)
	if err := r.check(ctx, d.Snippet); err != nil {
		return Snippet{}, err
	}
	b, err := body(d)
	if err != nil {
		return Snippet{}, err
	}
	err = content.CreateEntry(ctx, r.db, &d.Entry, b, d.Tags, func(id string) []store.Stmt { return ownRows(id, d.Snippet) })
	return d.Snippet, err
}

// Save writes the fields, replaces the files and adds a revision.
func (r *Repo) Save(ctx context.Context, d Draft, origin string) (Snippet, error) {
	d.Kind = Kind
	cur, err := r.Get(ctx, d.ID)
	if err != nil {
		return Snippet{}, err
	}
	d.Files = normalize(d.Files)
	if err := r.check(ctx, d.Snippet); err != nil {
		return Snippet{}, err
	}
	b, err := body(d)
	if err != nil {
		return Snippet{}, err
	}
	if _, err := content.SaveEntry(ctx, r.db, cur.Entry, &d.Entry, b, origin, d.Tags, ownRows(d.ID, d.Snippet)); err != nil {
		return Snippet{}, err
	}
	return r.Get(ctx, d.ID)
}

// Publish points the published revision at the current one.
func (r *Repo) Publish(ctx context.Context, id string) (Snippet, error) {
	s, err := r.Get(ctx, id)
	if err != nil {
		return Snippet{}, err
	}
	err = content.PublishEntry(ctx, r.db, s.Entry, func(e content.Entry) error {
		x := s
		x.Entry = e
		return Validate(x)
	})
	if err != nil {
		return Snippet{}, err
	}
	return r.Get(ctx, id)
}

// Get loads a snippet with its current files, any status.
func (r *Repo) Get(ctx context.Context, id string) (Snippet, error) {
	return r.one(ctx, "e.id = ?", id)
}

// GetPublished loads a public snippet by language and slug, with the files
// of its published revision (edits since then are not public yet).
func (r *Repo) GetPublished(ctx context.Context, lang, slug string) (Snippet, error) {
	s, err := r.one(ctx, "e.lang = ? AND e.slug = ? AND e.status = 'published' AND e.visibility <> 'private'", lang, slug)
	if err != nil || s.PublishedRevisionID == nil {
		return s, err
	}
	rev, err := content.GetRevision(ctx, r.db, *s.PublishedRevisionID)
	if err != nil {
		return Snippet{}, err
	}
	s.Files = FilesOf(rev.DataJSON)
	return s, nil
}

func (r *Repo) one(ctx context.Context, where string, args ...any) (Snippet, error) {
	res, err := r.db.Query(ctx, selectSnippet+" WHERE "+where+" LIMIT 1", args...)
	if err != nil {
		return Snippet{}, err
	}
	rows, err := store.Scan[row](res)
	if err != nil {
		return Snippet{}, err
	}
	if len(rows) == 0 {
		return Snippet{}, content.NotFound(Kind, where)
	}
	s := Snippet{Entry: rows[0].Entry, CategoryID: rows[0].CategoryID}
	if rows[0].GistURL != nil {
		s.GistURL = *rows[0].GistURL
	}
	res, err = r.db.Query(ctx, `SELECT filename, language, code, sort FROM snippet_files WHERE snippet_id = ? ORDER BY sort, filename`, s.ID)
	if err != nil {
		return Snippet{}, err
	}
	s.Files, err = store.Scan[File](res)
	return s, err
}

// Item is a public snippet in a list: the entry, its published files and
// tag slugs, and its rendered description.
type Item struct {
	Snippet
	HTML string
	Tags []string
}

type itemRow struct {
	row
	BodyHTML string  `json:"body_html"`
	DataJSON string  `json:"data_json"`
	TagList  *string `json:"tag_list"`
}

// Feed returns published public snippets in lang, newest first, with the
// files of their published revisions; tag filters when set.
func (r *Repo) Feed(ctx context.Context, lang, tag string, limit int) ([]Item, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	q := `SELECT ` + content.EntryColumns + `, s.category_id, s.gist_url, rv.body_html, rv.data_json,
  (SELECT group_concat(t.slug, ' ') FROM entry_tags et JOIN tags t ON t.id = et.tag_id WHERE et.entry_id = e.id) AS tag_list
FROM entries e JOIN snippets s ON s.entry_id = e.id JOIN revisions rv ON rv.id = e.published_revision_id
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
	rows, err := store.Scan[itemRow](res)
	if err != nil {
		return nil, err
	}
	out := make([]Item, len(rows))
	for i, x := range rows {
		s := Snippet{Entry: x.Entry, CategoryID: x.CategoryID, Files: FilesOf(x.DataJSON)}
		if x.GistURL != nil {
			s.GistURL = *x.GistURL
		}
		out[i] = Item{Snippet: s, HTML: x.BodyHTML}
		if x.TagList != nil {
			out[i].Tags = strings.Fields(*x.TagList)
		}
	}
	return out, nil
}

// PublishedHTML is the rendered description of a snippet's published revision.
func (r *Repo) PublishedHTML(ctx context.Context, s Snippet) (string, error) {
	if s.PublishedRevisionID == nil {
		return "", nil
	}
	rev, err := content.GetRevision(ctx, r.db, *s.PublishedRevisionID)
	return rev.BodyHTML, err
}

// Archive takes a snippet off the site.
func (r *Repo) Archive(ctx context.Context, id string) error {
	return content.ArchiveEntry(ctx, r.db, Kind, id)
}

// Delete removes a snippet and its files.
func (r *Repo) Delete(ctx context.Context, id string) error {
	return content.DeleteEntry(ctx, r.db, Kind, id)
}
