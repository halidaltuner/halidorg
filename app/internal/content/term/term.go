// Package term is the glossary kind (CONTENT_MODEL.md §2 term, Phase 5.5):
// a term with its other names and a short definition; the body explains
// it. Terms live at /{lang}/glossary/{slug}, listed A to Z.
package term

import (
	"context"
	"encoding/json"
	"strings"
	"unicode/utf8"

	"halid.org/engine/internal/content"
	"halid.org/engine/internal/render"
	"halid.org/engine/internal/store"
)

// Kind is the entries.kind value.
const Kind = "term"

// Term is an entry plus the terms row. Entry.Title is the term.
type Term struct {
	content.Entry
	Aliases  []string `json:"-"`
	ShortDef string   `json:"short_def"`
}

// Draft is the input for Create and Save.
type Draft struct {
	Term
	Markdown string
	Tags     []string
}

// Validate returns nil or a content.ValidationError with every problem. A
// term needs its title, unlike notes.
func Validate(t Term) error {
	v := content.Validate(t.Entry, false)
	if t.Kind != Kind {
		v["kind"] = "must be term"
	}
	if strings.TrimSpace(t.Title) == "" {
		v["title"] = "the term"
	}
	if utf8.RuneCountInString(t.ShortDef) > 300 {
		v["short_def"] = "at most 300 characters"
	}
	if len(t.Aliases) > 10 {
		v["aliases"] = "at most 10"
	}
	for _, a := range t.Aliases {
		if utf8.RuneCountInString(a) > 60 {
			v["aliases"] = "each at most 60 characters"
		}
	}
	if len(v) == 0 {
		return nil
	}
	return v
}

// Repo reads and writes terms.
type Repo struct{ db store.DB }

// NewRepo returns a repository over db.
func NewRepo(db store.DB) *Repo { return &Repo{db: db} }

const selectTerm = `SELECT ` + content.EntryColumns + `, t.aliases_json, t.short_def FROM entries e JOIN terms t ON t.entry_id = e.id`

type row struct {
	content.Entry
	Aliases  string `json:"aliases_json"`
	ShortDef string `json:"short_def"`
}

func (r row) term() Term {
	t := Term{Entry: r.Entry, ShortDef: r.ShortDef}
	_ = json.Unmarshal([]byte(r.Aliases), &t.Aliases)
	return t
}

// normalize trims the fields and drops empty and repeated aliases.
func normalize(d *Draft) {
	d.Kind = Kind
	d.ShortDef = strings.TrimSpace(d.ShortDef)
	seen := map[string]bool{}
	var out []string
	for _, a := range d.Aliases {
		a = strings.TrimSpace(a)
		if k := strings.ToLower(a); a != "" && !seen[k] && k != strings.ToLower(d.Title) {
			seen[k] = true
			out = append(out, a)
		}
	}
	d.Aliases = out
}

func ownRow(id string, t Term) store.Stmt {
	aliases, _ := json.Marshal(append([]string{}, t.Aliases...))
	return store.Stmt{SQL: `INSERT INTO terms (entry_id, term, aliases_json, short_def) VALUES (?, ?, ?, ?)
ON CONFLICT (entry_id) DO UPDATE SET term = excluded.term, aliases_json = excluded.aliases_json, short_def = excluded.short_def`,
		Args: []any{id, strings.TrimSpace(t.Title), string(aliases), t.ShortDef}}
}

func body(d Draft) (content.Body, error) {
	out, err := render.Markdown([]byte(d.Markdown), render.Options{Lang: d.Lang})
	if err != nil {
		return content.Body{}, err
	}
	return content.Body{MD: d.Markdown, HTML: out.HTML, TOCJSON: out.TOCJSON()}, nil
}

// Create validates and inserts a draft term.
func (r *Repo) Create(ctx context.Context, d Draft) (Term, error) {
	normalize(&d)
	if d.Status == "" || d.Status == content.Published {
		d.Status = content.Draft
	}
	if d.Visibility == "" {
		d.Visibility = content.Public
	}
	if err := Validate(d.Term); err != nil {
		return Term{}, err
	}
	b, err := body(d)
	if err != nil {
		return Term{}, err
	}
	err = content.CreateEntry(ctx, r.db, &d.Entry, b, d.Tags, func(id string) []store.Stmt { return []store.Stmt{ownRow(id, d.Term)} })
	return d.Term, err
}

// Save writes the fields and a revision.
func (r *Repo) Save(ctx context.Context, d Draft, origin string) (Term, error) {
	normalize(&d)
	cur, err := r.Get(ctx, d.ID)
	if err != nil {
		return Term{}, err
	}
	if err := Validate(d.Term); err != nil {
		return Term{}, err
	}
	b, err := body(d)
	if err != nil {
		return Term{}, err
	}
	if _, err := content.SaveEntry(ctx, r.db, cur.Entry, &d.Entry, b, origin, d.Tags, []store.Stmt{ownRow(d.ID, d.Term)}); err != nil {
		return Term{}, err
	}
	return r.Get(ctx, d.ID)
}

// Publish makes the term public.
func (r *Repo) Publish(ctx context.Context, id string) (Term, error) {
	t, err := r.Get(ctx, id)
	if err != nil {
		return Term{}, err
	}
	err = content.PublishEntry(ctx, r.db, t.Entry, func(e content.Entry) error {
		x := t
		x.Entry = e
		return Validate(x)
	})
	if err != nil {
		return Term{}, err
	}
	return r.Get(ctx, id)
}

// Get loads a term by id, any status.
func (r *Repo) Get(ctx context.Context, id string) (Term, error) {
	return r.one(ctx, "e.id = ?", id)
}

// GetPublished loads a public term by language and slug.
func (r *Repo) GetPublished(ctx context.Context, lang, slug string) (Term, error) {
	return r.one(ctx, "e.lang = ? AND e.slug = ? AND e.status = 'published' AND e.visibility <> 'private'", lang, slug)
}

func (r *Repo) one(ctx context.Context, where string, args ...any) (Term, error) {
	res, err := r.db.Query(ctx, selectTerm+" WHERE "+where+" LIMIT 1", args...)
	if err != nil {
		return Term{}, err
	}
	rows, err := store.Scan[row](res)
	if err != nil {
		return Term{}, err
	}
	if len(rows) == 0 {
		return Term{}, content.NotFound(Kind, where)
	}
	return rows[0].term(), nil
}

// List returns the published public terms in lang, A to Z.
func (r *Repo) List(ctx context.Context, lang string) ([]Term, error) {
	res, err := r.db.Query(ctx, selectTerm+` WHERE e.lang = ? AND e.status = 'published' AND e.visibility = 'public'
ORDER BY lower(t.term), e.id LIMIT 1000`, lang)
	if err != nil {
		return nil, err
	}
	rows, err := store.Scan[row](res)
	if err != nil {
		return nil, err
	}
	out := make([]Term, len(rows))
	for i, x := range rows {
		out[i] = x.term()
	}
	return out, nil
}
