// Package capture is Quick Capture (design/Quick Capture.dc.html): paste a
// URL on a phone, get its title and description, save it as a link, fav or
// note, as a draft or published. Every step is a plain form: GET fetches
// (the bookmarklet opens it with ?url=&title=), POST saves.
package capture

import (
	"context"
	"errors"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"halid.org/engine/internal/admin"
	"halid.org/engine/internal/auth"
	"halid.org/engine/internal/content"
	"halid.org/engine/internal/content/fav"
	"halid.org/engine/internal/content/link"
	"halid.org/engine/internal/content/note"
	"halid.org/engine/internal/fetch"
	"halid.org/engine/internal/i18n"
	"halid.org/engine/internal/jobs"
	"halid.org/engine/internal/store"
)

// Fetcher reads a page's title and description (fetch.Client.Meta). The
// container fetches the page itself: the request never reaches the Worker
// bridge (fetch refuses .internal hosts and private addresses).
type Fetcher interface {
	Meta(ctx context.Context, url string) (fetch.Meta, error)
}

// Handlers serves /admin/quick.
type Handlers struct {
	db    store.DB
	ui    *admin.UI
	auth  *auth.Auth
	jobs  *jobs.Jobs
	fetch Fetcher
	links *link.Repo
	favs  *fav.Repo
	notes *note.Repo
	page  *template.Template
	saved *template.Template
}

// New parses the Quick Capture templates.
func New(db store.DB, u *admin.UI, a *auth.Auth, j *jobs.Jobs, f Fetcher) (*Handlers, error) {
	page, err := u.ParseBare("quick", "templates/capture/quick.html")
	if err != nil {
		return nil, err
	}
	saved, err := u.ParseBare("quick-saved", "templates/capture/saved.html")
	if err != nil {
		return nil, err
	}
	return &Handlers{db: db, ui: u, auth: a, jobs: j, fetch: f, links: link.NewRepo(db), favs: fav.NewRepo(db),
		notes: note.NewRepo(db), page: page, saved: saved}, nil
}

// Register adds the routes. Saving is a write (admin.Guard in a session);
// publishing also needs a recent sign-in, checked after the draft is saved
// so a re-auth never loses what was typed.
func (h *Handlers) Register(mux *http.ServeMux) {
	s := func(f http.HandlerFunc) http.Handler { return h.auth.RequireSession(f) }
	w := func(f http.HandlerFunc) http.Handler { return admin.Guard(s(f)) }
	mux.Handle("GET /admin/quick", s(h.form))
	mux.Handle("POST /admin/quick", w(h.save))
	mux.Handle("GET /admin/quick/saved/{id}", s(h.savedPage))
	mux.Handle("POST /admin/quick/{id}/publish", w(h.publish))
}

// Kinds Quick Capture saves, in button order.
var kinds = []string{link.Kind, fav.Kind, note.Kind}

type option struct {
	Value, Label string
	On           bool
}

type page struct {
	Lang, Env   string
	Dark        bool
	URL         string
	Title       string
	Desc        string
	Domain      string
	Kind        string
	Comment     string
	EntryLang   string
	Fetched     bool // the fields below the URL show
	FetchError  string
	Errors      []string
	Kinds       []option
	Langs       []option
	Bookmarklet template.URL
	// Duplicate is an entry that already has this address. The form says
	// so and carries dup_ok=URL, so pressing save again saves anyway.
	Duplicate *duplicate
}

func (h *Handlers) newPage(r *http.Request) page {
	lang := i18n.FromRequest(r)
	sh := h.ui.Shell(r, "", "")
	return page{Lang: lang, Env: sh.Env, Kind: link.Kind, EntryLang: lang, Bookmarklet: admin.Bookmarklet()}
}

func (p *page) options() {
	p.Kinds = nil
	for _, k := range kinds {
		p.Kinds = append(p.Kinds, option{Value: k, Label: i18n.T(p.Lang, "palette.kind."+k), On: k == p.Kind})
	}
	p.Langs = nil
	for _, l := range i18n.Supported {
		p.Langs = append(p.Langs, option{Value: l, Label: l, On: l == p.EntryLang})
	}
}

func (p *page) read(r *http.Request) {
	p.URL = strings.TrimSpace(r.FormValue("url"))
	p.Title = strings.TrimSpace(r.FormValue("title"))
	p.Desc = strings.TrimSpace(r.FormValue("desc"))
	p.Comment = strings.TrimSpace(r.FormValue("comment"))
	if k := r.FormValue("kind"); contains(kinds, k) {
		p.Kind = k
	}
	if l := r.FormValue("lang"); i18n.IsSupported(l) {
		p.EntryLang = l
	}
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// form serves the capture screen. With ?url= (the fetch button, or the
// bookmarklet) it fetches the page and fills title, description and domain;
// a failed fetch keeps the fields open so they can be typed. The button
// with an empty URL opens the fields too, for a note without a link.
func (h *Handlers) form(w http.ResponseWriter, r *http.Request) {
	p := h.newPage(r)
	p.read(r)
	if _, ok := r.URL.Query()["url"]; ok {
		p.Fetched = true
		if p.URL == "" {
			p.Kind = note.Kind
		} else {
			h.fill(r, &p)
			dup, err := h.findDuplicate(r.Context(), p.Lang, p.URL)
			if err != nil {
				h.fail(w, err)
				return
			}
			p.Duplicate = dup
		}
	}
	p.options()
	h.ui.Render(w, http.StatusOK, h.page, p)
}

// fill fetches p.URL. Fetched values replace the form's title and
// description only when the form had none (the bookmarklet sends the
// tab's title; an edited title survives a refetch).
func (h *Handlers) fill(r *http.Request, p *page) {
	if !strings.Contains(p.URL, "://") {
		p.URL = "https://" + p.URL
	}
	p.Domain = content.Domain(p.URL)
	ctx, cancel := context.WithTimeout(r.Context(), 6*time.Second)
	defer cancel()
	m, err := h.fetch.Meta(ctx, p.URL)
	if err != nil {
		slog.Info("quick capture fetch", "url", p.URL, "err", err)
		p.FetchError = i18n.T(p.Lang, "qc.fetch_failed")
		if errors.Is(err, fetch.ErrBlocked) {
			p.FetchError = i18n.T(p.Lang, "qc.fetch_blocked")
		}
		return
	}
	if p.Title == "" {
		p.Title = m.Title
	}
	if p.Desc == "" {
		p.Desc = m.Description
	}
}

// favType guesses a fav's type from where it lives; the fav editor
// (Phase 2.5) can change it.
func favType(u string) string {
	switch d := content.Domain(u); {
	case d == "github.com" || d == "gitlab.com" || d == "codeberg.org":
		return "repo"
	case d == "youtube.com" || d == "youtu.be" || d == "vimeo.com" || strings.HasSuffix(d, ".youtube.com"):
		return "video"
	}
	return "other"
}

// slugs yields candidate slugs: the title's, else the domain's, else the
// kind and date, then the same with -2 to -9.
func slugs(p page) []string {
	base := content.Slugify(p.Title)
	if base == "" {
		base = content.Slugify(content.Domain(p.URL))
	}
	if base == "" {
		base = p.Kind + "-" + time.Now().UTC().Format("2006-01-02")
	}
	if len(base) > 76 {
		base = strings.TrimRight(base[:76], "-")
	}
	out := []string{base}
	for i := 2; i <= 9; i++ {
		out = append(out, fmt.Sprintf("%s-%d", base, i))
	}
	return out
}

// create saves p as a draft of its kind and returns the entry id. A slug
// that is taken moves on to the next candidate.
func (h *Handlers) create(ctx context.Context, p page) (string, error) {
	var err error
	for _, slug := range slugs(p) {
		e := content.Entry{Slug: slug, Lang: p.EntryLang, Title: p.Title}
		switch p.Kind {
		case link.Kind:
			e.Summary = p.Comment
			var l link.Link
			l, err = h.links.Create(ctx, link.Draft{Link: link.Link{Entry: e, URL: p.URL}})
			if err == nil {
				return l.ID, nil
			}
		case fav.Kind:
			e.Summary = p.Comment
			var f fav.Fav
			f, err = h.favs.Create(ctx, fav.Draft{Fav: fav.Fav{Entry: e, URL: p.URL, Type: favType(p.URL)}})
			if err == nil {
				return f.ID, nil
			}
		case note.Kind:
			n := note.Note{Entry: e}
			if p.URL != "" {
				n.SourceURL = p.URL
			}
			var x note.Note
			x, err = h.notes.Create(ctx, note.Draft{Note: n, Markdown: p.Comment})
			if err == nil {
				return x.ID, nil
			}
		}
		var v content.ValidationError
		if !errors.As(err, &v) || v["slug"] == "" || len(v) > 1 {
			return "", err
		}
	}
	return "", err
}

// check returns the problems that stop a save, in the page language.
func check(p page) []string {
	var errs []string
	if p.Kind != note.Kind || p.URL != "" {
		if !content.HTTPSURL(p.URL) {
			errs = append(errs, i18n.T(p.Lang, "qc.err_url"))
		}
	}
	if p.Kind != note.Kind && p.Title == "" {
		errs = append(errs, i18n.T(p.Lang, "qc.err_title"))
	}
	if p.Kind == note.Kind && p.Comment == "" && p.Title == "" {
		errs = append(errs, i18n.T(p.Lang, "qc.err_note"))
	}
	return errs
}

// save creates the draft, then publishes it when asked and the sign-in is
// recent. Otherwise publishing goes through re-auth and comes back to the
// saved screen with a publish button: the draft is already safe.
func (h *Handlers) save(w http.ResponseWriter, r *http.Request) {
	p := h.newPage(r)
	p.read(r)
	p.Fetched = true
	p.Domain = content.Domain(p.URL)
	if errs := check(p); len(errs) > 0 {
		p.Errors = errs
		p.options()
		h.ui.Render(w, http.StatusUnprocessableEntity, h.page, p)
		return
	}
	// Saved before? Say so once; the form then carries dup_ok with the
	// address it was shown for, so editing the address checks again.
	if p.URL != "" && r.FormValue("dup_ok") != p.URL {
		dup, err := h.findDuplicate(r.Context(), p.Lang, p.URL)
		if err != nil {
			h.fail(w, err)
			return
		}
		if dup != nil {
			p.Duplicate = dup
			p.options()
			h.ui.Render(w, http.StatusConflict, h.page, p)
			return
		}
	}
	id, err := h.create(r.Context(), p)
	var v content.ValidationError
	if errors.As(err, &v) {
		for k, msg := range v {
			p.Errors = append(p.Errors, k+": "+msg)
		}
		p.options()
		h.ui.Render(w, http.StatusUnprocessableEntity, h.page, p)
		return
	}
	if err != nil {
		h.fail(w, err)
		return
	}
	h.enqueue(r.Context(), id, nil)
	saved := "/admin/quick/saved/" + id
	if r.FormValue("action") != "publish" {
		http.Redirect(w, r, saved, http.StatusSeeOther)
		return
	}
	if !h.auth.Recent(r) {
		http.Redirect(w, r, auth.ReauthPath(saved+"?publish=1"), http.StatusSeeOther)
		return
	}
	if err := h.publishEntry(r.Context(), id); err != nil {
		h.fail(w, err)
		return
	}
	http.Redirect(w, r, saved, http.StatusSeeOther)
}

func (h *Handlers) publish(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	saved := "/admin/quick/saved/" + id
	if !h.auth.Recent(r) {
		http.Redirect(w, r, auth.ReauthPath(saved+"?publish=1"), http.StatusSeeOther)
		return
	}
	if err := h.publishEntry(r.Context(), id); errors.Is(err, content.ErrNotFound) {
		http.NotFound(w, r)
		return
	} else if err != nil {
		h.fail(w, err)
		return
	}
	http.Redirect(w, r, saved, http.StatusSeeOther)
}

// publishEntry publishes a captured entry of any of the three kinds.
func (h *Handlers) publishEntry(ctx context.Context, id string) error {
	prev, err := jobs.RefOf(ctx, h.db, id)
	if err != nil {
		return err
	}
	if prev == nil {
		return content.NotFound("entry", id)
	}
	switch prev.Kind {
	case link.Kind:
		_, err = h.links.Publish(ctx, id)
	case fav.Kind:
		_, err = h.favs.Publish(ctx, id)
	case note.Kind:
		_, err = h.notes.Publish(ctx, id)
	default:
		return content.NotFound("captured entry", id)
	}
	if err != nil {
		return err
	}
	h.enqueue(ctx, id, prev)
	return nil
}

type savedPage struct {
	Lang, Env    string
	Dark         bool
	ID, Kind     string
	Meta         string
	Published    bool
	OfferPublish bool // back from re-auth with the draft still a draft
	Public       string
	Edit         string // the entry in its admin editor
}

func (h *Handlers) savedPage(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	res, err := h.db.Query(r.Context(), `SELECT e.kind, e.lang, e.slug, e.status,
  coalesce(l.url, f.url, n.source_url, '') AS url
FROM entries e LEFT JOIN links l ON l.entry_id = e.id LEFT JOIN favs f ON f.entry_id = e.id
LEFT JOIN notes n ON n.entry_id = e.id WHERE e.id = ?`, id)
	if err != nil {
		h.fail(w, err)
		return
	}
	rows, err := store.Scan[struct {
		Kind   string `json:"kind"`
		Lang   string `json:"lang"`
		Slug   string `json:"slug"`
		Status string `json:"status"`
		URL    string `json:"url"`
	}](res)
	if err != nil {
		h.fail(w, err)
		return
	}
	if len(rows) == 0 || !contains(kinds, rows[0].Kind) {
		http.NotFound(w, r)
		return
	}
	e := rows[0]
	lang := i18n.FromRequest(r)
	p := savedPage{Lang: lang, Env: h.ui.Shell(r, "", "").Env, ID: id, Kind: i18n.T(lang, "palette.kind."+e.Kind), Published: e.Status == content.Published}
	parts := []string{i18n.T(lang, "qc.status_"+map[bool]string{true: "published", false: "draft"}[p.Published]), e.Lang}
	if d := content.Domain(e.URL); d != "" {
		parts = append(parts, d)
	}
	p.Meta = strings.Join(parts, " · ")
	p.OfferPublish = !p.Published && r.URL.Query().Get("publish") == "1"
	p.Edit = admin.EditorHref(e.Kind, id)
	if p.Published {
		switch e.Kind {
		case note.Kind:
			p.Public = "/" + e.Lang + "/notes/" + url.PathEscape(e.Slug)
		case link.Kind:
			p.Public = "/" + e.Lang + "/links"
		case fav.Kind:
			p.Public = "/" + e.Lang + "/favs#" + e.Slug
		}
	}
	h.ui.Render(w, http.StatusOK, h.saved, p)
}

func (h *Handlers) enqueue(ctx context.Context, id string, prev *jobs.Ref) {
	if err := jobs.Enqueue(ctx, h.jobs, id, prev); err != nil {
		slog.Error("enqueue entry.changed", "entry", id, "err", err)
	}
}

func (h *Handlers) fail(w http.ResponseWriter, err error) {
	slog.Error("quick capture", "err", err)
	http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
}
