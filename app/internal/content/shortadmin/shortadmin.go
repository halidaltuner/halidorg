// Package shortadmin is the admin list and editor shared by the short-form
// kinds: notes, snippets, links and favs (Phase 2.6, derive: dashboard
// density, DESIGN.md §20). Each kind describes itself with a Kind in its own
// handlers_admin.go; this package owns the routes, the form and the
// save / publish / delete flow, which match the post editor's: writes go
// through admin.Guard, publish checks a recent sign-in after saving, delete
// needs one before, and every write enqueues entry.changed with the
// pre-write ref.
package shortadmin

import (
	"context"
	"errors"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"halid.org/engine/internal/admin"
	"halid.org/engine/internal/auth"
	"halid.org/engine/internal/content"
	"halid.org/engine/internal/i18n"
	"halid.org/engine/internal/jobs"
	"halid.org/engine/internal/store"
)

// Draft is a kind's draft as the editor sees it: the entry, the Markdown
// body and the tags. Each kind's Draft implements it on its pointer.
type Draft interface {
	Base() *content.Entry
	Text() *string
	TagList() *[]string
}

// Kind describes one kind to the editor.
type Kind struct {
	Name    string // entries.kind
	Section string // URL segment and sidebar key, e.g. "notes"
	Body    bool   // the form has a Markdown body (notes, snippets)
	// Fields is the template file that defines "entry-fields", the kind's
	// own inputs. It reads .Item (the Draft) and .Options.
	Fields string
	Extra  []string // more template files the fields use (the tone picker)
	// Public is the item's public page, or "" for kinds listed on a page
	// of their own (links, favs). Deleting a published item with a page
	// leaves a 410 there.
	Public  func(d Draft) string
	New     func() Draft
	Load    func(ctx context.Context, id string) (Draft, error) // entry and kind fields; text and tags are filled here
	Read    func(r *http.Request, d Draft)                      // the kind's fields from the form
	Create  func(ctx context.Context, d Draft) (string, error)
	Save    func(ctx context.Context, d Draft, origin string) error
	Publish func(ctx context.Context, id string) error
	// Options is extra data for the fields template (categories, files);
	// nil when the kind needs none.
	Options func(ctx context.Context, lang string, d Draft) (any, error)
	// Optional hooks. Nav is the sidebar key when it differs from Section
	// (build logs sit under projects). Prefill fills a new draft from the
	// query (a build log's project). DeleteStmts replace the plain delete
	// of the entry (a project takes its build logs with it).
	Nav         string
	Prefill     func(r *http.Request, d Draft)
	DeleteStmts func(e content.Entry) []store.Stmt
}

func (k Kind) nav() string {
	if k.Nav != "" {
		return k.Nav
	}
	return k.Section
}

// Admin serves one kind's list and editor.
type Admin struct {
	k    Kind
	db   store.DB
	ui   *admin.UI
	auth *auth.Auth
	jobs *jobs.Jobs
	list *template.Template
	edit *template.Template
}

// New parses the templates for k.
func New(k Kind, db store.DB, u *admin.UI, a *auth.Auth, j *jobs.Jobs) (*Admin, error) {
	list, err := u.Parse(k.Section, "templates/admin/entry_list.html")
	if err != nil {
		return nil, err
	}
	edit, err := u.Parse(k.Section+"-edit", append([]string{"templates/admin/entry_edit.html", k.Fields}, k.Extra...)...)
	if err != nil {
		return nil, err
	}
	return &Admin{k: k, db: db, ui: u, auth: a, jobs: j, list: list, edit: edit}, nil
}

func (h *Admin) base() string { return "/admin/" + h.k.Section }

// Register adds the routes. They are literal per kind, so they sit next to
// the kind's other admin routes (/admin/links/session) without overlap.
func (h *Admin) Register(mux *http.ServeMux) {
	s := func(f http.HandlerFunc) http.Handler { return h.auth.RequireSession(f) }
	w := func(f http.HandlerFunc) http.Handler { return admin.Guard(s(f)) }
	b := h.base()
	mux.Handle("GET "+b, s(h.listPage))
	mux.Handle("GET "+b+"/new", s(h.newPage))
	mux.Handle("POST "+b, w(h.create))
	mux.Handle("GET "+b+"/{id}", s(h.editPage))
	mux.Handle("POST "+b+"/{id}", w(h.save))
	mux.Handle("POST "+b+"/{id}/publish", w(h.publish))
	mux.Handle("POST "+b+"/{id}/delete", admin.Guard(h.auth.RequireSession(h.auth.RequireRecentAuth(http.HandlerFunc(h.delete)))))
}

// --- List -------------------------------------------------------------------

type listRow struct {
	Href, Title, Lang, Status, StatusKey, Updated string
}

type listPage struct {
	admin.Shell
	Section, Kind  string
	Rows           []listRow
	Status, Filter string // Filter is the language
	Statuses       []string
	Notice         string
}

var statuses = []string{content.Draft, content.Published, content.Archived}

func (h *Admin) listPage(w http.ResponseWriter, r *http.Request) {
	lang := i18n.FromRequest(r)
	q := r.URL.Query()
	p := listPage{Shell: h.ui.Shell(r, i18n.T(lang, "admin.nav."+h.k.Section), h.k.nav()),
		Section: h.k.Section, Kind: h.k.Name, Statuses: statuses, Status: q.Get("status"), Filter: q.Get("lang")}
	where, args := "e.kind = ?", []any{h.k.Name}
	if contains(statuses, p.Status) {
		where += " AND e.status = ?"
		args = append(args, p.Status)
	} else {
		p.Status = ""
	}
	if i18n.IsSupported(p.Filter) {
		where += " AND e.lang = ?"
		args = append(args, p.Filter)
	} else {
		p.Filter = ""
	}
	if s := q.Get("deleted"); s != "" {
		p.Notice = fmt.Sprintf(i18n.T(lang, "entry.deleted"), s)
	}
	// The body's start names untitled notes.
	res, err := h.db.Query(r.Context(), `SELECT e.id, e.title, e.summary, e.slug, e.lang, e.status, e.updated_at, substr(r.body_md, 1, 120) AS body
FROM entries e LEFT JOIN revisions r ON r.id = e.current_revision_id
WHERE `+where+` ORDER BY e.updated_at DESC LIMIT 200`, args...)
	if err != nil {
		h.fail(w, err)
		return
	}
	rows, err := store.Scan[struct {
		ID        string  `json:"id"`
		Title     string  `json:"title"`
		Summary   string  `json:"summary"`
		Slug      string  `json:"slug"`
		Lang      string  `json:"lang"`
		Status    string  `json:"status"`
		UpdatedAt int64   `json:"updated_at"`
		Body      *string `json:"body"`
	}](res)
	if err != nil {
		h.fail(w, err)
		return
	}
	now := content.Clock().Unix()
	for _, x := range rows {
		t := firstOf(x.Title, x.Summary)
		if t == "" && x.Body != nil {
			t = firstOf(*x.Body)
		}
		if t == "" {
			t = i18n.T(lang, "admin.untitled") + " · " + x.Slug
		}
		p.Rows = append(p.Rows, listRow{Href: h.base() + "/" + x.ID, Title: t, Lang: x.Lang, StatusKey: x.Status,
			Status: i18n.T(lang, "admin.status."+x.Status), Updated: admin.Ago(lang, x.UpdatedAt, now)})
	}
	h.ui.Render(w, http.StatusOK, h.list, p)
}

// firstOf is the first non-empty line, cut to 80 characters.
func firstOf(ss ...string) string {
	for _, s := range ss {
		s = strings.TrimSpace(s)
		if i := strings.IndexByte(s, '\n'); i >= 0 {
			s = strings.TrimSpace(s[:i])
		}
		s = strings.TrimLeft(s, "#> ")
		if s == "" {
			continue
		}
		if utf8.RuneCountInString(s) > 80 {
			s = string([]rune(s)[:79]) + "…"
		}
		return s
	}
	return ""
}

// --- Editor -----------------------------------------------------------------

type editPage struct {
	admin.Shell
	Section, Kind string
	New           bool
	Action        string // the form's action
	Item          Draft
	Entry         content.Entry
	Markdown      string
	TagsValue     string
	Body          bool
	Errors        []string
	Options       any
	Langs         []string
	PublicHref    string // the public page, once published
	NotLive       bool   // published, with a saved body not yet live
	Reauthed      bool
	Notice        string
}

func (h *Admin) page(r *http.Request, d Draft, isNew bool, errs []string) (editPage, error) {
	lang := i18n.FromRequest(r)
	e := *d.Base()
	title := i18n.T(lang, "kind."+h.k.Name+".new")
	if !isNew {
		title = firstOf(e.Title, e.Summary, *d.Text())
		if title == "" {
			title = i18n.T(lang, "admin.untitled") + " · " + e.Slug
		}
	}
	p := editPage{Shell: h.ui.Shell(r, title, h.k.nav()), Section: h.k.Section, Kind: h.k.Name, New: isNew,
		Item: d, Entry: e, Markdown: *d.Text(), TagsValue: strings.Join(*d.TagList(), ", "), Body: h.k.Body,
		Errors: errs, Langs: i18n.Supported, Action: h.base()}
	if !isNew {
		p.Action = h.base() + "/" + e.ID
		if e.Status == content.Published {
			if h.k.Public != nil {
				p.PublicHref = h.k.Public(d)
			}
			// Only the body waits for publish; links and favs have none, and
			// their fields are live on save.
			p.NotLive = h.k.Body && (e.PublishedRevisionID == nil || e.CurrentRevisionID == nil || *e.PublishedRevisionID != *e.CurrentRevisionID)
		}
	}
	if h.k.Options != nil {
		o, err := h.k.Options(r.Context(), lang, d)
		if err != nil {
			return p, err
		}
		p.Options = o
	}
	return p, nil
}

func (h *Admin) newPage(w http.ResponseWriter, r *http.Request) {
	d := h.k.New()
	d.Base().Lang = i18n.FromRequest(r)
	if h.k.Prefill != nil {
		h.k.Prefill(r, d)
	}
	p, err := h.page(r, d, true, nil)
	if err != nil {
		h.fail(w, err)
		return
	}
	h.ui.Render(w, http.StatusOK, h.edit, p)
}

// load reads the entry with its current text and tags.
func (h *Admin) load(w http.ResponseWriter, r *http.Request) (Draft, bool) {
	d, err := h.k.Load(r.Context(), r.PathValue("id"))
	if err == nil && d.Base().Kind != h.k.Name {
		err = content.NotFound(h.k.Name, r.PathValue("id"))
	}
	if errors.Is(err, content.ErrNotFound) {
		http.NotFound(w, r)
		return nil, false
	}
	if err == nil {
		err = h.stored(r.Context(), d)
	}
	if err != nil {
		h.fail(w, err)
		return nil, false
	}
	return d, true
}

func (h *Admin) stored(ctx context.Context, d Draft) error {
	e := d.Base()
	if e.CurrentRevisionID != nil {
		rev, err := content.GetRevision(ctx, h.db, *e.CurrentRevisionID)
		if err != nil {
			return err
		}
		*d.Text() = rev.BodyMD
	}
	tags, err := content.TagsOf(ctx, h.db, e.ID)
	if err != nil {
		return err
	}
	out := make([]string, len(tags))
	for i, t := range tags {
		out[i] = t.Slug
	}
	*d.TagList() = out
	return nil
}

func (h *Admin) editPage(w http.ResponseWriter, r *http.Request) {
	d, ok := h.load(w, r)
	if !ok {
		return
	}
	p, err := h.page(r, d, false, nil)
	if err != nil {
		h.fail(w, err)
		return
	}
	lang := i18n.FromRequest(r)
	q := r.URL.Query()
	p.Reauthed = q.Get("publish") == "1" && h.auth.Recent(r)
	switch {
	case q.Get("published") == "1":
		p.Notice = i18n.T(lang, "entry.published")
	case q.Get("saved") == "1":
		p.Notice = i18n.T(lang, "entry.saved")
	}
	h.ui.Render(w, http.StatusOK, h.edit, p)
}

// read fills d from the form: the shared fields here, the kind's own in Read.
// Fields the form does not show (the body of a link) keep their stored value.
func (h *Admin) read(r *http.Request, d Draft, isNew bool) {
	e := d.Base()
	e.Title = strings.TrimSpace(r.FormValue("title"))
	// An emptied slug keeps the stored one; a new entry gets one (slugs).
	if v := strings.TrimSpace(r.FormValue("slug")); v != "" || isNew {
		e.Slug = v
	}
	e.Summary = strings.TrimSpace(strings.ReplaceAll(r.FormValue("summary"), "\r\n", "\n"))
	if isNew {
		e.Lang = r.FormValue("lang")
	}
	if h.k.Body {
		*d.Text() = strings.ReplaceAll(r.FormValue("markdown"), "\r\n", "\n")
	}
	var tags []string
	for _, t := range strings.Split(r.FormValue("tags"), ",") {
		if t = content.NormalizeTag(t); t != "" {
			tags = append(tags, t)
		}
	}
	*d.TagList() = tags
	h.k.Read(r, d)
}

// slugs are the candidates for a new entry without a typed slug: from the
// title, else the text, else the kind and the date; a taken one moves on.
func (h *Admin) slugs(d Draft) []string {
	e := d.Base()
	if e.Slug != "" {
		return []string{e.Slug}
	}
	base := content.Slugify(e.Title)
	if base == "" {
		base = content.Slugify(strings.Join(strings.Fields(firstOf(e.Summary, *d.Text())), " "))
	}
	if base == "" {
		base = h.k.Name + "-" + content.Clock().UTC().Format("2006-01-02")
	}
	if len(base) > 60 {
		base = strings.TrimRight(base[:60], "-")
	}
	out := []string{base}
	for i := 2; i <= 9; i++ {
		out = append(out, fmt.Sprintf("%s-%d", base, i))
	}
	return out
}

func (h *Admin) create(w http.ResponseWriter, r *http.Request) {
	d := h.k.New()
	h.read(r, d, true)
	var id string
	var err error
	for _, slug := range h.slugs(d) {
		d.Base().Slug = slug
		if id, err = h.k.Create(r.Context(), d); err == nil {
			break
		}
		var v content.ValidationError
		if !errors.As(err, &v) || !strings.Contains(v["slug"], "already used") || len(v) > 1 || r.FormValue("slug") != "" {
			break
		}
	}
	if err != nil {
		if r.FormValue("slug") == "" {
			d.Base().Slug = ""
		}
		h.formError(w, r, d, true, err)
		return
	}
	h.enqueue(r.Context(), id, nil)
	if r.FormValue("action") == "publish" {
		h.finishPublish(w, r, id, nil)
		return
	}
	http.Redirect(w, r, h.base()+"/"+id+"?saved=1", http.StatusSeeOther)
}

func (h *Admin) save(w http.ResponseWriter, r *http.Request) {
	d, ok := h.load(w, r)
	if !ok {
		return
	}
	prev, _ := jobs.RefOf(r.Context(), h.db, d.Base().ID)
	h.read(r, d, false)
	if err := h.k.Save(r.Context(), d, content.OriginManual); err != nil {
		h.formError(w, r, d, false, err)
		return
	}
	h.enqueue(r.Context(), d.Base().ID, prev)
	http.Redirect(w, r, h.base()+"/"+d.Base().ID+"?saved=1", http.StatusSeeOther)
}

// publish saves the form, then publishes if the sign-in is recent; else the
// browser goes through re-auth back to the editor, which offers to finish.
// The "publish now" button shown after re-auth posts now=1 and no fields.
func (h *Admin) publish(w http.ResponseWriter, r *http.Request) {
	d, ok := h.load(w, r)
	if !ok {
		return
	}
	id := d.Base().ID
	prev, _ := jobs.RefOf(r.Context(), h.db, id)
	if r.FormValue("now") != "1" {
		origin := content.OriginManual
		if h.auth.Recent(r) {
			origin = content.OriginPublish
		}
		h.read(r, d, false)
		if err := h.k.Save(r.Context(), d, origin); err != nil {
			h.formError(w, r, d, false, err)
			return
		}
	}
	h.finishPublish(w, r, id, prev)
}

func (h *Admin) finishPublish(w http.ResponseWriter, r *http.Request, id string, prev *jobs.Ref) {
	edit := h.base() + "/" + id
	if !h.auth.Recent(r) {
		h.enqueue(r.Context(), id, prev)
		http.Redirect(w, r, auth.ReauthPath(edit+"?publish=1"), http.StatusSeeOther)
		return
	}
	if err := h.k.Publish(r.Context(), id); err != nil {
		var v content.ValidationError
		if errors.As(err, &v) {
			d, lerr := h.k.Load(r.Context(), id)
			if lerr == nil {
				lerr = h.stored(r.Context(), d)
			}
			if lerr != nil {
				h.fail(w, lerr)
				return
			}
			h.formError(w, r, d, false, err)
			return
		}
		h.fail(w, err)
		return
	}
	h.enqueue(r.Context(), id, prev)
	http.Redirect(w, r, edit+"?published=1", http.StatusSeeOther)
}

// formError shows the form again with the typed values and the problems.
func (h *Admin) formError(w http.ResponseWriter, r *http.Request, d Draft, isNew bool, err error) {
	var v content.ValidationError
	if !errors.As(err, &v) {
		h.fail(w, err)
		return
	}
	p, perr := h.page(r, d, isNew, Errors(i18n.FromRequest(r), v))
	if perr != nil {
		h.fail(w, perr)
		return
	}
	h.ui.Render(w, http.StatusUnprocessableEntity, h.edit, p)
}

// delete removes the entry. A published item with its own page leaves a
// 410 there, so the page and its links say gone instead of 404.
func (h *Admin) delete(w http.ResponseWriter, r *http.Request) {
	d, ok := h.load(w, r)
	if !ok {
		return
	}
	e := *d.Base()
	prev, err := jobs.RefOf(r.Context(), h.db, e.ID)
	if err != nil {
		h.fail(w, err)
		return
	}
	var stmts []store.Stmt
	if e.Status == content.Published && h.k.Public != nil {
		if from := h.k.Public(d); from != "" {
			stmts = append(stmts, store.Stmt{SQL: `INSERT INTO redirects (from_path, to_path, code, created_at) VALUES (?, '', 410, ?)
ON CONFLICT (from_path) DO UPDATE SET to_path = '', code = 410`, Args: []any{from, content.Clock().Unix()}})
		}
	}
	if h.k.DeleteStmts != nil {
		stmts = append(stmts, h.k.DeleteStmts(e)...)
	} else {
		stmts = append(stmts, store.Stmt{SQL: `DELETE FROM entries WHERE id = ? AND kind = ?`, Args: []any{e.ID, h.k.Name}})
	}
	if _, err := h.db.Batch(r.Context(), stmts...); err != nil {
		h.fail(w, err)
		return
	}
	h.enqueue(r.Context(), e.ID, prev)
	http.Redirect(w, r, h.base()+"?deleted="+url.QueryEscape(e.Slug), http.StatusSeeOther)
}

func (h *Admin) enqueue(ctx context.Context, id string, prev *jobs.Ref) {
	if err := jobs.Enqueue(ctx, h.jobs, id, prev); err != nil {
		slog.Error("enqueue entry.changed", "entry", id, "err", err)
	}
}

func (h *Admin) fail(w http.ResponseWriter, err error) {
	slog.Error(h.k.Name+" admin", "err", err)
	http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
}

// Errors turns validation errors into catalog messages: "admin.err.<field>"
// when the catalog has one, else the field and the message. File fields
// (files.N.name) name the file by its position.
func Errors(lang string, v content.ValidationError) []string {
	keys := make([]string, 0, len(v))
	for k := range v {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var out []string
	for _, f := range keys {
		msg := v[f]
		key := "admin.err." + f
		if f == "slug" && strings.Contains(msg, "already used") {
			key = "admin.err.slug_taken"
		}
		if t := i18n.T(lang, key); t != key {
			out = append(out, t)
			continue
		}
		// Row fields (files.0.filename, parts.2.price) name the group and
		// the row by its position.
		if parts := strings.Split(f, "."); len(parts) == 3 {
			if n, err := strconv.Atoi(parts[1]); err == nil {
				group := i18n.T(lang, "entry.row."+parts[0])
				if group == "entry.row."+parts[0] {
					group = parts[0]
				}
				out = append(out, fmt.Sprintf(i18n.T(lang, "entry.err.row"), group, n+1, msg))
				continue
			}
		}
		out = append(out, f+": "+msg)
	}
	return out
}

// Option is one choice in a select.
type Option struct {
	Value, Label string
	On           bool
}

// CategoryOptions are kind's categories, children after their parent, with
// cur selected; the first option is "none".
func CategoryOptions(ctx context.Context, db store.DB, kind, lang string, cur *string) ([]Option, error) {
	cats, err := content.Categories(ctx, db, kind)
	if err != nil {
		return nil, err
	}
	out := []Option{{Value: "", Label: i18n.T(lang, "entry.no_category"), On: cur == nil}}
	add := func(c content.Category, prefix string) {
		out = append(out, Option{Value: c.ID, Label: prefix + c.Name(lang), On: cur != nil && *cur == c.ID})
	}
	for _, c := range cats {
		if c.ParentID != nil {
			continue
		}
		add(c, "")
		for _, ch := range cats {
			if ch.ParentID != nil && *ch.ParentID == c.ID {
				add(ch, "↳ ")
			}
		}
	}
	return out, nil
}

// Ptr is the form value as a nullable id.
func Ptr(r *http.Request, name string) *string {
	if v := strings.TrimSpace(r.FormValue(name)); v != "" {
		return &v
	}
	return nil
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

// Rows reads repeated form rows: each field name appears once per row, in
// row order, and order names the field that sorts them (ties keep the form
// order). Rows whose fields are all blank are dropped, so an empty row in
// the form adds one and emptying a row removes it, without JS.
func Rows(r *http.Request, order string, fields ...string) [][]string {
	_ = r.ParseForm()
	cols := make([][]string, len(fields))
	n := 0
	for i, f := range fields {
		cols[i] = r.PostForm[f]
		n = max(n, len(cols[i]))
	}
	orders := r.PostForm[order]
	type row struct {
		vals  []string
		order int
	}
	var rows []row
	for i := 0; i < n; i++ {
		vals := make([]string, len(fields))
		blank := true
		for j := range fields {
			if i < len(cols[j]) {
				vals[j] = strings.TrimSpace(cols[j][i])
			}
			if vals[j] != "" {
				blank = false
			}
		}
		if blank {
			continue
		}
		o := i
		if i < len(orders) {
			if v, err := strconv.Atoi(strings.TrimSpace(orders[i])); err == nil {
				o = v
			}
		}
		rows = append(rows, row{vals, o})
	}
	sort.SliceStable(rows, func(a, b int) bool { return rows[a].order < rows[b].order })
	out := make([][]string, len(rows))
	for i, x := range rows {
		out[i] = x.vals
	}
	return out
}
