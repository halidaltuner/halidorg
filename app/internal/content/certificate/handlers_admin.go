package certificate

import (
	"errors"
	"html/template"
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

// Admin serves /admin/certificates (derive: dashboard density): the list
// with the issuers below it, and a form per certificate. Every write
// re-renders the certificate pages and feeds and About (its card).
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
func NewAdmin(db store.DB, u *admin.UI, a *auth.Auth, j *jobs.Jobs) (*Admin, error) {
	list, err := u.Parse("certificates", "templates/certificate/admin_list.html")
	if err != nil {
		return nil, err
	}
	edit, err := u.Parse("certificate-edit", "templates/certificate/admin_edit.html")
	if err != nil {
		return nil, err
	}
	return &Admin{db: db, repo: NewRepo(db), ui: u, auth: a, jobs: j, list: list, edit: edit}, nil
}

// Register adds the routes: writes go through admin.Guard inside a session;
// deleting a certificate needs a recent sign-in.
func (h *Admin) Register(mux *http.ServeMux) {
	s := func(f http.HandlerFunc) http.Handler { return h.auth.RequireSession(f) }
	w := func(f http.HandlerFunc) http.Handler { return admin.Guard(s(f)) }
	mux.Handle("GET /admin/certificates", s(h.listPage))
	mux.Handle("GET /admin/certificates/new", s(h.newPage))
	mux.Handle("POST /admin/certificates", w(h.save))
	mux.Handle("GET /admin/certificates/{id}", s(h.editPage))
	mux.Handle("POST /admin/certificates/{id}", w(h.save))
	mux.Handle("POST /admin/certificates/{id}/delete", admin.Guard(h.auth.RequireSession(h.auth.RequireRecentAuth(http.HandlerFunc(h.delete)))))
	mux.Handle("POST /admin/certificates/issuers", w(h.saveIssuer))
	mux.Handle("POST /admin/certificates/issuers/{id}/delete", w(h.deleteIssuer))
}

type listRow struct {
	Href, Name, Issuer, Issued, Expires, Status, StatusKey string
	Published                                              bool
}

type listPage struct {
	admin.Shell
	Rows    []listRow
	Issuers []Issuer
	Tones   []string
	Notice  string
	Errors  []string
}

func (h *Admin) listPage(w http.ResponseWriter, r *http.Request) {
	h.renderList(w, r, http.StatusOK, nil)
}

func (h *Admin) renderList(w http.ResponseWriter, r *http.Request, code int, errs []string) {
	ctx := r.Context()
	lang := i18n.FromRequest(r)
	p := listPage{Shell: h.ui.Shell(r, i18n.T(lang, "admin.nav.certificates"), "certificates"), Tones: content.Tones, Errors: errs}
	if n := r.URL.Query().Get("done"); n != "" && errs == nil {
		p.Notice = i18n.T(lang, "admin.cert.done_"+n)
	}
	var err error
	if p.Issuers, err = h.repo.Issuers(ctx); err != nil {
		h.fail(w, err)
		return
	}
	names := map[string]string{}
	for _, i := range p.Issuers {
		names[i.ID] = i.Name
	}
	all, err := h.repo.All(ctx)
	if err != nil {
		h.fail(w, err)
		return
	}
	now := content.Clock()
	for _, c := range all {
		st := c.Status(now)
		p.Rows = append(p.Rows, listRow{Href: "/admin/certificates/" + c.ID, Name: c.Name, Issuer: names[c.IssuerID],
			Issued: Month(&c.IssuedAt), Expires: Month(c.ExpiresAt), StatusKey: st, Status: i18n.T(lang, "cert.status."+st), Published: c.Published})
	}
	h.ui.Render(w, code, h.list, p)
}

type editPage struct {
	admin.Shell
	Cert            Certificate
	New             bool
	Issuers, Badges []shortadmin.Option
	Skills          string
	Issued, Expires string
	Credly, Sort    string
	Errors          []string
	Notice          string
}

func (h *Admin) newPage(w http.ResponseWriter, r *http.Request) {
	h.renderEdit(w, r, http.StatusOK, Certificate{}, nil)
}

func (h *Admin) editPage(w http.ResponseWriter, r *http.Request) {
	c, err := h.repo.Get(r.Context(), r.PathValue("id"))
	if errors.Is(err, content.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		h.fail(w, err)
		return
	}
	h.renderEdit(w, r, http.StatusOK, c, nil)
}

func (h *Admin) renderEdit(w http.ResponseWriter, r *http.Request, code int, c Certificate, errs []string) {
	ctx := r.Context()
	lang := i18n.FromRequest(r)
	title := c.Name
	if c.ID == "" {
		title = i18n.T(lang, "admin.cert.new")
	}
	p := editPage{Shell: h.ui.Shell(r, title, "certificates"), Cert: c, New: c.ID == "", Errors: errs,
		Skills: strings.Join(c.Skills, ", "), Expires: Month(c.ExpiresAt), Sort: strconv.Itoa(c.Sort)}
	if c.IssuedAt != 0 {
		p.Issued = Month(&c.IssuedAt)
	}
	if c.CredlyURL != nil {
		p.Credly = *c.CredlyURL
	}
	if r.URL.Query().Get("saved") == "1" && errs == nil {
		p.Notice = i18n.T(lang, "entry.saved")
	}
	issuers, err := h.repo.Issuers(ctx)
	if err != nil {
		h.fail(w, err)
		return
	}
	p.Issuers = []shortadmin.Option{{Value: "", Label: i18n.T(lang, "admin.cert.pick_issuer"), On: c.IssuerID == ""}}
	for _, i := range issuers {
		p.Issuers = append(p.Issuers, shortadmin.Option{Value: i.ID, Label: i.Name, On: i.ID == c.IssuerID})
	}
	res, err := h.db.Query(ctx, `SELECT id, name FROM media WHERE mime LIKE 'image/%' AND trim(alt_tr) <> '' AND trim(alt_en) <> ''
ORDER BY created_at DESC LIMIT 300`)
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
	p.Badges = []shortadmin.Option{{Value: "", Label: i18n.T(lang, "kind.project.no_image"), On: c.BadgeID == nil}}
	for _, m := range media {
		name := m.Name
		if name == "" {
			name = m.ID
		}
		p.Badges = append(p.Badges, shortadmin.Option{Value: m.ID, Label: name, On: c.BadgeID != nil && *c.BadgeID == m.ID})
	}
	h.ui.Render(w, code, h.edit, p)
}

// read fills c from the form; unparseable months become 0 or -1, which
// Validate reports.
func read(r *http.Request, c *Certificate) {
	c.IssuerID = r.FormValue("issuer")
	c.Name = r.FormValue("name")
	c.Skills = strings.Split(r.FormValue("skills"), ",")
	c.BadgeID = shortadmin.Ptr(r, "badge")
	c.CredlyURL = shortadmin.Ptr(r, "credly_url")
	c.IssuedAt, _ = ParseMonth(strings.TrimSpace(r.FormValue("issued")))
	c.ExpiresAt = nil
	if s := strings.TrimSpace(r.FormValue("expires")); s != "" {
		v, ok := ParseMonth(s)
		if !ok {
			v = -1
		}
		c.ExpiresAt = &v
	}
	c.Sort, _ = strconv.Atoi(strings.TrimSpace(r.FormValue("sort")))
	c.Published = r.FormValue("published") == "1"
}

func (h *Admin) save(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var c Certificate
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
		c = cur
	}
	read(r, &c)
	saved, err := h.repo.Save(ctx, c)
	var v content.ValidationError
	if errors.As(err, &v) {
		h.renderEdit(w, r, http.StatusUnprocessableEntity, c, shortadmin.Errors(i18n.FromRequest(r), v))
		return
	}
	if err != nil {
		h.fail(w, err)
		return
	}
	h.refresh(r)
	http.Redirect(w, r, "/admin/certificates/"+saved.ID+"?saved=1", http.StatusSeeOther)
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
	http.Redirect(w, r, "/admin/certificates?done=deleted", http.StatusSeeOther)
}

func (h *Admin) saveIssuer(w http.ResponseWriter, r *http.Request) {
	sort, _ := strconv.Atoi(strings.TrimSpace(r.FormValue("sort")))
	_, err := h.repo.SaveIssuer(r.Context(), Issuer{ID: r.FormValue("id"), Name: r.FormValue("name"), Slug: r.FormValue("slug"),
		Tone: r.FormValue("tone"), Sort: sort})
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
	http.Redirect(w, r, "/admin/certificates?done=issuer", http.StatusSeeOther)
}

func (h *Admin) deleteIssuer(w http.ResponseWriter, r *http.Request) {
	err := h.repo.DeleteIssuer(r.Context(), r.PathValue("id"))
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
	http.Redirect(w, r, "/admin/certificates?done=issuer_deleted", http.StatusSeeOther)
}

// Paths are the pages that show certificates.
func Paths() []string {
	var out []string
	for _, l := range i18n.Supported {
		b := "/" + l + "/certificates"
		out = append(out, b, b+"/feed.xml", b+"/atom.xml", b+"/feed.json", "/"+l+"/about")
	}
	return out
}

func (h *Admin) refresh(r *http.Request) {
	if err := jobs.EnqueuePages(r.Context(), h.jobs, Paths()...); err != nil {
		slog.Error("enqueue pages.changed", "err", err) // the dashboard lists failed jobs
	}
}

func (h *Admin) fail(w http.ResponseWriter, err error) {
	slog.Error("certificates admin", "err", err)
	http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
}
