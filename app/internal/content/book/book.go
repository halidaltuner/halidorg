// Package book is the reading list kind (CONTENT_MODEL.md §2 book, Phase
// 5.5): a book with its author, status, progress and rating. Books have no
// page of their own; they live on /{lang}/reading, and the ones being read
// show on Now.
package book

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"halid.org/engine/internal/content"
	"halid.org/engine/internal/render"
	"halid.org/engine/internal/store"
)

// Kind is the entries.kind value.
const Kind = "book"

// Statuses in the reading page's order.
var Statuses = []string{"reading", "want", "done", "dropped"}

// Book is an entry plus the books row. Entry.Title is the book's title,
// Entry.Summary a short take, Entry.CoverMediaID its cover.
type Book struct {
	content.Entry
	Author     string `json:"author"`
	ISBN       string `json:"isbn"`
	State      string `json:"book_status"` // reading, want, done, dropped
	Progress   int    `json:"progress_pct"`
	StartedAt  *int64 `json:"started_at"`
	FinishedAt *int64 `json:"finished_at"`
	Rating     *int   `json:"rating"`
}

// Draft is the input for Create and Save; Markdown is longer notes.
type Draft struct {
	Book
	Markdown string
	Tags     []string
}

// Validate returns nil or a content.ValidationError with every problem.
func Validate(b Book) error {
	v := content.Validate(b.Entry, false)
	if b.Kind != Kind {
		v["kind"] = "must be book"
	}
	if strings.TrimSpace(b.Title) == "" {
		v["title"] = "the book's title"
	}
	if !slices.Contains(Statuses, b.State) {
		v["book_status"] = "reading, want, done or dropped"
	}
	if b.Progress < 0 || b.Progress > 100 {
		v["progress_pct"] = "0 to 100"
	}
	if b.Rating != nil && (*b.Rating < 1 || *b.Rating > 5) {
		v["rating"] = "1 to 5"
	}
	for f, s := range map[string]struct {
		val string
		max int
	}{"author": {b.Author, 120}, "isbn": {b.ISBN, 20}} {
		if utf8.RuneCountInString(s.val) > s.max {
			v[f] = fmt.Sprintf("at most %d characters", s.max)
		}
	}
	for f, t := range map[string]*int64{"started_at": b.StartedAt, "finished_at": b.FinishedAt} {
		if t != nil && *t <= 0 {
			v[f] = "a date"
		}
	}
	if b.StartedAt != nil && b.FinishedAt != nil && *b.FinishedAt > 0 && *b.FinishedAt < *b.StartedAt {
		v["finished_at"] = "before it was started"
	}
	if len(v) == 0 {
		return nil
	}
	return v
}

// Repo reads and writes books.
type Repo struct{ db store.DB }

// NewRepo returns a repository over db.
func NewRepo(db store.DB) *Repo { return &Repo{db: db} }

const selectBook = `SELECT ` + content.EntryColumns + `, b.author, b.isbn, b.status AS book_status, b.progress_pct,
  b.started_at, b.finished_at, b.rating FROM entries e JOIN books b ON b.entry_id = e.id`

// normalize trims the fields; a finished book is at 100%.
func normalize(d *Draft) {
	d.Kind = Kind
	d.Author, d.ISBN = strings.TrimSpace(d.Author), strings.TrimSpace(d.ISBN)
	if d.State == "" {
		d.State = "want"
	}
	if d.State == "done" {
		d.Progress = 100
	}
}

func ownRow(id string, b Book) store.Stmt {
	return store.Stmt{SQL: `INSERT INTO books (entry_id, author, isbn, status, progress_pct, started_at, finished_at, rating)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (entry_id) DO UPDATE SET author = excluded.author, isbn = excluded.isbn, status = excluded.status,
  progress_pct = excluded.progress_pct, started_at = excluded.started_at, finished_at = excluded.finished_at, rating = excluded.rating`,
		Args: []any{id, b.Author, b.ISBN, b.State, b.Progress, b.StartedAt, b.FinishedAt, b.Rating}}
}

func body(d Draft) (content.Body, error) {
	out, err := render.Markdown([]byte(d.Markdown), render.Options{Lang: d.Lang})
	if err != nil {
		return content.Body{}, err
	}
	return content.Body{MD: d.Markdown, HTML: out.HTML}, nil
}

// check validates b and its cover: any image from media.
func (r *Repo) check(ctx context.Context, b Book) error {
	if err := Validate(b); err != nil {
		return err
	}
	if b.CoverMediaID == nil {
		return nil
	}
	res, err := r.db.Query(ctx, `SELECT COUNT(*) AS n FROM media WHERE id = ? AND mime LIKE 'image/%'`, *b.CoverMediaID)
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

// Create validates and inserts a draft book.
func (r *Repo) Create(ctx context.Context, d Draft) (Book, error) {
	normalize(&d)
	if d.Status == "" || d.Status == content.Published {
		d.Status = content.Draft
	}
	if d.Visibility == "" {
		d.Visibility = content.Public
	}
	if err := r.check(ctx, d.Book); err != nil {
		return Book{}, err
	}
	b, err := body(d)
	if err != nil {
		return Book{}, err
	}
	err = content.CreateEntry(ctx, r.db, &d.Entry, b, d.Tags, func(id string) []store.Stmt { return []store.Stmt{ownRow(id, d.Book)} })
	return d.Book, err
}

// Save writes the fields and a revision.
func (r *Repo) Save(ctx context.Context, d Draft, origin string) (Book, error) {
	normalize(&d)
	cur, err := r.Get(ctx, d.ID)
	if err != nil {
		return Book{}, err
	}
	if err := r.check(ctx, d.Book); err != nil {
		return Book{}, err
	}
	b, err := body(d)
	if err != nil {
		return Book{}, err
	}
	if _, err := content.SaveEntry(ctx, r.db, cur.Entry, &d.Entry, b, origin, d.Tags, []store.Stmt{ownRow(d.ID, d.Book)}); err != nil {
		return Book{}, err
	}
	return r.Get(ctx, d.ID)
}

// Publish makes the book public.
func (r *Repo) Publish(ctx context.Context, id string) (Book, error) {
	b, err := r.Get(ctx, id)
	if err != nil {
		return Book{}, err
	}
	err = content.PublishEntry(ctx, r.db, b.Entry, func(e content.Entry) error {
		x := b
		x.Entry = e
		return Validate(x)
	})
	if err != nil {
		return Book{}, err
	}
	return r.Get(ctx, id)
}

// Get loads a book by id, any status.
func (r *Repo) Get(ctx context.Context, id string) (Book, error) {
	res, err := r.db.Query(ctx, selectBook+" WHERE e.id = ?", id)
	if err != nil {
		return Book{}, err
	}
	rows, err := store.Scan[Book](res)
	if err != nil {
		return Book{}, err
	}
	if len(rows) == 0 {
		return Book{}, content.NotFound(Kind, id)
	}
	return rows[0], nil
}

// List returns the published public books in lang with one of the
// statuses (all when none): the ones being read by progress, the rest
// newest first.
func (r *Repo) List(ctx context.Context, lang string, statuses ...string) ([]Book, error) {
	q := selectBook + ` WHERE ` + content.LangOrFallback("e") + ` AND e.status = 'published' AND e.visibility = 'public'`
	args := []any{lang, lang}
	if len(statuses) > 0 {
		q += ` AND b.status IN (?` + strings.Repeat(", ?", len(statuses)-1) + `)`
		for _, s := range statuses {
			args = append(args, s)
		}
	}
	q += ` ORDER BY b.progress_pct DESC, coalesce(b.finished_at, b.started_at, e.published_at) DESC, e.id DESC LIMIT 500`
	res, err := r.db.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	return store.Scan[Book](res)
}
