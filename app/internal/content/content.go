// Package content holds what every kind shares: the entries row, revisions
// with autosave coalescing (ADR-006), tags, and the validation rules of
// CONTENT_MODEL.md §6. Kind packages (content/post, ...) add their own table
// and call into this one. All I/O goes through store.DB.
package content

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/oklog/ulid/v2"

	"halid.org/engine/internal/store"
)

// Statuses, visibilities, revision origins.
const (
	Draft     = "draft"
	Scheduled = "scheduled"
	Published = "published"
	Archived  = "archived"

	Public   = "public"
	Unlisted = "unlisted"
	Private  = "private"

	OriginAutosave = "autosave"
	OriginManual   = "manual"
	OriginPublish  = "publish"
	OriginImport   = "import"
)

// ErrNotFound is returned when an entry does not exist.
var ErrNotFound = errors.New("content: not found")

// Clock is swapped in tests.
var Clock = func() time.Time { return time.Now() }

// NewID returns a ULID. ulid.Make is monotonic within a process, so ids
// sort by creation even inside one second.
func NewID() string { return ulid.Make().String() }

// Entry is a row of entries.
type Entry struct {
	ID                  string  `json:"id"`
	Kind                string  `json:"kind"`
	Slug                string  `json:"slug"`
	Lang                string  `json:"lang"`
	TranslationGroupID  string  `json:"translation_group_id"`
	Title               string  `json:"title"`
	Summary             string  `json:"summary"`
	Status              string  `json:"status"`
	Visibility          string  `json:"visibility"`
	PublishedAt         *int64  `json:"published_at"`
	CreatedAt           int64   `json:"created_at"`
	UpdatedAt           int64   `json:"updated_at"`
	CurrentRevisionID   *string `json:"current_revision_id"`
	PublishedRevisionID *string `json:"published_revision_id"`
	CoverMediaID        *string `json:"cover_media_id"`
	Tone                *string `json:"tone"`
	SeriesID            *string `json:"series_id"`
	SeriesOrder         *int64  `json:"series_order"`
}

// EntryColumns is the column list matching Entry, for SELECTs that alias
// the table as e.
const EntryColumns = `e.id, e.kind, e.slug, e.lang, e.translation_group_id, e.title, e.summary,
  e.status, e.visibility, e.published_at, e.created_at, e.updated_at,
  e.current_revision_id, e.published_revision_id, e.cover_media_id, e.tone,
  e.series_id, e.series_order`

// Revision is a row of revisions.
type Revision struct {
	ID        string `json:"id"`
	EntryID   string `json:"entry_id"`
	BodyMD    string `json:"body_md"`
	BodyHTML  string `json:"body_html"`
	TOCJSON   string `json:"toc_json"`
	DataJSON  string `json:"data_json"`
	Origin    string `json:"origin"`
	CreatedAt int64  `json:"created_at"`
	Note      string `json:"note"`
}

// Body is what a save writes into a revision. The caller renders it.
type Body struct {
	MD       string
	HTML     string
	TOCJSON  string // "[]" when empty
	DataJSON string // "{}" when empty
	Note     string
}

func (b Body) normalized() Body {
	if b.TOCJSON == "" {
		b.TOCJSON = "[]"
	}
	if b.DataJSON == "" {
		b.DataJSON = "{}"
	}
	return b
}

// --- Validation -------------------------------------------------------------

// ValidationError lists every field problem at once, for the editor.
type ValidationError map[string]string

func (v ValidationError) Error() string {
	parts := make([]string, 0, len(v))
	for f, m := range v {
		parts = append(parts, f+": "+m)
	}
	return "content: invalid: " + strings.Join(parts, "; ")
}

var slugRE = regexp.MustCompile(`^[a-z0-9-]{1,80}$`)

var tones = map[string]bool{"cyan": true, "indigo": true, "amber": true, "green": true, "orange": true, "rose": true, "violet": true, "navy": true}

// ValidSlug reports whether s is a valid slug (CONTENT_MODEL.md §6).
func ValidSlug(s string) bool { return slugRE.MatchString(s) }

// Validate checks the shared rules; kinds add theirs to the returned map.
// titleOptional is true for kinds like note that may publish untitled.
func Validate(e Entry, titleOptional bool) ValidationError {
	v := ValidationError{}
	if !ValidSlug(e.Slug) {
		v["slug"] = "lowercase letters, digits and hyphens, 1 to 80 characters"
	}
	if e.Lang != "tr" && e.Lang != "en" {
		v["lang"] = "must be tr or en"
	}
	switch e.Status {
	case Draft, Scheduled, Published, Archived:
	default:
		v["status"] = "unknown status"
	}
	switch e.Visibility {
	case Public, Unlisted, Private:
	default:
		v["visibility"] = "unknown visibility"
	}
	if e.Status == Published || e.Status == Scheduled {
		if !titleOptional && strings.TrimSpace(e.Title) == "" {
			v["title"] = "required to publish"
		}
		if e.PublishedAt == nil {
			v["published_at"] = "required to publish"
		}
	}
	if utf8.RuneCountInString(e.Summary) > 160 {
		v["summary"] = "at most 160 characters (SEO description)"
	}
	if e.Tone != nil && !tones[*e.Tone] {
		v["tone"] = "unknown tone"
	}
	return v
}

// --- Writes -----------------------------------------------------------------

// InsertEntry returns the statements that create an entry and its first
// revision. Kinds append their own row and run everything in one Batch.
func InsertEntry(e *Entry, b Body, origin string) []store.Stmt {
	now := Clock().Unix()
	if e.ID == "" {
		e.ID = NewID()
	}
	if e.TranslationGroupID == "" {
		e.TranslationGroupID = NewID()
	}
	e.CreatedAt, e.UpdatedAt = now, now
	rev := NewID()
	e.CurrentRevisionID = &rev
	b = b.normalized()
	return []store.Stmt{
		{SQL: `INSERT INTO entries (id, kind, slug, lang, translation_group_id, title, summary, status,
  visibility, published_at, created_at, updated_at, cover_media_id, tone, series_id, series_order)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			Args: []any{e.ID, e.Kind, e.Slug, e.Lang, e.TranslationGroupID, e.Title, e.Summary, e.Status,
				e.Visibility, e.PublishedAt, now, now, e.CoverMediaID, e.Tone, e.SeriesID, e.SeriesOrder}},
		insertRevision(rev, e.ID, b, origin, now),
		{SQL: `UPDATE entries SET current_revision_id = ? WHERE id = ?`, Args: []any{rev, e.ID}},
	}
}

func insertRevision(id, entryID string, b Body, origin string, now int64) store.Stmt {
	return store.Stmt{
		SQL: `INSERT INTO revisions (id, entry_id, body_md, body_html, toc_json, data_json, origin, created_at, note)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		Args: []any{id, entryID, b.MD, b.HTML, b.TOCJSON, b.DataJSON, origin, now, b.Note},
	}
}

// UpdateEntry returns the statement that writes the editable entry fields.
func UpdateEntry(e *Entry) store.Stmt {
	e.UpdatedAt = Clock().Unix()
	return store.Stmt{
		SQL: `UPDATE entries SET slug = ?, title = ?, summary = ?, status = ?, visibility = ?, published_at = ?,
  cover_media_id = ?, tone = ?, series_id = ?, series_order = ?, updated_at = ? WHERE id = ?`,
		Args: []any{e.Slug, e.Title, e.Summary, e.Status, e.Visibility, e.PublishedAt,
			e.CoverMediaID, e.Tone, e.SeriesID, e.SeriesOrder, e.UpdatedAt, e.ID},
	}
}

// SaveRevision writes a body. Consecutive autosaves coalesce: when both
// this save and the newest revision are autosaves, that revision is
// rewritten in place. Every other save inserts a new revision, so an
// explicit save always leaves a restorable point (ADR-006). It returns the
// id of the revision that now holds the body.
func SaveRevision(ctx context.Context, db store.DB, entryID string, b Body, origin string) (string, error) {
	b = b.normalized()
	now := Clock().Unix()
	if origin == OriginAutosave {
		res, err := db.Query(ctx, `SELECT id, origin FROM revisions WHERE entry_id = ? ORDER BY id DESC LIMIT 1`, entryID)
		if err != nil {
			return "", err
		}
		last, err := store.Scan[struct {
			ID     string `json:"id"`
			Origin string `json:"origin"`
		}](res)
		if err != nil {
			return "", err
		}
		if len(last) == 1 && last[0].Origin == OriginAutosave {
			_, err := db.Batch(ctx,
				store.Stmt{SQL: `UPDATE revisions SET body_md = ?, body_html = ?, toc_json = ?, data_json = ?, created_at = ? WHERE id = ?`,
					Args: []any{b.MD, b.HTML, b.TOCJSON, b.DataJSON, now, last[0].ID}},
				store.Stmt{SQL: `UPDATE entries SET current_revision_id = ?, updated_at = ? WHERE id = ?`,
					Args: []any{last[0].ID, now, entryID}},
			)
			return last[0].ID, err
		}
	}
	id := NewID()
	stmts := []store.Stmt{
		insertRevision(id, entryID, b, origin, now),
		{SQL: `UPDATE entries SET current_revision_id = ?, updated_at = ? WHERE id = ?`, Args: []any{id, now, entryID}},
	}
	res, err := db.Batch(ctx, stmts...)
	if err != nil {
		return "", err
	}
	if res[1].Changes == 0 {
		return "", ErrNotFound
	}
	return id, nil
}

// PublishStmts move the published pointer to revID and set the status.
// Publishing renders and purges in jobs (Phase 1.3); this is the row change.
func PublishStmts(entryID, revID string, publishedAt int64) []store.Stmt {
	now := Clock().Unix()
	return []store.Stmt{{
		SQL: `UPDATE entries SET status = 'published', published_revision_id = ?,
  published_at = COALESCE(published_at, ?), updated_at = ? WHERE id = ?`,
		Args: []any{revID, publishedAt, now, entryID},
	}}
}

// Revisions lists an entry's revisions, newest first, without bodies.
func Revisions(ctx context.Context, db store.DB, entryID string, limit int) ([]Revision, error) {
	if limit <= 0 {
		limit = 50
	}
	res, err := db.Query(ctx, `SELECT id, entry_id, '' AS body_md, '' AS body_html, '[]' AS toc_json, '{}' AS data_json,
  origin, created_at, note FROM revisions WHERE entry_id = ? ORDER BY id DESC LIMIT ?`, entryID, limit)
	if err != nil {
		return nil, err
	}
	return store.Scan[Revision](res)
}

// GetRevision loads one revision with its body.
func GetRevision(ctx context.Context, db store.DB, id string) (Revision, error) {
	res, err := db.Query(ctx, `SELECT id, entry_id, body_md, body_html, toc_json, data_json, origin, created_at, note
FROM revisions WHERE id = ?`, id)
	if err != nil {
		return Revision{}, err
	}
	revs, err := store.Scan[Revision](res)
	if err != nil {
		return Revision{}, err
	}
	if len(revs) == 0 {
		return Revision{}, ErrNotFound
	}
	return revs[0], nil
}

// --- Tags -------------------------------------------------------------------

// NormalizeTag turns editor input into a slug, or "" when nothing is left.
func NormalizeTag(s string) string {
	s = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(s), "#")))
	s = strings.NewReplacer(" ", "-", "_", "-").Replace(s)
	if !ValidSlug(s) {
		return ""
	}
	return s
}

// SetTagsStmts replace an entry's tags. Unknown slugs are created with the
// slug as both names; the tag admin (Phase 2) renames them.
func SetTagsStmts(entryID string, slugs []string) []store.Stmt {
	stmts := []store.Stmt{{SQL: `DELETE FROM entry_tags WHERE entry_id = ?`, Args: []any{entryID}}}
	seen := map[string]bool{}
	for _, raw := range slugs {
		s := NormalizeTag(raw)
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		stmts = append(stmts,
			store.Stmt{SQL: `INSERT INTO tags (id, slug, name_tr, name_en) VALUES (?, ?, ?, ?) ON CONFLICT (slug) DO NOTHING`,
				Args: []any{NewID(), s, s, s}},
			store.Stmt{SQL: `INSERT INTO entry_tags (entry_id, tag_id) SELECT ?, id FROM tags WHERE slug = ?`,
				Args: []any{entryID, s}},
		)
	}
	return stmts
}

// Tag is a row of tags.
type Tag struct {
	ID     string  `json:"id"`
	Slug   string  `json:"slug"`
	NameTR string  `json:"name_tr"`
	NameEN string  `json:"name_en"`
	Tone   *string `json:"tone"`
}

// Name is the tag's label in lang.
func (t Tag) Name(lang string) string {
	if lang == "tr" && t.NameTR != "" {
		return t.NameTR
	}
	if t.NameEN != "" {
		return t.NameEN
	}
	return t.Slug
}

// TagsOf returns the tags of one entry, by slug.
func TagsOf(ctx context.Context, db store.DB, entryID string) ([]Tag, error) {
	res, err := db.Query(ctx, `SELECT t.id, t.slug, t.name_tr, t.name_en, t.tone FROM tags t
JOIN entry_tags et ON et.tag_id = t.id WHERE et.entry_id = ? ORDER BY t.slug`, entryID)
	if err != nil {
		return nil, err
	}
	return store.Scan[Tag](res)
}

// IsConstraint reports whether err is a uniqueness or check failure from
// D1 or SQLite, so handlers can show a field error instead of a 500.
func IsConstraint(err error) bool {
	if err == nil {
		return false
	}
	m := strings.ToLower(err.Error())
	return strings.Contains(m, "unique constraint") || strings.Contains(m, "check constraint")
}

// NotFound builds the not-found error for a kind lookup; errors.Is matches
// ErrNotFound.
func NotFound(kind, key string) error { return fmt.Errorf("%w: %s %s", ErrNotFound, kind, key) }

// slugFold maps letters with diacritics (Turkish first) to ASCII for slugs.
var slugFold = strings.NewReplacer(
	"ı", "i", "İ", "i", "ğ", "g", "Ğ", "g", "ü", "u", "Ü", "u", "ş", "s", "Ş", "s", "ö", "o", "Ö", "o", "ç", "c", "Ç", "c",
	"â", "a", "Â", "a", "î", "i", "Î", "i", "û", "u", "Û", "u", "é", "e", "è", "e", "ê", "e", "ë", "e", "á", "a", "à", "a",
	"ä", "a", "ó", "o", "ò", "o", "ú", "u", "ù", "u", "ñ", "n", "ß", "ss",
	"'", "", "’", "", // apostrophes join: Ghost'tan -> ghosttan
)

// Slugify derives a slug from a title: ASCII, lowercase, hyphens, at most
// 80 characters, or "" when nothing usable is left.
func Slugify(title string) string {
	s := strings.ToLower(slugFold.Replace(title))
	var b strings.Builder
	dash := false
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			dash = false
		case b.Len() > 0 && !dash:
			b.WriteByte('-')
			dash = true
		}
	}
	out := strings.TrimRight(b.String(), "-")
	if len(out) > 80 {
		out = strings.TrimRight(out[:80], "-")
	}
	return out
}
