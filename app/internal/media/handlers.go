package media

import (
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"halid.org/engine/internal/admin"
	"halid.org/engine/internal/auth"
	"halid.org/engine/internal/content"
	"halid.org/engine/internal/i18n"
)

// Handlers serve /admin/media (derive: dashboard density and tokens).
type Handlers struct {
	repo *Repo
	ui   *admin.UI
	auth *auth.Auth
	page *template.Template
}

// New parses the media templates.
func New(repo *Repo, u *admin.UI, a *auth.Auth) (*Handlers, error) {
	t, err := u.Parse("media", "templates/admin/dashboard.html", "templates/admin/media.html")
	if err != nil {
		return nil, err
	}
	return &Handlers{repo: repo, ui: u, auth: a, page: t}, nil
}

// Register adds the routes. Uploads and edits go through admin.Guard.
func (h *Handlers) Register(mux *http.ServeMux) {
	s := func(f http.HandlerFunc) http.Handler { return h.auth.RequireSession(f) }
	w := func(f http.HandlerFunc) http.Handler { return admin.Guard(s(f)) }
	mux.Handle("GET /admin/media", s(h.list))
	mux.Handle("POST /admin/media", w(h.upload))
	mux.Handle("POST /admin/media/{id}/alt", w(h.alt))
	mux.Handle("POST /admin/media/{id}/delete", w(h.delete))
}

// Card is one grid card.
type Card struct {
	Lang  string
	Item  Item
	URL   string
	Ready bool
	Size  string
	Error string
}

func card(lang string, it Item) Card {
	return Card{Lang: lang, Item: it, URL: it.URL(), Ready: it.Ready(), Size: size(it.Bytes)}
}

func size(n int64) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%d KB", n>>10)
	}
	return fmt.Sprintf("%d B", n)
}

type listPage struct {
	admin.Shell
	Cards  []Card
	Errors []string
	Max    string
}

func (h *Handlers) list(w http.ResponseWriter, r *http.Request) {
	h.render(w, r, http.StatusOK, nil)
}

func (h *Handlers) render(w http.ResponseWriter, r *http.Request, status int, errs []string) {
	lang := i18n.FromRequest(r)
	items, err := h.repo.List(r.Context(), 200)
	if err != nil {
		h.fail(w, err)
		return
	}
	p := listPage{Shell: h.ui.Shell(r, i18n.T(lang, "admin.media.title"), "media"), Errors: errs, Max: size(MaxBytes)}
	for _, it := range items {
		p.Cards = append(p.Cards, card(lang, it))
	}
	h.ui.Render(w, status, h.page, p)
}

// upload stores every file in the "file" field. The JS island sends one
// file per request (for its progress bar) and gets the card back; the plain
// form sends several and returns to the grid.
func (h *Handlers) upload(w http.ResponseWriter, r *http.Request) {
	lang := i18n.FromRequest(r)
	r.Body = http.MaxBytesReader(w, r.Body, 5*MaxBytes)
	if err := r.ParseMultipartForm(MaxBytes); err != nil {
		h.uploadError(w, r, i18n.T(lang, "admin.media.too_large"))
		return
	}
	defer func() { _ = r.MultipartForm.RemoveAll() }()
	files := r.MultipartForm.File["file"]
	if len(files) == 0 {
		h.uploadError(w, r, i18n.T(lang, "admin.media.no_file"))
		return
	}
	var cards []Card
	var errs []string
	for _, fh := range files {
		if fh.Size > MaxBytes {
			errs = append(errs, fh.Filename+": "+i18n.T(lang, "admin.media.too_large"))
			continue
		}
		f, err := fh.Open()
		if err != nil {
			h.fail(w, err)
			return
		}
		data, err := io.ReadAll(io.LimitReader(f, MaxBytes+1))
		_ = f.Close()
		if err != nil {
			h.fail(w, err)
			return
		}
		it, err := h.repo.Upload(r.Context(), fh.Filename, data)
		switch {
		case errors.Is(err, ErrTooLarge):
			errs = append(errs, fh.Filename+": "+i18n.T(lang, "admin.media.too_large"))
		case errors.Is(err, ErrType):
			errs = append(errs, fh.Filename+": "+i18n.T(lang, "admin.media.bad_type"))
		case err != nil:
			h.fail(w, err)
			return
		default:
			cards = append(cards, card(lang, it))
		}
	}
	if r.Header.Get("X-Upload") == "1" {
		if len(errs) > 0 {
			http.Error(w, errs[0], http.StatusUnprocessableEntity)
			return
		}
		if strings.Contains(r.Header.Get("Accept"), "application/json") {
			// The block editor inserts the file itself (static/js/editor.js).
			it := cards[0].Item
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{"id": it.ID, "url": it.URL(), "name": it.Name, "mime": it.Mime, "image": it.IsImage()})
			return
		}
		h.ui.Fragment(w, http.StatusCreated, h.page, "media-card", cards[0])
		return
	}
	if len(errs) > 0 {
		h.render(w, r, http.StatusUnprocessableEntity, errs)
		return
	}
	http.Redirect(w, r, "/admin/media", http.StatusSeeOther)
}

func (h *Handlers) uploadError(w http.ResponseWriter, r *http.Request, msg string) {
	if r.Header.Get("X-Upload") == "1" {
		http.Error(w, msg, http.StatusUnprocessableEntity)
		return
	}
	h.render(w, r, http.StatusUnprocessableEntity, []string{msg})
}

// alt saves both alt texts; htmx gets the card back.
func (h *Handlers) alt(w http.ResponseWriter, r *http.Request) {
	it, err := h.repo.SetAlt(r.Context(), r.PathValue("id"), r.FormValue("alt_tr"), r.FormValue("alt_en"))
	if errors.Is(err, content.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		h.fail(w, err)
		return
	}
	if admin.IsHTMX(r) {
		h.ui.Fragment(w, http.StatusOK, h.page, "media-card", card(i18n.FromRequest(r), it))
		return
	}
	http.Redirect(w, r, "/admin/media#m-"+it.ID, http.StatusSeeOther)
}

func (h *Handlers) delete(w http.ResponseWriter, r *http.Request) {
	lang := i18n.FromRequest(r)
	id := r.PathValue("id")
	err := h.repo.Delete(r.Context(), id)
	if errors.Is(err, content.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if errors.Is(err, ErrInUse) {
		if admin.IsHTMX(r) {
			it, _ := h.repo.Get(r.Context(), id)
			c := card(lang, it)
			c.Error = i18n.T(lang, "admin.media.in_use")
			h.ui.Fragment(w, http.StatusOK, h.page, "media-card", c)
			return
		}
		h.render(w, r, http.StatusConflict, []string{i18n.T(lang, "admin.media.in_use")})
		return
	}
	if err != nil {
		h.fail(w, err)
		return
	}
	if admin.IsHTMX(r) {
		w.WriteHeader(http.StatusOK) // the card swaps to nothing
		return
	}
	http.Redirect(w, r, "/admin/media", http.StatusSeeOther)
}

func (h *Handlers) fail(w http.ResponseWriter, err error) {
	slog.Error("media admin", "err", err)
	http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
}
