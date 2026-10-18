// Package media is the admin media library (Phase 1.7): images uploaded to
// R2 through store.Blob, with TR and EN alt text that must both be set
// before an image can be used (CONTENT_MODEL.md, media).
package media

import (
	"bytes"
	"context"
	"errors"
	"image"
	_ "image/gif"  // decoders for DecodeConfig
	_ "image/jpeg" //
	_ "image/png"  //
	"net/http"
	"path"
	"strings"

	_ "golang.org/x/image/webp" // WebP dimensions

	"halid.org/engine/internal/content"
	"halid.org/engine/internal/store"
)

// MaxBytes is the largest upload. R2 itself takes far more; this keeps the
// bridge request (Worker → R2) and admin memory small.
const MaxBytes = 10 << 20

// Item is a media row.
type Item struct {
	ID        string `json:"id"`
	R2Key     string `json:"r2_key"`
	Mime      string `json:"mime"`
	Width     *int   `json:"width"`
	Height    *int   `json:"height"`
	AltTR     string `json:"alt_tr"`
	AltEN     string `json:"alt_en"`
	CreatedAt int64  `json:"created_at"`
	Name      string `json:"name"`
	Bytes     int64  `json:"bytes"`
}

// Ready reports whether both alt texts are set: only then may it be used.
func (m Item) Ready() bool {
	return strings.TrimSpace(m.AltTR) != "" && strings.TrimSpace(m.AltEN) != ""
}

// URL is where the edge Worker serves the object (worker/src/r2.ts).
func (m Item) URL() string { return "/" + m.R2Key }

// Alt is the alt text for lang.
func (m Item) Alt(lang string) string {
	if lang == "tr" {
		return m.AltTR
	}
	return m.AltEN
}

// allowed are the accepted types, sniffed from the bytes, never taken from
// the client. SVG is excluded: it can carry script. PDF is for documents
// such as the CV (About); the Worker serves it as a download.
var allowed = map[string]string{
	"image/png": ".png", "image/jpeg": ".jpg", "image/gif": ".gif", "image/webp": ".webp",
	"application/pdf": ".pdf",
}

// IsImage reports whether the item is an image (not a document).
func (m Item) IsImage() bool { return strings.HasPrefix(m.Mime, "image/") }

// Errors the admin shows to the user.
var (
	ErrTooLarge = errors.New("media: file too large")
	ErrType     = errors.New("media: not a PNG, JPEG, GIF or WebP image, or a PDF")
	ErrInUse    = errors.New("media: used as a cover")
	ErrNotReady = errors.New("media: alt text missing")
)

// Repo stores media rows and objects.
type Repo struct {
	db   store.DB
	blob store.Blob
}

// NewRepo returns a repository over db and blob.
func NewRepo(db store.DB, blob store.Blob) *Repo { return &Repo{db: db, blob: blob} }

const columns = `id, r2_key, mime, width, height, alt_tr, alt_en, created_at, name, bytes`

// Upload checks, stores and records one file. The R2 object is written
// first; a failed insert leaves an orphan object rather than a row that
// points nowhere.
func (r *Repo) Upload(ctx context.Context, name string, data []byte) (Item, error) {
	if len(data) > MaxBytes {
		return Item{}, ErrTooLarge
	}
	mime := http.DetectContentType(data)
	ext, ok := allowed[mime]
	if !ok {
		return Item{}, ErrType
	}
	it := Item{ID: content.NewID(), Mime: mime, Name: cleanName(name), Bytes: int64(len(data)), CreatedAt: content.Clock().Unix()}
	if mime != "application/pdf" {
		cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
		if err != nil {
			return Item{}, ErrType
		}
		it.Width, it.Height = &cfg.Width, &cfg.Height
	}
	it.R2Key = "media/" + it.ID + "/" + fileName(it.Name, ext)
	if err := r.blob.Put(ctx, it.R2Key, data, mime); err != nil {
		return Item{}, err
	}
	_, err := r.db.Query(ctx, `INSERT INTO media (`+columns+`) VALUES (?, ?, ?, ?, ?, '', '', ?, ?, ?)`,
		it.ID, it.R2Key, it.Mime, it.Width, it.Height, it.CreatedAt, it.Name, it.Bytes)
	return it, err
}

// cleanName keeps the original name for display, without any path.
func cleanName(name string) string {
	name = path.Base(strings.ReplaceAll(name, "\\", "/"))
	if r := []rune(name); len(r) > 120 {
		name = string(r[:120])
	}
	if name == "." || name == "/" {
		return ""
	}
	return name
}

// fileName is the URL-safe object name: the slugified base plus the
// extension of the sniffed type.
func fileName(name, ext string) string {
	base := strings.TrimSuffix(name, path.Ext(name))
	s := content.Slugify(base)
	if s == "" {
		s = "file"
	}
	if len(s) > 60 {
		s = strings.TrimRight(s[:60], "-")
	}
	return s + ext
}

// List returns the newest items first.
func (r *Repo) List(ctx context.Context, limit int) ([]Item, error) {
	if limit <= 0 || limit > 200 {
		limit = 200
	}
	res, err := r.db.Query(ctx, `SELECT `+columns+` FROM media ORDER BY created_at DESC, id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	return store.Scan[Item](res)
}

// Ready lists images with both alt texts, for pickers.
func (r *Repo) Ready(ctx context.Context) ([]Item, error) {
	res, err := r.db.Query(ctx, `SELECT `+columns+` FROM media WHERE mime LIKE 'image/%' AND trim(alt_tr) <> '' AND trim(alt_en) <> ''
ORDER BY created_at DESC LIMIT 200`)
	if err != nil {
		return nil, err
	}
	return store.Scan[Item](res)
}

// Get returns one item.
func (r *Repo) Get(ctx context.Context, id string) (Item, error) {
	res, err := r.db.Query(ctx, `SELECT `+columns+` FROM media WHERE id = ?`, id)
	if err != nil {
		return Item{}, err
	}
	items, err := store.Scan[Item](res)
	if err != nil {
		return Item{}, err
	}
	if len(items) == 0 {
		return Item{}, content.NotFound("media", id)
	}
	return items[0], nil
}

// SetAlt stores both alt texts.
func (r *Repo) SetAlt(ctx context.Context, id, tr, en string) (Item, error) {
	res, err := r.db.Query(ctx, `UPDATE media SET alt_tr = ?, alt_en = ? WHERE id = ?`, strings.TrimSpace(tr), strings.TrimSpace(en), id)
	if err != nil {
		return Item{}, err
	}
	if res.Changes == 0 {
		return Item{}, content.NotFound("media", id)
	}
	return r.Get(ctx, id)
}

// Delete removes an item that no entry uses as its cover, no project shows
// in its gallery and no page (About, Now) names in its current or published
// version. Markdown that embeds the URL is not tracked; the admin warns
// about that.
func (r *Repo) Delete(ctx context.Context, id string) error {
	it, err := r.Get(ctx, id)
	if err != nil {
		return err
	}
	res, err := r.db.Query(ctx, `SELECT (SELECT COUNT(*) FROM entries WHERE cover_media_id = ?)
  + (SELECT COUNT(*) FROM project_gallery WHERE media_id = ?)
  + (SELECT COUNT(*) FROM entries e JOIN revisions r ON r.id IN (e.current_revision_id, e.published_revision_id)
     WHERE e.kind = 'page' AND instr(r.data_json, ?) > 0) AS n`, id, id, id)
	if err != nil {
		return err
	}
	if rows, _ := store.Scan[struct {
		N int `json:"n"`
	}](res); len(rows) == 1 && rows[0].N > 0 {
		return ErrInUse
	}
	if _, err := r.db.Query(ctx, `DELETE FROM media WHERE id = ?`, id); err != nil {
		return err
	}
	return r.blob.Delete(ctx, it.R2Key)
}

// CheckCover is what entry saves call for cover_media_id: the item must
// be an image and have both alt texts.
func (r *Repo) CheckCover(ctx context.Context, id string) error {
	it, err := r.Get(ctx, id)
	if err != nil {
		return err
	}
	if !it.IsImage() {
		return ErrType
	}
	if !it.Ready() {
		return ErrNotReady
	}
	return nil
}
