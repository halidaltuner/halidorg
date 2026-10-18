// Package taxonomy is the admin for categories and tags (derive, dashboard
// density, DESIGN.md §7): create categories, rename both in TR and EN, pick
// a DESIGN.md tone, reorder categories, merge tags. Every change re-renders
// the entries it touches through entry.changed, so links pages, awesome
// lists, tag pages and entry pages show the new names.
package taxonomy

import (
	"context"
	"errors"
	"html/template"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"halid.org/engine/internal/admin"
	"halid.org/engine/internal/auth"
	"halid.org/engine/internal/content"
	"halid.org/engine/internal/i18n"
	"halid.org/engine/internal/jobs"
	"halid.org/engine/internal/store"
)

// Handlers serves /admin/categories and /admin/tags.
type Handlers struct {
	db   store.DB
	ui   *admin.UI
	auth *auth.Auth
	jobs *jobs.Jobs
	cats *template.Template
	tags *template.Template
}

// New parses the templates.
func New(db store.DB, u *admin.UI, a *auth.Auth, j *jobs.Jobs) (*Handlers, error) {
	cats, err := u.Parse("categories", "templates/admin/dashboard.html", "templates/taxonomy/tone.html", "templates/taxonomy/categories.html")
	if err != nil {
		return nil, err
	}
	tags, err := u.Parse("tags", "templates/admin/dashboard.html", "templates/taxonomy/tone.html", "templates/taxonomy/tags.html")
	if err != nil {
		return nil, err
	}
	return &Handlers{db: db, ui: u, auth: a, jobs: j, cats: cats, tags: tags}, nil
}

// Register adds the routes; writes go through admin.Guard in a session.
func (h *Handlers) Register(mux *http.ServeMux) {
	s := func(f http.HandlerFunc) http.Handler { return h.auth.RequireSession(f) }
	w := func(f http.HandlerFunc) http.Handler { return admin.Guard(s(f)) }
	mux.Handle("GET /admin/categories", s(h.categories))
	mux.Handle("POST /admin/categories", w(h.createCategory))
	mux.Handle("POST /admin/categories/{id}", w(h.saveCategory))
	mux.Handle("POST /admin/categories/{id}/move", w(h.moveCategory))
	mux.Handle("GET /admin/tags", s(h.tagsPage))
	mux.Handle("POST /admin/tags/merge", w(h.mergeTags))
	mux.Handle("POST /admin/tags/{id}", w(h.saveTag))
}

// kinds that have categories now, in tab order (homelab: Phase 4).
var kinds = []string{"link", "snippet", "fav"}

type tab struct {
	Kind, Label, Href string
	On                bool
}

type catRow struct {
	content.Category
	Entries  int
	Children bool
	First    bool
	Last     bool
}

type catPage struct {
	admin.Shell
	Kind    string
	Tabs    []tab
	Rows    []catRow
	Parents []content.Category // top-level categories of the kind
	Tones   []string
	Notice  string
	Error   string
	New     content.Category // the create form, kept on an error
}

func tone(r *http.Request) *string {
	t := r.FormValue("tone")
	if t == "" {
		return nil
	}
	return &t
}

func parent(r *http.Request) *string {
	p := r.FormValue("parent")
	if p == "" {
		return nil
	}
	return &p
}

func kindOf(r *http.Request) string {
	k := r.FormValue("kind")
	for _, x := range kinds {
		if x == k {
			return k
		}
	}
	return kinds[0]
}

func (h *Handlers) catPage(r *http.Request, kind string) (catPage, error) {
	lang := i18n.FromRequest(r)
	p := catPage{Shell: h.ui.Shell(r, i18n.T(lang, "admin.cat.title"), "categories"), Kind: kind, Tones: content.Tones}
	for _, k := range kinds {
		p.Tabs = append(p.Tabs, tab{Kind: k, Label: i18n.T(lang, "admin.nav."+k+"s"), Href: "/admin/categories?kind=" + k, On: k == kind})
	}
	cats, err := content.Categories(r.Context(), h.db, kind)
	if err != nil {
		return p, err
	}
	counts, err := h.counts(r.Context(), kind)
	if err != nil {
		return p, err
	}
	hasChildren := map[string]bool{}
	for _, c := range cats {
		if c.ParentID != nil {
			hasChildren[*c.ParentID] = true
		}
	}
	for i, c := range cats {
		p.Rows = append(p.Rows, catRow{Category: c, Entries: counts[c.ID], Children: hasChildren[c.ID], First: i == 0, Last: i == len(cats)-1})
		if c.ParentID == nil {
			p.Parents = append(p.Parents, c)
		}
	}
	p.New.Kind = kind
	return p, nil
}

func (h *Handlers) counts(ctx context.Context, kind string) (map[string]int, error) {
	table := map[string]string{"link": "links", "snippet": "snippets", "fav": "favs"}[kind]
	res, err := h.db.Query(ctx, `SELECT category_id AS id, COUNT(*) AS n FROM `+table+` WHERE category_id IS NOT NULL GROUP BY category_id`)
	if err != nil {
		return nil, err
	}
	rows, err := store.Scan[struct {
		ID string `json:"id"`
		N  int    `json:"n"`
	}](res)
	out := map[string]int{}
	for _, x := range rows {
		out[x.ID] = x.N
	}
	return out, err
}

func (h *Handlers) categories(w http.ResponseWriter, r *http.Request) {
	p, err := h.catPage(r, kindOf(r))
	if err != nil {
		h.fail(w, err)
		return
	}
	if r.URL.Query().Get("saved") != "" {
		p.Notice = i18n.T(p.Lang, "admin.tax.saved")
	}
	h.ui.Render(w, http.StatusOK, h.cats, p)
}

// back returns to the page with a notice, so a reload does not post again.
func back(w http.ResponseWriter, r *http.Request, path string) {
	http.Redirect(w, r, path+"&saved=1", http.StatusSeeOther)
}

func message(err error) string {
	var v content.ValidationError
	if errors.As(err, &v) {
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		parts := make([]string, len(keys))
		for i, k := range keys {
			parts[i] = k + ": " + v[k]
		}
		return strings.Join(parts, "; ")
	}
	return ""
}

func (h *Handlers) createCategory(w http.ResponseWriter, r *http.Request) {
	kind := kindOf(r)
	c := content.Category{Kind: kind, Slug: strings.TrimSpace(r.FormValue("slug")), NameTR: strings.TrimSpace(r.FormValue("name_tr")),
		NameEN: strings.TrimSpace(r.FormValue("name_en")), Tone: tone(r), ParentID: parent(r), Sort: 1000}
	if c.Slug == "" {
		c.Slug = content.Slugify(c.NameEN + " " + c.NameTR)
	}
	err := h.checkParent(r.Context(), c)
	if err == nil {
		err = content.CreateCategory(r.Context(), h.db, &c)
	}
	if msg := message(err); msg != "" {
		p, perr := h.catPage(r, kind)
		if perr != nil {
			h.fail(w, perr)
			return
		}
		p.Error, p.New = msg, c
		h.ui.Render(w, http.StatusUnprocessableEntity, h.cats, p)
		return
	}
	if err != nil {
		h.fail(w, err)
		return
	}
	// A new category has no entries yet: nothing public changes.
	back(w, r, "/admin/categories?kind="+url.QueryEscape(kind))
}

// checkParent applies UpdateCategory's parent rule to a new category.
func (h *Handlers) checkParent(ctx context.Context, c content.Category) error {
	if c.ParentID == nil {
		return nil
	}
	p, err := content.GetCategory(ctx, h.db, *c.ParentID)
	if errors.Is(err, content.ErrNotFound) || (err == nil && (p.Kind != c.Kind || p.ParentID != nil)) {
		return content.ValidationError{"parent": "a top-level category of the same kind"}
	}
	return err
}

func (h *Handlers) saveCategory(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	c, err := content.GetCategory(ctx, h.db, r.PathValue("id"))
	if errors.Is(err, content.ErrNotFound) {
		http.NotFound(w, r)
		return
	} else if err != nil {
		h.fail(w, err)
		return
	}
	c.NameTR, c.NameEN, c.Tone, c.ParentID = r.FormValue("name_tr"), r.FormValue("name_en"), tone(r), parent(r)
	ids, prevs, err := h.snapshot(ctx, content.CategoryEntries, c.ID)
	if err != nil {
		h.fail(w, err)
		return
	}
	if err := content.UpdateCategory(ctx, h.db, c); message(err) != "" {
		p, perr := h.catPage(r, c.Kind)
		if perr != nil {
			h.fail(w, perr)
			return
		}
		p.Error = c.Slug + ": " + message(err)
		h.ui.Render(w, http.StatusUnprocessableEntity, h.cats, p)
		return
	} else if err != nil {
		h.fail(w, err)
		return
	}
	// A new parent also changes the old parent's awesome list: the
	// entries' refs before and after name both.
	h.enqueue(ctx, ids, prevs)
	back(w, r, "/admin/categories?kind="+url.QueryEscape(c.Kind))
}

func (h *Handlers) moveCategory(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	c, err := content.GetCategory(ctx, h.db, r.PathValue("id"))
	if errors.Is(err, content.ErrNotFound) {
		http.NotFound(w, r)
		return
	} else if err != nil {
		h.fail(w, err)
		return
	}
	dir := 1
	if r.FormValue("dir") == "up" {
		dir = -1
	}
	if err := content.MoveCategory(ctx, h.db, c.ID, dir); err != nil {
		h.fail(w, err)
		return
	}
	// Order shows on the links page and the awesome index of each
	// language the category's entries are in.
	ids, prevs, err := h.snapshot(ctx, content.CategoryEntries, c.ID)
	if err != nil {
		h.fail(w, err)
		return
	}
	h.enqueue(ctx, ids, prevs)
	back(w, r, "/admin/categories?kind="+url.QueryEscape(c.Kind))
}

// --- Tags ---------------------------------------------------------------

type tagPage struct {
	admin.Shell
	Rows   []content.TagRow
	Tones  []string
	Notice string
	Error  string
}

func (h *Handlers) tagPage(r *http.Request) (tagPage, error) {
	lang := i18n.FromRequest(r)
	rows, err := content.AllTags(r.Context(), h.db)
	return tagPage{Shell: h.ui.Shell(r, i18n.T(lang, "admin.tag.title"), "tags"), Rows: rows, Tones: content.Tones}, err
}

func (h *Handlers) tagsPage(w http.ResponseWriter, r *http.Request) {
	p, err := h.tagPage(r)
	if err != nil {
		h.fail(w, err)
		return
	}
	if r.URL.Query().Get("saved") != "" {
		p.Notice = i18n.T(p.Lang, "admin.tax.saved")
	}
	h.ui.Render(w, http.StatusOK, h.tags, p)
}

func (h *Handlers) tagError(w http.ResponseWriter, r *http.Request, msg string) {
	p, err := h.tagPage(r)
	if err != nil {
		h.fail(w, err)
		return
	}
	p.Error = msg
	h.ui.Render(w, http.StatusUnprocessableEntity, h.tags, p)
}

func (h *Handlers) saveTag(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	t := content.Tag{ID: r.PathValue("id"), NameTR: r.FormValue("name_tr"), NameEN: r.FormValue("name_en"), Tone: tone(r)}
	ids, prevs, err := h.snapshot(ctx, content.TagEntries, t.ID)
	if err != nil {
		h.fail(w, err)
		return
	}
	err = content.UpdateTag(ctx, h.db, t)
	switch {
	case errors.Is(err, content.ErrNotFound):
		http.NotFound(w, r)
		return
	case message(err) != "":
		h.tagError(w, r, message(err))
		return
	case err != nil:
		h.fail(w, err)
		return
	}
	h.enqueue(ctx, ids, prevs)
	back(w, r, "/admin/tags?")
}

// mergeTags moves every entry of "from" onto "into". The refs are taken
// first, so the old tag's pages are purged with the entries' new ones.
func (h *Handlers) mergeTags(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	from, into := r.FormValue("from"), r.FormValue("into")
	ids, prevs, err := h.snapshot(ctx, content.TagEntries, from)
	if err != nil {
		h.fail(w, err)
		return
	}
	err = content.MergeTags(ctx, h.db, from, into)
	switch {
	case errors.Is(err, content.ErrNotFound):
		h.tagError(w, r, i18n.T(i18n.FromRequest(r), "admin.tag.merge_missing"))
		return
	case message(err) != "":
		h.tagError(w, r, i18n.T(i18n.FromRequest(r), "admin.tag.merge_same"))
		return
	case err != nil:
		h.fail(w, err)
		return
	}
	h.enqueue(ctx, ids, prevs)
	back(w, r, "/admin/tags?")
}

// --- Jobs ---------------------------------------------------------------

// maxJobs caps the render jobs one change queues; a tag on more entries
// than this re-renders the rest on their next edit or deploy refresh.
const maxJobs = 200

// snapshot reads the entries a change touches and their refs before it.
func (h *Handlers) snapshot(ctx context.Context, list func(context.Context, store.DB, string) ([]string, error), id string) ([]string, []*jobs.Ref, error) {
	ids, err := list(ctx, h.db, id)
	if err != nil {
		return nil, nil, err
	}
	if len(ids) > maxJobs {
		slog.Warn("taxonomy: capped render jobs", "id", id, "entries", len(ids))
		ids = ids[:maxJobs]
	}
	prevs := make([]*jobs.Ref, len(ids))
	for i, e := range ids {
		if prevs[i], err = jobs.RefOf(ctx, h.db, e); err != nil {
			return nil, nil, err
		}
	}
	return ids, prevs, nil
}

func (h *Handlers) enqueue(ctx context.Context, ids []string, prevs []*jobs.Ref) {
	for i, id := range ids {
		var prev *jobs.Ref
		if prevs != nil {
			prev = prevs[i]
		}
		if err := jobs.Enqueue(ctx, h.jobs, id, prev); err != nil {
			slog.Error("enqueue entry.changed", "entry", id, "err", err)
		}
	}
}

func (h *Handlers) fail(w http.ResponseWriter, err error) {
	slog.Error("taxonomy admin", "err", err)
	http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
}
