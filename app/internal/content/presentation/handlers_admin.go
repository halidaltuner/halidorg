package presentation

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"halid.org/engine/internal/admin"
	"halid.org/engine/internal/auth"
	"halid.org/engine/internal/content"
	"halid.org/engine/internal/content/shortadmin"
	"halid.org/engine/internal/i18n"
	"halid.org/engine/internal/jobs"
	"halid.org/engine/internal/store"
)

// Base, Text and TagList let the shared editor (shortadmin) edit a
// presentation.
func (d *Draft) Base() *content.Entry { return &d.Entry }
func (d *Draft) Text() *string        { return &d.Markdown }
func (d *Draft) TagList() *[]string   { return &d.Tags }

// options is what templates/presentation/admin_fields.html reads.
type options struct {
	Types, Langs, Covers []shortadmin.Option
	Outline              []Slide
	Links                []Link
	HeldOn, Slides       string
	Minutes, Recording   string
	// The deck panel, once the presentation is saved.
	Saved                bool
	DeckURL, DeckVersion string
	DeckAction           string
	MaxMB                int
	Bridge               string
}

// NewEditor is the presentation list and editor at /admin/presentations
// (Phase 5.4, derive). The outline and related links are rows like project
// links; the deck has its own upload form below the editor (Decks).
func NewEditor(db store.DB, blob store.Blob, u *admin.UI, a *auth.Auth, j *jobs.Jobs) (*shortadmin.Admin, error) {
	r := NewRepo(db)
	return shortadmin.New(shortadmin.Kind{
		Name: Kind, Section: "presentations", Body: true, Fields: "templates/presentation/admin_fields.html",
		Public: func(d shortadmin.Draft) string { e := d.Base(); return "/" + e.Lang + "/presentations/" + e.Slug },
		New: func() shortadmin.Draft {
			return &Draft{Presentation: Presentation{Entry: content.Entry{Kind: Kind}, Type: "talk"}}
		},
		Load: func(ctx context.Context, id string) (shortadmin.Draft, error) {
			p, err := r.Get(ctx, id)
			return &Draft{Presentation: p}, err
		},
		Read: readForm,
		Create: func(ctx context.Context, d shortadmin.Draft) (string, error) {
			p, err := r.Create(ctx, *d.(*Draft))
			return p.ID, err
		},
		Save: func(ctx context.Context, d shortadmin.Draft, origin string) error {
			_, err := r.Save(ctx, *d.(*Draft), origin)
			return err
		},
		Publish: func(ctx context.Context, id string) error {
			_, err := r.Publish(ctx, id)
			return err
		},
		DeleteStmts: func(e content.Entry) []store.Stmt { return DeleteStmts(e.ID) },
		Deleted:     func(ctx context.Context, e content.Entry) error { return DeleteDeck(ctx, blob, e.ID) },
		Options: func(ctx context.Context, lang string, d shortadmin.Draft) (any, error) {
			return formOptions(ctx, db, lang, d.(*Draft))
		},
	}, db, u, a, j)
}

func readForm(req *http.Request, d shortadmin.Draft) {
	p := d.(*Draft)
	p.Type = req.FormValue("pres_type")
	p.Event = req.FormValue("event")
	p.City = req.FormValue("city")
	p.TalkLang = req.FormValue("talk_lang")
	p.HeldOn = nil
	if s := strings.TrimSpace(req.FormValue("held_on")); s != "" {
		v := day(s)
		p.HeldOn = &v
	}
	p.SlideCount, _ = strconv.Atoi(strings.TrimSpace(req.FormValue("slide_count")))
	p.RecordingURL = shortadmin.Ptr(req, "recording_url")
	p.RecordingMinutes = nil
	if s := strings.TrimSpace(req.FormValue("recording_minutes")); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil {
			n = -1
		}
		p.RecordingMinutes = &n
	}
	p.CoverMediaID = shortadmin.Ptr(req, "cover")
	p.Outline = nil
	for _, x := range shortadmin.Rows(req, "outline_n", "outline_n", "outline_title") {
		n, _ := strconv.Atoi(x[0])
		p.Outline = append(p.Outline, Slide{N: n, Title: x[1]})
	}
	p.Links = nil
	for _, x := range shortadmin.Rows(req, "link_order", "link_label", "link_url") {
		p.Links = append(p.Links, Link{Label: x[0], URL: x[1]})
	}
}

func formOptions(ctx context.Context, db store.DB, lang string, p *Draft) (options, error) {
	o := options{HeldOn: Day(p.HeldOn), Saved: p.ID != "", DeckURL: p.DeckURL(), MaxMB: MaxDeckBytes >> 20, Bridge: Bridge}
	if p.SlideCount > 0 {
		o.Slides = strconv.Itoa(p.SlideCount)
	}
	if p.RecordingURL != nil {
		o.Recording = *p.RecordingURL
	}
	if p.RecordingMinutes != nil && *p.RecordingMinutes > 0 {
		o.Minutes = strconv.Itoa(*p.RecordingMinutes)
	}
	if p.DeckVersion != nil {
		o.DeckVersion = *p.DeckVersion
	}
	if p.ID != "" {
		o.DeckAction = "/admin/presentations/" + p.ID + "/deck"
	}
	for _, t := range Types {
		o.Types = append(o.Types, shortadmin.Option{Value: t, Label: i18n.T(lang, "kind.presentation.type."+t), On: p.Type == t})
	}
	talk := p.TalkLang
	if talk == "" {
		talk = p.Lang
	}
	for _, l := range i18n.Supported {
		o.Langs = append(o.Langs, shortadmin.Option{Value: l, Label: l, On: talk == l})
	}
	res, err := db.Query(ctx, `SELECT id, name FROM media WHERE mime LIKE 'image/%' ORDER BY created_at DESC LIMIT 200`)
	if err != nil {
		return o, err
	}
	media, err := store.Scan[struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}](res)
	if err != nil {
		return o, err
	}
	o.Covers = []shortadmin.Option{{Value: "", Label: i18n.T(lang, "kind.presentation.no_cover"), On: p.CoverMediaID == nil}}
	for _, m := range media {
		name := m.Name
		if name == "" {
			name = m.ID
		}
		o.Covers = append(o.Covers, shortadmin.Option{Value: m.ID, Label: name, On: p.CoverMediaID != nil && *p.CoverMediaID == m.ID})
	}
	o.Outline = append(append([]Slide(nil), p.Outline...), Slide{}, Slide{}, Slide{})
	o.Links = append(append([]Link(nil), p.Links...), Link{}, Link{})
	return o, nil
}

// Decks serves the deck upload of a saved presentation.
type Decks struct {
	db   store.DB
	blob store.Blob
	repo *Repo
	auth *auth.Auth
	jobs *jobs.Jobs
}

// NewDecks returns the deck upload handler.
func NewDecks(db store.DB, blob store.Blob, a *auth.Auth, j *jobs.Jobs) *Decks {
	return &Decks{db: db, blob: blob, repo: NewRepo(db), auth: a, jobs: j}
}

// Register adds POST /admin/presentations/{id}/deck, a state-changing
// route: admin.Guard inside a session.
func (h *Decks) Register(mux *http.ServeMux) {
	mux.Handle("POST /admin/presentations/{id}/deck", admin.Guard(h.auth.RequireSession(http.HandlerFunc(h.upload))))
}

// upload stores the "deck" file and returns to the editor, which says what
// happened (?notice=deck_done or ?error=deck_{reason}). The page and its
// list re-render: the slide count and the deck version are on them.
func (h *Decks) upload(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := r.PathValue("id")
	back := "/admin/presentations/" + id + "?error=deck_"
	r.Body = http.MaxBytesReader(w, r.Body, MaxDeckBytes+1<<20)
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		http.Redirect(w, r, back+"too_large", http.StatusSeeOther)
		return
	}
	defer func() { _ = r.MultipartForm.RemoveAll() }()
	f, _, err := r.FormFile("deck")
	if err != nil {
		http.Redirect(w, r, back+"no_file", http.StatusSeeOther)
		return
	}
	data, err := io.ReadAll(io.LimitReader(f, MaxDeckBytes+1))
	_ = f.Close()
	if err != nil {
		h.fail(w, err)
		return
	}
	prev, err := jobs.RefOf(ctx, h.db, id)
	if err != nil {
		h.fail(w, err)
		return
	}
	_, err = h.repo.SetDeck(ctx, h.blob, id, data)
	switch {
	case errors.Is(err, content.ErrNotFound):
		http.NotFound(w, r)
		return
	case errors.Is(err, ErrDeckLarge):
		http.Redirect(w, r, back+"too_large", http.StatusSeeOther)
		return
	case errors.Is(err, ErrDeckType):
		http.Redirect(w, r, back+"bad_type", http.StatusSeeOther)
		return
	case err != nil:
		h.fail(w, err)
		return
	}
	if err := jobs.Enqueue(ctx, h.jobs, id, prev); err != nil {
		slog.Error("enqueue entry.changed", "entry", id, "err", err) // the dashboard lists failed jobs
	}
	http.Redirect(w, r, "/admin/presentations/"+id+"?notice=deck_done", http.StatusSeeOther)
}

func (h *Decks) fail(w http.ResponseWriter, err error) {
	slog.Error("presentation deck", "err", err)
	http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
}
