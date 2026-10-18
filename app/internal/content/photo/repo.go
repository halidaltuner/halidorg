package photo

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"halid.org/engine/internal/content"
	"halid.org/engine/internal/media"
	"halid.org/engine/internal/store"
)

// Repo reads and writes photos and albums.
type Repo struct {
	db   store.DB
	blob store.Blob // for EXIF; nil skips it
}

// NewRepo returns a repository over db and the media bucket.
func NewRepo(db store.DB, blob store.Blob) *Repo { return &Repo{db: db, blob: blob} }

// --- Albums -----------------------------------------------------------------

// Albums lists every album in order.
func (r *Repo) Albums(ctx context.Context) ([]Album, error) {
	res, err := r.db.Query(ctx, `SELECT id, slug, name_tr, name_en, sort FROM photo_albums ORDER BY sort, slug`)
	if err != nil {
		return nil, err
	}
	return store.Scan[Album](res)
}

// SaveAlbum inserts (no id) or updates an album.
func (r *Repo) SaveAlbum(ctx context.Context, a Album) (Album, error) {
	a.NameTR, a.NameEN, a.Slug = strings.TrimSpace(a.NameTR), strings.TrimSpace(a.NameEN), strings.TrimSpace(a.Slug)
	if a.Slug == "" {
		a.Slug = content.Slugify(a.NameEN)
	}
	v := content.ValidationError{}
	if a.NameTR == "" || a.NameEN == "" || utf8.RuneCountInString(a.NameTR) > 60 || utf8.RuneCountInString(a.NameEN) > 60 {
		v["album_name"] = "both names, 1 to 60 characters"
	}
	if !content.ValidSlug(a.Slug) || len(a.Slug) > 40 {
		v["album_slug"] = "lowercase letters, digits and hyphens, at most 40"
	}
	if len(v) > 0 {
		return a, v
	}
	if a.ID == "" {
		a.ID = content.NewID()
	}
	_, err := r.db.Query(ctx, `INSERT INTO photo_albums (id, slug, name_tr, name_en, sort) VALUES (?, ?, ?, ?, ?)
ON CONFLICT (id) DO UPDATE SET slug = excluded.slug, name_tr = excluded.name_tr, name_en = excluded.name_en, sort = excluded.sort`,
		a.ID, a.Slug, a.NameTR, a.NameEN, a.Sort)
	if err != nil && strings.Contains(strings.ToLower(err.Error()), "unique") {
		return a, content.ValidationError{"album_slug": "already used"}
	}
	return a, err
}

// DeleteAlbum removes an album without photos.
func (r *Repo) DeleteAlbum(ctx context.Context, id string) error {
	res, err := r.db.Query(ctx, `SELECT COUNT(*) AS n FROM photos WHERE album_id = ?`, id)
	if err != nil {
		return err
	}
	rows, err := store.Scan[struct {
		N int `json:"n"`
	}](res)
	if err != nil {
		return err
	}
	if len(rows) == 1 && rows[0].N > 0 {
		return content.ValidationError{"album_in_use": "the album still has photos"}
	}
	_, err = r.db.Query(ctx, `DELETE FROM photo_albums WHERE id = ?`, id)
	return err
}

// --- Photos -----------------------------------------------------------------

type row struct {
	Photo
	Pub int `json:"published"`
}

const columns = `id, album_id, media_id, title_tr, title_en, url_500px, camera, lens, exposure, place, taken_at,
  row_span, col_span, sort, published, created_at, updated_at, description_tr, description_en`

func (r *Repo) list(ctx context.Context, where string, args ...any) ([]Photo, error) {
	res, err := r.db.Query(ctx, `SELECT `+columns+` FROM photos WHERE `+where+
		` ORDER BY sort, coalesce(taken_at, created_at) DESC, id DESC LIMIT 1000`, args...)
	if err != nil {
		return nil, err
	}
	rows, err := store.Scan[row](res)
	if err != nil {
		return nil, err
	}
	out := make([]Photo, len(rows))
	for i, x := range rows {
		out[i] = x.Photo
		out[i].Published = x.Pub == 1
	}
	return out, nil
}

// All lists every photo (admin).
func (r *Repo) All(ctx context.Context) ([]Photo, error) { return r.list(ctx, "1 = 1") }

// Published lists the published photos (the page, feeds).
func (r *Repo) Published(ctx context.Context) ([]Photo, error) { return r.list(ctx, "published = 1") }

// Get loads one photo.
func (r *Repo) Get(ctx context.Context, id string) (Photo, error) {
	ps, err := r.list(ctx, "id = ?", id)
	if err != nil {
		return Photo{}, err
	}
	if len(ps) == 0 {
		return Photo{}, content.NotFound("photo", id)
	}
	return ps[0], nil
}

// Validate checks a photo's fields (the album and image are checked
// against their tables in Save).
func Validate(p Photo) error {
	v := content.ValidationError{}
	if p.TitleTR == "" && p.TitleEN == "" {
		v["title"] = "a title in at least one language"
	}
	for f, s := range map[string]string{"title": p.TitleTR + p.TitleEN, "camera": p.Camera, "lens": p.Lens, "exposure": p.Exposure, "place": p.Place} {
		if utf8.RuneCountInString(s) > 120 {
			v[f] = "at most 120 characters"
		}
	}
	for f, s := range map[string]string{"description_tr": p.DescTR, "description_en": p.DescEN} {
		if utf8.RuneCountInString(s) > 2000 {
			v[f] = "at most 2000 characters"
		}
	}
	if p.AlbumID == "" {
		v["album"] = "pick an album"
	}
	if p.MediaID == "" {
		v["image"] = "pick an image"
	}
	if p.URL500px != nil && !(content.HTTPSURL(*p.URL500px) && strings.HasPrefix(*p.URL500px, "https://500px.com/")) {
		v["url_500px"] = "a https://500px.com/ address"
	}
	if p.RowSpan < 1 || p.RowSpan > 3 {
		v["row_span"] = "1 to 3"
	}
	if p.ColSpan < 1 || p.ColSpan > 2 {
		v["col_span"] = "1 or 2"
	}
	if len(v) > 0 {
		return v
	}
	return nil
}

func trimAll(p *Photo) {
	for _, s := range []*string{&p.TitleTR, &p.TitleEN, &p.DescTR, &p.DescEN, &p.Camera, &p.Lens, &p.Exposure, &p.Place} {
		*s = strings.TrimSpace(*s)
	}
}

// Save inserts (no id) or updates a photo after checking the album and that
// the image is in media. Its alt text is the media item's, else the photo
// title. Empty camera, lens, exposure and date are filled from the image's
// EXIF when it has them.
func (r *Repo) Save(ctx context.Context, p Photo) (Photo, error) {
	trimAll(&p)
	if p.RowSpan == 0 {
		p.RowSpan = 2
	}
	if p.ColSpan == 0 {
		p.ColSpan = 1
	}
	if err := Validate(p); err != nil {
		return p, err
	}
	res, err := r.db.Query(ctx, `SELECT (SELECT COUNT(*) FROM photo_albums WHERE id = ?) AS album,
  (SELECT r2_key FROM media WHERE id = ? AND mime LIKE 'image/%') AS r2_key`,
		p.AlbumID, p.MediaID)
	if err != nil {
		return p, err
	}
	ok, err := store.Scan[struct {
		Album int     `json:"album"`
		Key   *string `json:"r2_key"`
	}](res)
	if err != nil {
		return p, err
	}
	if len(ok) != 1 || ok[0].Album == 0 {
		return p, content.ValidationError{"album": "pick an album"}
	}
	if ok[0].Key == nil {
		return p, content.ValidationError{"image": "an image from media"}
	}
	if p.Camera == "" || p.Lens == "" || p.Exposure == "" || p.TakenAt == nil {
		if e, err := r.EXIFOf(ctx, *ok[0].Key); err == nil {
			Fill(&p, e)
		}
	}
	now := content.Clock().Unix()
	if p.ID == "" {
		p.ID, p.CreatedAt = content.NewID(), now
	}
	p.UpdatedAt = now
	pub := 0
	if p.Published {
		pub = 1
	}
	_, err = r.db.Query(ctx, `INSERT INTO photos (`+columns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (id) DO UPDATE SET album_id = excluded.album_id, media_id = excluded.media_id, title_tr = excluded.title_tr,
  title_en = excluded.title_en, url_500px = excluded.url_500px, camera = excluded.camera, lens = excluded.lens,
  exposure = excluded.exposure, place = excluded.place, taken_at = excluded.taken_at, row_span = excluded.row_span,
  col_span = excluded.col_span, sort = excluded.sort, published = excluded.published, updated_at = excluded.updated_at,
  description_tr = excluded.description_tr, description_en = excluded.description_en`,
		p.ID, p.AlbumID, p.MediaID, p.TitleTR, p.TitleEN, p.URL500px, p.Camera, p.Lens, p.Exposure, p.Place, p.TakenAt,
		p.RowSpan, p.ColSpan, p.Sort, pub, p.CreatedAt, p.UpdatedAt, p.DescTR, p.DescEN)
	if err != nil {
		return p, fmt.Errorf("photo: save: %w", err)
	}
	return p, nil
}

// EXIFOf reads the EXIF of a media object by its R2 key.
func (r *Repo) EXIFOf(ctx context.Context, key string) (EXIF, error) {
	if r.blob == nil {
		return EXIF{}, nil
	}
	obj, err := r.blob.Get(ctx, key)
	if err != nil {
		return EXIF{}, err
	}
	return ReadEXIF(obj.Body), nil
}

// EXIFOfMedia reads the EXIF of a media item by id.
func (r *Repo) EXIFOfMedia(ctx context.Context, mediaID string) (EXIF, error) {
	res, err := r.db.Query(ctx, `SELECT r2_key FROM media WHERE id = ? AND mime LIKE 'image/%'`, mediaID)
	if err != nil {
		return EXIF{}, err
	}
	rows, err := store.Scan[struct {
		Key string `json:"r2_key"`
	}](res)
	if err != nil || len(rows) == 0 {
		return EXIF{}, err
	}
	return r.EXIFOf(ctx, rows[0].Key)
}

// Fill puts EXIF values into the empty fields of p.
func Fill(p *Photo, e EXIF) {
	if p.Camera == "" {
		p.Camera = e.Camera
	}
	if p.Lens == "" {
		p.Lens = e.Lens
	}
	if p.Exposure == "" {
		p.Exposure = e.Exposure
	}
	if p.TakenAt == nil && e.TakenAt != nil {
		v := e.TakenAt.Unix()
		p.TakenAt = &v
	}
}

// Delete removes a photo.
func (r *Repo) Delete(ctx context.Context, id string) error {
	res, err := r.db.Query(ctx, `DELETE FROM photos WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if res.Changes == 0 {
		return content.NotFound("photo", id)
	}
	return nil
}

// Date formats a taken_at as 2006-01-02; "" for nil.
func Date(sec *int64) string {
	if sec == nil {
		return ""
	}
	return time.Unix(*sec, 0).UTC().Format("2006-01-02")
}

// Upload stores an image in media and adds it to the album as a photo
// titled after the file name in both languages, with its EXIF filled in.
// Errors from media (media.ErrType, media.ErrTooLarge) are returned as is.
func (r *Repo) Upload(ctx context.Context, albumID, name string, data []byte, publish bool) (Photo, error) {
	res, err := r.db.Query(ctx, `SELECT COUNT(*) AS n FROM photo_albums WHERE id = ?`, albumID)
	if err != nil {
		return Photo{}, err
	}
	n, err := store.Scan[struct {
		N int `json:"n"`
	}](res)
	if err != nil {
		return Photo{}, err
	}
	if len(n) != 1 || n[0].N == 0 {
		return Photo{}, content.ErrNotFound
	}
	it, err := media.NewRepo(r.db, r.blob).Upload(ctx, name, data)
	if err != nil {
		return Photo{}, err
	}
	if !it.IsImage() {
		_ = media.NewRepo(r.db, r.blob).Delete(ctx, it.ID)
		return Photo{}, media.ErrType
	}
	t := TitleFromFile(it.Name)
	return r.Save(ctx, Photo{AlbumID: albumID, MediaID: it.ID, TitleTR: t, TitleEN: t, Published: publish})
}

// TitleFromFile turns "galata_blue-hour.jpg" into "galata blue hour".
func TitleFromFile(name string) string {
	if i := strings.LastIndexByte(name, '.'); i > 0 {
		name = name[:i]
	}
	t := strings.Join(strings.Fields(strings.NewReplacer("_", " ", "-", " ", ".", " ").Replace(name)), " ")
	if t == "" {
		t = "photo"
	}
	if r := []rune(t); len(r) > 200 {
		t = string(r[:200])
	}
	return t
}
