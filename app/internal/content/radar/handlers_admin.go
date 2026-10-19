package radar

import (
	"context"
	"errors"
	"html/template"
	"log/slog"
	"net/http"
	"time"

	"halid.org/engine/internal/admin"
	"halid.org/engine/internal/auth"
	"halid.org/engine/internal/content"
	"halid.org/engine/internal/content/shortadmin"
	"halid.org/engine/internal/i18n"
	"halid.org/engine/internal/jobs"
	"halid.org/engine/internal/store"
)

// Admin serves /admin/radar (derive: dashboard density, DESIGN.md §34): a
// table of the working copy, each row saved on its own (htmx, or a plain
// post without JS), a form to add an item, and the freeze page with what
// the next edition changes.
type Admin struct {
	db     store.DB
	repo   *Repo
	ui     *admin.UI
	auth   *auth.Auth
	jobs   *jobs.Jobs
	page   *template.Template
	freeze *template.Template
}

// NewAdmin parses the pages.
func NewAdmin(db store.DB, u *admin.UI, a *auth.Auth, j *jobs.Jobs) (*Admin, error) {
	page, err := u.Parse("radar", "templates/radar/admin.html")
	if err != nil {
		return nil, err
	}
	freeze, err := u.Parse("radar-freeze", "templates/radar/admin_freeze.html")
	if err != nil {
		return nil, err
	}
	return &Admin{db: db, repo: NewRepo(db), ui: u, auth: a, jobs: j, page: page, freeze: freeze}, nil
}

// Register adds the routes. Row saves and new items go through admin.Guard
// inside a session; deleting and freezing need a recent sign-in.
func (h *Admin) Register(mux *http.ServeMux) {
	s := func(f http.HandlerFunc) http.Handler { return h.auth.RequireSession(f) }
	w := func(f http.HandlerFunc) http.Handler { return admin.Guard(s(f)) }
	recent := func(f http.HandlerFunc) http.Handler {
		return admin.Guard(h.auth.RequireSession(h.auth.RequireRecentAuth(f)))
	}
	mux.Handle("GET /admin/radar", s(h.list))
	mux.Handle("POST /admin/radar", w(h.create))
	mux.Handle("GET /admin/radar/freeze", s(h.freezePage))
	mux.Handle("POST /admin/radar/freeze", recent(h.doFreeze))
	mux.Handle("POST /admin/radar/{group}", w(h.save))
	mux.Handle("POST /admin/radar/{group}/delete", recent(h.delete))
}

// Option is one choice in a select.
type Option struct {
	Value, Label string
	On           bool
}

// row is one table row: the item, its choices and its own errors.
type row struct {
	Lang      string
	Item      Item
	Quadrants []Option
	Rings     []Option
	Errors    []string
	Saved     bool
	Published bool // on a frozen edition: has public pages
}

type listPage struct {
	admin.Shell
	Rows      []row
	New       row
	Errors    []string
	Notice    string
	Pending   summary
	Latest    *Edition
	Editions  []Edition
	HasChange bool
}

// summary counts what the next freeze changes.
type summary struct{ On, New, Moved, Dropped int }

func summarize(v View) summary {
	s := summary{On: len(v.Blips), Dropped: len(v.Dropped)}
	for _, b := range v.Blips {
		if b.New || (b.Prev == "" && len(v.Editions) == 0) {
			s.New++
		}
		if b.Moved {
			s.Moved++
		}
	}
	return s
}

func (h *Admin) row(lang string, i Item, published bool) row {
	r := row{Lang: lang, Item: i, Published: published}
	for _, q := range Quadrants {
		r.Quadrants = append(r.Quadrants, Option{Value: q, Label: i18n.T(lang, "radar.q."+q), On: i.Quadrant == q})
	}
	for _, g := range Rings {
		r.Rings = append(r.Rings, Option{Value: g, Label: i18n.T(lang, "radar.ring."+g), On: i.Ring == g})
	}
	return r
}

// published lists the groups with published entries.
func (h *Admin) published(ctx context.Context) (map[string]bool, error) {
	res, err := h.db.Query(ctx, `SELECT DISTINCT translation_group_id FROM entries WHERE kind = ? AND status = 'published'`, Kind)
	if err != nil {
		return nil, err
	}
	rows, err := store.Scan[struct {
		Group string `json:"translation_group_id"`
	}](res)
	out := map[string]bool{}
	for _, x := range rows {
		out[x.Group] = true
	}
	return out, err
}

func (h *Admin) list(w http.ResponseWriter, r *http.Request) {
	h.render(w, r, http.StatusOK, Item{Quadrant: Quadrants[0], Ring: Rings[0]}, nil)
}

func (h *Admin) render(w http.ResponseWriter, r *http.Request, code int, draft Item, errs []string) {
	ctx := r.Context()
	lang := i18n.FromRequest(r)
	items, err := h.repo.Items(ctx)
	if err != nil {
		h.fail(w, err)
		return
	}
	pub, err := h.published(ctx)
	if err != nil {
		h.fail(w, err)
		return
	}
	pending, err := h.repo.Pending(ctx)
	if err != nil {
		h.fail(w, err)
		return
	}
	p := listPage{Shell: h.ui.Shell(r, i18n.T(lang, "admin.nav.radar"), "radar"), New: h.row(lang, draft, false), Errors: errs,
		Pending: summarize(pending), Editions: pending.Editions}
	if len(pending.Editions) > 0 {
		p.Latest = &pending.Editions[0]
	}
	p.HasChange = p.Pending.New+p.Pending.Moved+p.Pending.Dropped > 0 || p.Latest == nil && p.Pending.On > 0
	if !p.HasChange && p.Latest != nil {
		// Renamed items and edited notes also go out with the next edition.
		snap, err := h.repo.Snapshot(ctx, p.Latest.ID)
		if err != nil {
			h.fail(w, err)
			return
		}
		p.HasChange = textChanged(snap, pending.Blips)
	}
	if n := r.URL.Query().Get("notice"); n == "created" || n == "deleted" || n == "frozen" {
		p.Notice = i18n.T(lang, "admin.radar.notice."+n)
	}
	for _, i := range items {
		p.Rows = append(p.Rows, h.row(lang, i, pub[i.Group]))
	}
	h.ui.Render(w, code, h.page, p)
}

// textChanged reports whether an item on both kept its ring but changed its
// name or a note.
func textChanged(prev, cur []Blip) bool {
	was := map[string]Blip{}
	for _, b := range prev {
		was[b.Group] = b
	}
	for _, b := range cur {
		if p, ok := was[b.Group]; ok && (p.Name != b.Name || p.NoteTR != b.NoteTR || p.NoteEN != b.NoteEN || p.Quadrant != b.Quadrant) {
			return true
		}
	}
	return false
}

func itemFrom(r *http.Request, i Item) Item {
	i.Name, i.Quadrant, i.Ring = r.FormValue("name"), r.FormValue("quadrant"), r.FormValue("ring")
	i.NoteTR, i.NoteEN = r.FormValue("note_tr"), r.FormValue("note_en")
	i.Off = r.FormValue("off") == "1"
	return i
}

func (h *Admin) create(w http.ResponseWriter, r *http.Request) {
	draft := itemFrom(r, Item{})
	_, err := h.repo.Create(r.Context(), draft)
	var v content.ValidationError
	if errors.As(err, &v) {
		h.render(w, r, http.StatusUnprocessableEntity, draft, shortadmin.Errors(i18n.FromRequest(r), v))
		return
	}
	if err != nil {
		h.fail(w, err)
		return
	}
	http.Redirect(w, r, "/admin/radar?notice=created", http.StatusSeeOther)
}

// save writes one row. With htmx the row comes back in place; without JS
// the page reloads at the row.
func (h *Admin) save(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := i18n.FromRequest(r)
	cur, err := h.repo.Get(ctx, r.PathValue("group"))
	if errors.Is(err, content.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		h.fail(w, err)
		return
	}
	ids, err := h.repo.EntryIDs(ctx, cur.Group)
	if err != nil {
		h.fail(w, err)
		return
	}
	next := itemFrom(r, cur)
	saved, err := h.repo.Save(ctx, next)
	var v content.ValidationError
	if errors.As(err, &v) {
		rw := h.row(lang, next, len(ids) > 0)
		rw.Errors = shortadmin.Errors(lang, v)
		if admin.IsHTMX(r) {
			h.ui.Fragment(w, http.StatusOK, h.page, "radar-row", rw)
			return
		}
		h.render(w, r, http.StatusUnprocessableEntity, Item{Quadrant: Quadrants[0], Ring: Rings[0]}, rw.Errors)
		return
	}
	if err != nil {
		h.fail(w, err)
		return
	}
	if admin.IsHTMX(r) {
		rw := h.row(lang, saved, len(ids) > 0)
		rw.Saved = true
		h.ui.Fragment(w, http.StatusOK, h.page, "radar-row", rw)
		return
	}
	http.Redirect(w, r, "/admin/radar#r-"+saved.Group, http.StatusSeeOther)
}

func (h *Admin) delete(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	group := r.PathValue("group")
	ids, err := h.repo.EntryIDs(ctx, group)
	if err != nil {
		h.fail(w, err)
		return
	}
	prev := h.refs(ctx, ids)
	err = h.repo.Delete(ctx, group)
	if errors.Is(err, content.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		h.fail(w, err)
		return
	}
	h.enqueue(ctx, prev)
	http.Redirect(w, r, "/admin/radar?notice=deleted", http.StatusSeeOther)
}

// refs snapshots entries before a write, for the render jobs.
func (h *Admin) refs(ctx context.Context, ids map[string]string) map[string]*jobs.Ref {
	out := map[string]*jobs.Ref{}
	for _, id := range ids {
		ref, err := jobs.RefOf(ctx, h.db, id)
		if err != nil {
			slog.Error("radar ref", "id", id, "err", err)
		}
		out[id] = ref
	}
	return out
}

func (h *Admin) enqueue(ctx context.Context, prev map[string]*jobs.Ref) {
	for id, ref := range prev {
		if err := jobs.Enqueue(ctx, h.jobs, id, ref); err != nil {
			slog.Error("enqueue entry.changed", "id", id, "err", err) // the dashboard lists failed jobs
		}
	}
}

// change is one line of the freeze diff.
type change struct {
	Name, Mark, Quadrant, From, To string
}

type freezePage struct {
	admin.Shell
	Label   string
	Latest  *Edition
	Changes []change
	Same    int
	Errors  []string
}

func (h *Admin) freezePage(w http.ResponseWriter, r *http.Request) {
	h.renderFreeze(w, r, http.StatusOK, "", nil)
}

func (h *Admin) renderFreeze(w http.ResponseWriter, r *http.Request, code int, label string, errs []string) {
	lang := i18n.FromRequest(r)
	v, err := h.repo.Pending(r.Context())
	if err != nil {
		h.fail(w, err)
		return
	}
	if label == "" {
		label = nextLabel(content.Clock().UTC(), v.Editions)
	}
	p := freezePage{Shell: h.ui.Shell(r, i18n.T(lang, "admin.radar.freeze"), "radar"), Label: label, Errors: errs}
	if len(v.Editions) > 0 {
		p.Latest = &v.Editions[0]
	}
	ring := func(g string) string {
		if g == "" {
			return ""
		}
		return i18n.T(lang, "radar.ring."+g)
	}
	for _, b := range v.Blips {
		c := change{Name: b.Name, Quadrant: i18n.T(lang, "radar.q."+b.Quadrant), To: ring(b.Ring)}
		switch {
		case b.Prev == "":
			c.Mark = "new"
		case b.Moved:
			c.Mark, c.From = "moved", ring(b.Prev)
		default:
			p.Same++
			continue
		}
		p.Changes = append(p.Changes, c)
	}
	for _, b := range v.Dropped {
		p.Changes = append(p.Changes, change{Name: b.Name, Mark: "dropped", Quadrant: i18n.T(lang, "radar.q."+b.Quadrant), From: ring(b.Ring)})
	}
	h.ui.Render(w, code, h.freeze, p)
}

// nextLabel suggests this month, or the month after the latest edition
// when that is already this month or later.
func nextLabel(now time.Time, eds []Edition) string {
	label := now.Format("2006-01")
	if len(eds) > 0 && eds[0].Label >= label {
		if t, err := time.Parse("2006-01", eds[0].Label); err == nil {
			label = t.AddDate(0, 1, 0).Format("2006-01")
		}
	}
	return label
}

func (h *Admin) doFreeze(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := i18n.FromRequest(r)
	label := r.FormValue("label")
	res, err := h.db.Query(ctx, `SELECT id FROM entries WHERE kind = ?`, Kind)
	if err != nil {
		h.fail(w, err)
		return
	}
	before, err := store.Scan[struct {
		ID string `json:"id"`
	}](res)
	if err != nil {
		h.fail(w, err)
		return
	}
	ids := map[string]string{}
	for _, x := range before {
		ids[x.ID] = x.ID
	}
	prev := h.refs(ctx, ids)
	f, err := h.repo.Freeze(ctx, label)
	var v content.ValidationError
	if errors.As(err, &v) {
		msgs := []string{}
		for k := range v {
			msgs = append(msgs, i18n.T(lang, "admin.radar.err."+k))
		}
		h.renderFreeze(w, r, http.StatusUnprocessableEntity, label, msgs)
		return
	}
	if err != nil {
		h.fail(w, err)
		return
	}
	for _, id := range f.Entries {
		if _, ok := prev[id]; !ok {
			prev[id] = nil // new entries: nothing to purge before
		}
	}
	h.enqueue(ctx, prev)
	http.Redirect(w, r, "/admin/radar?notice=frozen", http.StatusSeeOther)
}

func (h *Admin) fail(w http.ResponseWriter, err error) {
	slog.Error("radar admin", "err", err)
	http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
}
