package photo

import (
	"errors"
	"html/template"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"halid.org/engine/internal/admin"
	"halid.org/engine/internal/auth"
	"halid.org/engine/internal/content"
	"halid.org/engine/internal/content/shortadmin"
	"halid.org/engine/internal/i18n"
	"halid.org/engine/internal/jobs"
	"halid.org/engine/internal/media"
	"halid.org/engine/internal/store"
)

// Admin serves /admin/photos (derive: dashboard density): the list with the
// albums below it, and a form per photo with "fill from EXIF". Every write
// re-renders the photos page and its feeds.
type Admin struct {
	db   store.DB
	repo *Repo
	ui   *admin.UI
	auth *auth.Auth
	jobs *jobs.Jobs
	list *template.Template
	edit *template.Template
}

// NewAdmin parses the pages.
func NewAdmin(db store.DB, blob store.Blob, u *admin.UI, a *auth.Auth, j *jobs.Jobs) (*Admin, error) {
	list, err := u.Parse("photos", "templates/photo/admin_list.html")
	if err != nil {
		return nil, err
	}
	edit, err := u.Parse("photo-edit", "templates/photo/admin_edit.html")
	if err != nil {
		return nil, err
	}
	return &Admin{db: db, repo: NewRepo(db, blob), ui: u, auth: a, jobs: j, list: list, edit: edit}, nil
}

// Register adds the routes: writes go through admin.Guard inside a session;
// deleting a photo needs a recent sign-in.
func (h *Admin) Register(mux *http.ServeMux) {
	s := func(f http.HandlerFunc) http.Handler { return h.auth.RequireSession(f) }
	w := func(f http.HandlerFunc) http.Handler { return admin.Guard(s(f)) }
	mux.Handle("GET /admin/photos", s(h.listPage))
	mux.Handle("GET /admin/photos/new", s(h.newPage))
	mux.Handle("POST /admin/photos", w(h.save))
	mux.Handle("GET /admin/photos/{id}", s(h.editPage))
	mux.Handle("POST /admin/photos/{id}", w(h.save))
	mux.Handle("POST /admin/photos/{id}/delete", admin.Guard(h.auth.RequireSession(h.auth.RequireRecentAuth(http.HandlerFunc(h.delete)))))
	mux.Handle("POST /admin/photos/albums", w(h.saveAlbum))
	mux.Handle("POST /admin/photos/albums/{id}/delete", w(h.deleteAlbum))
	mux.Handle("POST /admin/photos/albums/{id}/upload", w(h.upload))
}

type listRow struct {
	Href, Title, Album, Date, Spans string
	Published                       bool
}

type listPage struct {
	admin.Shell
	Rows   []listRow
	Albums []Album
	Notice string
	Errors []string
}

func (h *Admin) listPage(w http.ResponseWriter, r *http.Request) {
	h.renderList(w, r, http.StatusOK, nil)
}

func (h *Admin) renderList(w http.ResponseWriter, r *http.Request, code int, errs []string) {
	ctx := r.Context()
	lang := i18n.FromRequest(r)
	p := listPage{Shell: h.ui.Shell(r, i18n.T(lang, "admin.nav.photos"), "photos"), Errors: errs}
	if n := r.URL.Query().Get("done"); n != "" && errs == nil {
		p.Notice = i18n.T(lang, "admin.photo.done_"+n)
	}
	var err error
	if p.Albums, err = h.repo.Albums(ctx); err != nil {
		h.fail(w, err)
		return
	}
	names := map[string]string{}
	for _, a := range p.Albums {
		names[a.ID] = a.Name(lang)
	}
	all, err := h.repo.All(ctx)
	if err != nil {
		h.fail(w, err)
		return
	}
	for _, x := range all {
		p.Rows = append(p.Rows, listRow{Href: "/admin/photos/" + x.ID, Title: x.Title(lang), Album: names[x.AlbumID], Date: Date(x.TakenAt),
			Spans: strconv.Itoa(x.ColSpan) + "×" + strconv.Itoa(x.RowSpan), Published: x.Published})
	}
	h.ui.Render(w, code, h.list, p)
}

type editPage struct {
	admin.Shell
	Photo          Photo
	New            bool
	Albums, Images []shortadmin.Option
	Rows, Cols     []shortadmin.Option
	Taken, URL     string
	Sort           string
	Errors         []string
	Notice         string
}

func (h *Admin) newPage(w http.ResponseWriter, r *http.Request) {
	h.renderEdit(w, r, http.StatusOK, Photo{RowSpan: 2, ColSpan: 1, Published: true}, nil, "")
}

func (h *Admin) editPage(w http.ResponseWriter, r *http.Request) {
	p, err := h.repo.Get(r.Context(), r.PathValue("id"))
	if errors.Is(err, content.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		h.fail(w, err)
		return
	}
	notice := ""
	if r.URL.Query().Get("saved") == "1" {
		notice = i18n.T(i18n.FromRequest(r), "entry.saved")
	}
	h.renderEdit(w, r, http.StatusOK, p, nil, notice)
}

func (h *Admin) renderEdit(w http.ResponseWriter, r *http.Request, code int, p Photo, errs []string, notice string) {
	ctx := r.Context()
	lang := i18n.FromRequest(r)
	title := p.Title(lang)
	if p.ID == "" {
		title = i18n.T(lang, "admin.photo.new")
	}
	e := editPage{Shell: h.ui.Shell(r, title, "photos"), Photo: p, New: p.ID == "", Errors: errs, Notice: notice,
		Taken: Date(p.TakenAt), Sort: strconv.Itoa(p.Sort)}
	if p.URL500px != nil {
		e.URL = *p.URL500px
	}
	for _, n := range []int{1, 2, 3} {
		e.Rows = append(e.Rows, shortadmin.Option{Value: strconv.Itoa(n), Label: strconv.Itoa(n), On: p.RowSpan == n})
	}
	for _, n := range []int{1, 2} {
		e.Cols = append(e.Cols, shortadmin.Option{Value: strconv.Itoa(n), Label: strconv.Itoa(n), On: p.ColSpan == n})
	}
	albums, err := h.repo.Albums(ctx)
	if err != nil {
		h.fail(w, err)
		return
	}
	e.Albums = []shortadmin.Option{{Value: "", Label: i18n.T(lang, "admin.photo.pick_album"), On: p.AlbumID == ""}}
	for _, a := range albums {
		e.Albums = append(e.Albums, shortadmin.Option{Value: a.ID, Label: a.Name(lang), On: a.ID == p.AlbumID})
	}
	res, err := h.db.Query(ctx, `SELECT id, name FROM media WHERE mime LIKE 'image/%' ORDER BY created_at DESC LIMIT 500`)
	if err != nil {
		h.fail(w, err)
		return
	}
	media, err := store.Scan[struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}](res)
	if err != nil {
		h.fail(w, err)
		return
	}
	e.Images = []shortadmin.Option{{Value: "", Label: i18n.T(lang, "admin.photo.pick_image"), On: p.MediaID == ""}}
	for _, m := range media {
		name := m.Name
		if name == "" {
			name = m.ID
		}
		e.Images = append(e.Images, shortadmin.Option{Value: m.ID, Label: name, On: m.ID == p.MediaID})
	}
	h.ui.Render(w, code, h.edit, e)
}

// read fills p from the form.
func read(r *http.Request, p *Photo) {
	p.AlbumID = r.FormValue("album")
	p.MediaID = r.FormValue("image")
	p.TitleTR, p.TitleEN = r.FormValue("title_tr"), r.FormValue("title_en")
	p.DescTR, p.DescEN = r.FormValue("description_tr"), r.FormValue("description_en")
	p.URL500px = shortadmin.Ptr(r, "url_500px")
	p.Camera, p.Lens, p.Exposure, p.Place = r.FormValue("camera"), r.FormValue("lens"), r.FormValue("exposure"), r.FormValue("place")
	p.TakenAt = nil
	if t, err := time.Parse("2006-01-02", strings.TrimSpace(r.FormValue("taken"))); err == nil {
		v := t.Unix()
		p.TakenAt = &v
	}
	p.RowSpan, _ = strconv.Atoi(r.FormValue("row_span"))
	p.ColSpan, _ = strconv.Atoi(r.FormValue("col_span"))
	p.Sort, _ = strconv.Atoi(strings.TrimSpace(r.FormValue("sort")))
	p.Published = r.FormValue("published") == "1"
}

// save stores the form; "fill from EXIF" re-shows it with the image's EXIF
// in the empty fields instead, without saving.
func (h *Admin) save(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := i18n.FromRequest(r)
	var p Photo
	if id := r.PathValue("id"); id != "" {
		cur, err := h.repo.Get(ctx, id)
		if errors.Is(err, content.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			h.fail(w, err)
			return
		}
		p = cur
	}
	read(r, &p)
	if r.FormValue("action") == "exif" {
		e, err := h.repo.EXIFOfMedia(ctx, p.MediaID)
		if err != nil {
			h.fail(w, err)
			return
		}
		before := p
		Fill(&p, e)
		notice := i18n.T(lang, "admin.photo.exif_none")
		if p != before {
			notice = i18n.T(lang, "admin.photo.exif_filled")
		}
		h.renderEdit(w, r, http.StatusOK, p, nil, notice)
		return
	}
	saved, err := h.repo.Save(ctx, p)
	var v content.ValidationError
	if errors.As(err, &v) {
		h.renderEdit(w, r, http.StatusUnprocessableEntity, p, shortadmin.Errors(lang, v), "")
		return
	}
	if err != nil {
		h.fail(w, err)
		return
	}
	h.refresh(r)
	http.Redirect(w, r, "/admin/photos/"+saved.ID+"?saved=1", http.StatusSeeOther)
}

func (h *Admin) delete(w http.ResponseWriter, r *http.Request) {
	if err := h.repo.Delete(r.Context(), r.PathValue("id")); errors.Is(err, content.ErrNotFound) {
		http.NotFound(w, r)
		return
	} else if err != nil {
		h.fail(w, err)
		return
	}
	h.refresh(r)
	http.Redirect(w, r, "/admin/photos?done=deleted", http.StatusSeeOther)
}

func (h *Admin) saveAlbum(w http.ResponseWriter, r *http.Request) {
	sort, _ := strconv.Atoi(strings.TrimSpace(r.FormValue("sort")))
	_, err := h.repo.SaveAlbum(r.Context(), Album{ID: r.FormValue("id"), NameTR: r.FormValue("name_tr"), NameEN: r.FormValue("name_en"),
		Slug: r.FormValue("slug"), Sort: sort})
	var v content.ValidationError
	if errors.As(err, &v) {
		h.renderList(w, r, http.StatusUnprocessableEntity, shortadmin.Errors(i18n.FromRequest(r), v))
		return
	}
	if err != nil {
		h.fail(w, err)
		return
	}
	h.refresh(r)
	http.Redirect(w, r, "/admin/photos?done=album", http.StatusSeeOther)
}

func (h *Admin) deleteAlbum(w http.ResponseWriter, r *http.Request) {
	err := h.repo.DeleteAlbum(r.Context(), r.PathValue("id"))
	var v content.ValidationError
	if errors.As(err, &v) {
		h.renderList(w, r, http.StatusUnprocessableEntity, shortadmin.Errors(i18n.FromRequest(r), v))
		return
	}
	if err != nil {
		h.fail(w, err)
		return
	}
	h.refresh(r)
	http.Redirect(w, r, "/admin/photos?done=album_deleted", http.StatusSeeOther)
}

// upload adds every file in the "file" field to the album as a photo
// (drafts unless "publish" is checked), then returns to the list.
func (h *Admin) upload(w http.ResponseWriter, r *http.Request) {
	lang := i18n.FromRequest(r)
	r.Body = http.MaxBytesReader(w, r.Body, 10*media.MaxBytes)
	if err := r.ParseMultipartForm(media.MaxBytes); err != nil {
		h.renderList(w, r, http.StatusUnprocessableEntity, []string{i18n.T(lang, "admin.photo.upload_too_large")})
		return
	}
	defer func() { _ = r.MultipartForm.RemoveAll() }()
	files := r.MultipartForm.File["file"]
	if len(files) == 0 {
		h.renderList(w, r, http.StatusUnprocessableEntity, []string{i18n.T(lang, "admin.media.no_file")})
		return
	}
	publish := r.FormValue("publish") == "1"
	var errs []string
	added := 0
	for _, fh := range files {
		f, err := fh.Open()
		if err != nil {
			h.fail(w, err)
			return
		}
		data, err := io.ReadAll(io.LimitReader(f, media.MaxBytes+1))
		_ = f.Close()
		if err != nil {
			h.fail(w, err)
			return
		}
		_, err = h.repo.Upload(r.Context(), r.PathValue("id"), fh.Filename, data, publish)
		switch {
		case errors.Is(err, content.ErrNotFound):
			http.NotFound(w, r)
			return
		case errors.Is(err, media.ErrTooLarge):
			errs = append(errs, fh.Filename+": "+i18n.T(lang, "admin.media.too_large"))
		case errors.Is(err, media.ErrType):
			errs = append(errs, fh.Filename+": "+i18n.T(lang, "admin.photo.bad_type"))
		case err != nil:
			h.fail(w, err)
			return
		default:
			added++
		}
	}
	if added > 0 {
		h.refresh(r)
	}
	if len(errs) > 0 {
		h.renderList(w, r, http.StatusUnprocessableEntity, errs)
		return
	}
	done := "uploaded_draft"
	if publish {
		done = "uploaded"
	}
	http.Redirect(w, r, "/admin/photos?done="+done, http.StatusSeeOther)
}

// Paths are the pages that show photos.
func Paths() []string {
	var out []string
	for _, l := range i18n.Supported {
		b := "/" + l + "/photos"
		out = append(out, b, b+"/feed.xml", b+"/atom.xml", b+"/feed.json")
	}
	return out
}

func (h *Admin) refresh(r *http.Request) {
	if err := jobs.EnqueuePages(r.Context(), h.jobs, Paths()...); err != nil {
		slog.Error("enqueue pages.changed", "err", err) // the dashboard lists failed jobs
	}
	if err := jobs.EnqueueSearchGallery(r.Context(), h.jobs); err != nil {
		slog.Error("enqueue search.gallery", "err", err)
	}
}

func (h *Admin) fail(w http.ResponseWriter, err error) {
	slog.Error("photos admin", "err", err)
	http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
}
