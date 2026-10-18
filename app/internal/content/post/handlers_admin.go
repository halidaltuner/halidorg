package post

import (
	"context"
	"errors"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"halid.org/engine/internal/admin"
	"halid.org/engine/internal/auth"
	"halid.org/engine/internal/content"
	"halid.org/engine/internal/i18n"
	"halid.org/engine/internal/jobs"
	"halid.org/engine/internal/render"
	"halid.org/engine/internal/render/og"
	"halid.org/engine/internal/store"
)

// Admin serves the post list and the editor (design/Admin Editor.dc.html,
// DESIGN.md §7). Every form works as a plain POST; htmx upgrades autosave,
// save, preview and the job retry to fragments.
type Admin struct {
	repo   *Repo
	db     store.DB
	ui     *admin.UI
	auth   *auth.Auth
	jobs   *jobs.Jobs
	origin string
	list   *template.Template
	newT   *template.Template
	editor *template.Template
}

// NewAdmin parses the post admin templates.
func NewAdmin(db store.DB, u *admin.UI, a *auth.Auth, j *jobs.Jobs) (*Admin, error) {
	list, err := u.Parse("posts", "templates/admin/dashboard.html", "templates/post/admin_list.html")
	if err != nil {
		return nil, err
	}
	newT, err := u.Parse("post-new", "templates/admin/dashboard.html", "templates/post/admin_new.html")
	if err != nil {
		return nil, err
	}
	// The editor is full height without the sidebar, so it skips the shell.
	editor, err := u.ParseBare("post-editor", "templates/post/editor.html")
	if err != nil {
		return nil, err
	}
	return &Admin{repo: NewRepo(db), db: db, ui: u, auth: a, jobs: j,
		origin: strings.TrimRight(os.Getenv("PUBLIC_ORIGIN"), "/"),
		list:   list, newT: newT, editor: editor}, nil
}

// Register adds the post admin routes. Writes go through admin.Guard (CSRF);
// delete also needs a recent sign-in. Publish checks that itself, after
// saving, so a re-auth never loses what was typed.
func (h *Admin) Register(mux *http.ServeMux) {
	s := func(f http.HandlerFunc) http.Handler { return h.auth.RequireSession(f) }
	w := func(f http.HandlerFunc) http.Handler { return admin.Guard(s(f)) }
	mux.Handle("GET /admin/posts", s(h.listPage))
	mux.Handle("GET /admin/posts/new", s(h.newPage))
	mux.Handle("POST /admin/posts", w(h.create))
	mux.Handle("GET /admin/posts/{id}", s(h.edit))
	mux.Handle("POST /admin/posts/{id}", w(h.save))
	mux.Handle("POST /admin/posts/{id}/publish", w(h.publish))
	mux.Handle("POST /admin/posts/{id}/translate", w(h.translate))
	mux.Handle("POST /admin/posts/{id}/delete", admin.Guard(h.auth.RequireSession(h.auth.RequireRecentAuth(http.HandlerFunc(h.delete)))))
	mux.Handle("POST /admin/preview", w(h.preview))
}

// --- List and new -----------------------------------------------------------

type listRow struct {
	admin.EntryRow
	Lang, Status, Updated string
}

type listPage struct {
	admin.Shell
	Rows []listRow
}

func (h *Admin) listPage(w http.ResponseWriter, r *http.Request) {
	sh := h.ui.Shell(r, i18n.T(i18n.FromRequest(r), "admin.posts.title"), "posts")
	posts, err := h.repo.List(r.Context(), ListFilter{Limit: 100})
	if err != nil {
		h.fail(w, err)
		return
	}
	now := content.Clock().Unix()
	p := listPage{Shell: sh}
	for _, x := range posts {
		p.Rows = append(p.Rows, listRow{
			EntryRow: admin.EntryRow{Kind: Kind, Title: title(sh.Lang, x), Href: "/admin/posts/" + x.ID},
			Lang:     x.Lang, Status: i18n.T(sh.Lang, "admin.status."+x.Status), Updated: admin.Ago(sh.Lang, x.UpdatedAt, now),
		})
	}
	h.ui.Render(w, http.StatusOK, h.list, p)
}

type newPage struct {
	admin.Shell
	Form   newForm
	Errors []string
}

type newForm struct{ Title, Slug, Lang string }

func (h *Admin) newPage(w http.ResponseWriter, r *http.Request) {
	sh := h.ui.Shell(r, i18n.T(i18n.FromRequest(r), "admin.posts.new"), "posts")
	h.ui.Render(w, http.StatusOK, h.newT, newPage{Shell: sh, Form: newForm{Lang: sh.Lang}})
}

// create makes an empty draft and opens it in the editor.
func (h *Admin) create(w http.ResponseWriter, r *http.Request) {
	f := newForm{Title: strings.TrimSpace(r.FormValue("title")), Slug: strings.TrimSpace(r.FormValue("slug")), Lang: r.FormValue("lang")}
	if f.Slug == "" {
		f.Slug = content.Slugify(f.Title)
	}
	p, err := h.repo.Create(r.Context(), Draft{Post: Post{Entry: content.Entry{Title: f.Title, Slug: f.Slug, Lang: f.Lang}}})
	var v content.ValidationError
	if errors.As(err, &v) {
		sh := h.ui.Shell(r, i18n.T(i18n.FromRequest(r), "admin.posts.new"), "posts")
		h.ui.Render(w, http.StatusUnprocessableEntity, h.newT, newPage{Shell: sh, Form: f, Errors: errorList(sh.Lang, v)})
		return
	}
	if err != nil {
		h.fail(w, err)
		return
	}
	h.enqueue(r.Context(), p.ID, nil)
	http.Redirect(w, r, "/admin/posts/"+p.ID, http.StatusSeeOther)
}

// --- Editor -----------------------------------------------------------------

type editorPage struct {
	Lang, Env  string
	Dark       bool
	Post       Post
	Markdown   string
	Tags       []string
	TagsValue  string
	Words      int
	Status     string
	SaveLabel  string
	Errors     []string
	Preview    template.HTML
	Series     []seriesOpt
	Others     []otherLang
	Revisions  []revRow
	RevCount   int
	Backlinks  int
	CoverURL   string
	SEOLen     int
	Reauthed   bool // back from re-auth on the way to publishing
	OpenDelete bool
	DeleteErr  string
}

type seriesOpt struct {
	ID, Title string
	Selected  bool
}

type otherLang struct {
	Lang, Href, Status string
}

type revRow struct{ At, Origin string }

func (h *Admin) load(w http.ResponseWriter, r *http.Request) (Post, bool) {
	p, err := h.repo.Get(r.Context(), r.PathValue("id"))
	if errors.Is(err, content.ErrNotFound) {
		http.NotFound(w, r)
		return Post{}, false
	}
	if err != nil {
		h.fail(w, err)
		return Post{}, false
	}
	return p, true
}

func (h *Admin) edit(w http.ResponseWriter, r *http.Request) {
	p, ok := h.load(w, r)
	if !ok {
		return
	}
	md := ""
	if p.CurrentRevisionID != nil {
		rev, err := content.GetRevision(r.Context(), h.db, *p.CurrentRevisionID)
		if err != nil {
			h.fail(w, err)
			return
		}
		md = rev.BodyMD
	}
	tags, err := content.TagsOf(r.Context(), h.db, p.ID)
	if err != nil {
		h.fail(w, err)
		return
	}
	slugs := make([]string, len(tags))
	for i, t := range tags {
		slugs[i] = t.Slug
	}
	q := r.URL.Query()
	page, err := h.page(r, p, md, slugs, nil)
	if err != nil {
		h.fail(w, err)
		return
	}
	page.Reauthed = q.Get("publish") == "1" && h.auth.Recent(r)
	page.OpenDelete = q.Get("delete") == "1"
	h.ui.Render(w, http.StatusOK, h.editor, page)
}

// page builds the editor view for p with the given (possibly unsaved) text.
func (h *Admin) page(r *http.Request, p Post, md string, tags []string, errs []string) (editorPage, error) {
	ctx := r.Context()
	lang := i18n.FromRequest(r)
	sh := h.ui.Shell(r, "", "posts")
	pg := editorPage{
		Lang: lang, Env: sh.Env, Post: p, Markdown: md, Tags: tags, TagsValue: strings.Join(tags, ", "),
		Words: len(strings.Fields(md)), Status: p.Status, Errors: errs,
		SEOLen: utf8.RuneCountInString(p.Summary), SaveLabel: h.saveLabel(lang, p, ""),
	}
	if out, err := render.Markdown([]byte(md), render.Options{Lang: p.Lang}); err == nil {
		pg.Preview = template.HTML(out.HTML) // renderer output (ADR-002)
	}
	var err error
	if pg.Series, err = h.series(ctx, lang, p); err != nil {
		return pg, err
	}
	if pg.Others, err = h.others(ctx, lang, p); err != nil {
		return pg, err
	}
	if pg.Revisions, pg.RevCount, err = h.revisions(ctx, lang, p.ID); err != nil {
		return pg, err
	}
	bl, err := content.Backlinks(ctx, h.db, p.ID, false)
	if err != nil {
		return pg, err
	}
	pg.Backlinks = len(bl)
	if p.Status == content.Published && p.PublishedAt != nil {
		card := og.Post(p.Lang, p.Title, *p.PublishedAt, p.ReadingMinutes)
		pg.CoverURL = og.URL(og.EntryKey(p.Lang, "posts", p.Slug), card)
	}
	return pg, nil
}

func (h *Admin) saveLabel(lang string, p Post, origin string) string {
	at := h.ui.Time(p.UpdatedAt, "15:04")
	key := "admin.editor.saved"
	if origin == "" && p.Status == content.Published && samePtr(p.CurrentRevisionID, p.PublishedRevisionID) {
		origin = content.OriginPublish
	}
	switch origin {
	case content.OriginAutosave:
		key = "admin.editor.autosaved"
	case content.OriginPublish:
		key = "admin.editor.published"
	}
	label := fmt.Sprintf(i18n.T(lang, key), at)
	if p.Status == content.Published && !samePtr(p.CurrentRevisionID, p.PublishedRevisionID) {
		label += " · " + i18n.T(lang, "admin.editor.not_live")
	}
	return label
}

func samePtr(a, b *string) bool { return a != nil && b != nil && *a == *b }

func (h *Admin) series(ctx context.Context, lang string, p Post) ([]seriesOpt, error) {
	res, err := h.db.Query(ctx, `SELECT id, slug, title_tr, title_en FROM series WHERE status <> 'archived' ORDER BY slug`)
	if err != nil {
		return nil, err
	}
	rows, err := store.Scan[struct {
		ID      string `json:"id"`
		Slug    string `json:"slug"`
		TitleTR string `json:"title_tr"`
		TitleEN string `json:"title_en"`
	}](res)
	if err != nil {
		return nil, err
	}
	out := make([]seriesOpt, 0, len(rows))
	for _, s := range rows {
		t := s.TitleEN
		if lang == "tr" && s.TitleTR != "" || t == "" {
			t = s.TitleTR
		}
		if t == "" {
			t = s.Slug
		}
		out = append(out, seriesOpt{ID: s.ID, Title: t, Selected: p.SeriesID != nil && *p.SeriesID == s.ID})
	}
	return out, nil
}

func (h *Admin) others(ctx context.Context, lang string, p Post) ([]otherLang, error) {
	res, err := h.db.Query(ctx, `SELECT id, lang, status FROM entries WHERE translation_group_id = ? AND id <> ? AND kind = 'post'`, p.TranslationGroupID, p.ID)
	if err != nil {
		return nil, err
	}
	rows, err := store.Scan[struct {
		ID     string `json:"id"`
		Lang   string `json:"lang"`
		Status string `json:"status"`
	}](res)
	if err != nil {
		return nil, err
	}
	var out []otherLang
	for _, l := range i18n.Supported {
		if l == p.Lang {
			continue
		}
		o := otherLang{Lang: l}
		for _, x := range rows {
			if x.Lang == l {
				o.Href, o.Status = "/admin/posts/"+x.ID, i18n.T(lang, "admin.status."+x.Status)
			}
		}
		out = append(out, o)
	}
	return out, nil
}

func (h *Admin) revisions(ctx context.Context, lang, id string) ([]revRow, int, error) {
	revs, err := content.Revisions(ctx, h.db, id, 5)
	if err != nil {
		return nil, 0, err
	}
	res, err := h.db.Query(ctx, `SELECT COUNT(*) AS n FROM revisions WHERE entry_id = ?`, id)
	if err != nil {
		return nil, 0, err
	}
	n := 0
	if rows, _ := store.Scan[struct {
		N int `json:"n"`
	}](res); len(rows) == 1 {
		n = rows[0].N
	}
	now := content.Clock()
	out := make([]revRow, len(revs))
	for i, rv := range revs {
		out[i] = revRow{At: h.when(lang, rv.CreatedAt, now), Origin: i18n.T(lang, "admin.rev."+rv.Origin)}
	}
	return out, n, nil
}

// when is HH:MM today, "yesterday", else MM-DD, in the admin time zone.
func (h *Admin) when(lang string, sec int64, now time.Time) string {
	day := h.ui.Time(sec, "2006-01-02")
	switch day {
	case h.ui.Time(now.Unix(), "2006-01-02"):
		return h.ui.Time(sec, "15:04")
	case h.ui.Time(now.Add(-24*time.Hour).Unix(), "2006-01-02"):
		return i18n.T(lang, "admin.editor.yesterday")
	}
	return h.ui.Time(sec, "01-02")
}

// formDraft reads the editor form over the stored post.
func formDraft(r *http.Request, p Post) Draft {
	d := Draft{Post: p}
	d.Title = strings.TrimSpace(r.FormValue("title"))
	d.Slug = strings.TrimSpace(r.FormValue("slug"))
	d.Summary = strings.TrimSpace(strings.ReplaceAll(r.FormValue("summary"), "\r\n", "\n"))
	d.Markdown = strings.ReplaceAll(r.FormValue("markdown"), "\r\n", "\n")
	d.SeriesID, d.SeriesOrder = nil, nil
	if s := r.FormValue("series"); s != "" {
		d.SeriesID = &s
		if n, err := strconv.ParseInt(r.FormValue("series_order"), 10, 64); err == nil && n > 0 {
			d.SeriesOrder = &n
		}
	}
	for _, t := range strings.Split(r.FormValue("tags"), ",") {
		if t = content.NormalizeTag(t); t != "" {
			d.Tags = append(d.Tags, t)
		}
	}
	return d
}

// save writes the form as a revision: autosaves coalesce, a manual save
// always adds one (content.SaveRevision).
func (h *Admin) save(w http.ResponseWriter, r *http.Request) {
	p, ok := h.load(w, r)
	if !ok {
		return
	}
	origin := content.OriginManual
	if r.FormValue("origin") == content.OriginAutosave {
		origin = content.OriginAutosave
	}
	d := formDraft(r, p)
	prev, _ := jobs.RefOf(r.Context(), h.db, p.ID)
	saved, err := h.write(r.Context(), d, origin)
	var v content.ValidationError
	if errors.As(err, &v) {
		h.formError(w, r, p, d, errorList(i18n.FromRequest(r), v))
		return
	}
	if err != nil {
		h.fail(w, err)
		return
	}
	h.enqueue(r.Context(), p.ID, prev)
	if !admin.IsHTMX(r) {
		http.Redirect(w, r, "/admin/posts/"+p.ID, http.StatusSeeOther)
		return
	}
	h.saved(w, r, saved, origin)
}

// write saves d and returns the post as stored.
func (h *Admin) write(ctx context.Context, d Draft, origin string) (Post, error) {
	if _, _, err := h.repo.Save(ctx, d, origin); err != nil {
		return Post{}, err
	}
	return h.repo.Get(ctx, d.ID)
}

type savedFragment struct {
	Lang, SaveLabel, Title string
	Errors                 []string
	Revisions              []revRow
	RevCount               int
}

// saved answers an htmx save: the save label, plus out-of-band swaps for the
// title, the error list and the revisions list.
func (h *Admin) saved(w http.ResponseWriter, r *http.Request, p Post, origin string) {
	lang := i18n.FromRequest(r)
	revs, n, err := h.revisions(r.Context(), lang, p.ID)
	if err != nil {
		h.fail(w, err)
		return
	}
	h.ui.Fragment(w, http.StatusOK, h.editor, "saved", savedFragment{
		Lang: lang, SaveLabel: h.saveLabel(lang, p, origin), Title: title(lang, p), Revisions: revs, RevCount: n,
	})
}

// formError shows validation errors: a fragment for htmx (2xx so it swaps),
// the whole editor with the typed values for a plain post.
func (h *Admin) formError(w http.ResponseWriter, r *http.Request, p Post, d Draft, errs []string) {
	lang := i18n.FromRequest(r)
	if admin.IsHTMX(r) {
		revs, n, _ := h.revisions(r.Context(), lang, p.ID)
		h.ui.Fragment(w, http.StatusOK, h.editor, "saved", savedFragment{
			Lang: lang, SaveLabel: i18n.T(lang, "admin.editor.not_saved"), Title: title(lang, p), Errors: errs, Revisions: revs, RevCount: n,
		})
		return
	}
	shown := d.Post
	shown.Status = p.Status
	page, err := h.page(r, shown, d.Markdown, d.Tags, errs)
	if err != nil {
		h.fail(w, err)
		return
	}
	page.SaveLabel = i18n.T(lang, "admin.editor.not_saved")
	h.ui.Render(w, http.StatusUnprocessableEntity, h.editor, page)
}

// publish saves the form first, then publishes if the sign-in is recent.
// Otherwise the text is already stored and the browser goes through re-auth
// back to the editor, which offers to finish publishing.
func (h *Admin) publish(w http.ResponseWriter, r *http.Request) {
	p, ok := h.load(w, r)
	if !ok {
		return
	}
	prev, _ := jobs.RefOf(r.Context(), h.db, p.ID)
	recent := h.auth.Recent(r)
	// The "publish now" button shown after re-auth posts no fields: the text
	// was saved on the way out, so there is nothing to write.
	if r.FormValue("now") != "1" {
		origin := content.OriginManual
		if recent {
			origin = content.OriginPublish
		}
		d := formDraft(r, p)
		if _, err := h.write(r.Context(), d, origin); err != nil {
			var v content.ValidationError
			if errors.As(err, &v) {
				h.formError(w, r, p, d, errorList(i18n.FromRequest(r), v))
				return
			}
			h.fail(w, err)
			return
		}
	}
	editor := "/admin/posts/" + p.ID
	if !recent {
		h.enqueue(r.Context(), p.ID, prev)
		h.go2(w, r, auth.ReauthPath(editor+"?publish=1"))
		return
	}
	if _, err := h.repo.Publish(r.Context(), p.ID); err != nil {
		var v content.ValidationError
		if errors.As(err, &v) {
			cur, _ := h.repo.Get(r.Context(), p.ID)
			md, tags, _ := h.stored(r.Context(), cur)
			h.formError(w, r, cur, Draft{Post: cur, Markdown: md, Tags: tags}, errorList(i18n.FromRequest(r), v))
			return
		}
		h.fail(w, err)
		return
	}
	h.enqueue(r.Context(), p.ID, prev)
	h.go2(w, r, editor)
}

// stored returns the current revision's Markdown and the tags.
func (h *Admin) stored(ctx context.Context, p Post) (string, []string, error) {
	md := ""
	if p.CurrentRevisionID != nil {
		rev, err := content.GetRevision(ctx, h.db, *p.CurrentRevisionID)
		if err != nil {
			return "", nil, err
		}
		md = rev.BodyMD
	}
	tags, err := content.TagsOf(ctx, h.db, p.ID)
	if err != nil {
		return "", nil, err
	}
	out := make([]string, len(tags))
	for i, t := range tags {
		out[i] = t.Slug
	}
	return md, out, nil
}

// go2 navigates: HX-Redirect for htmx (a 200, so htmx acts on it), 303 otherwise.
func (h *Admin) go2(w http.ResponseWriter, r *http.Request, to string) {
	if admin.IsHTMX(r) {
		w.Header().Set("HX-Redirect", to)
		w.WriteHeader(http.StatusOK)
		return
	}
	http.Redirect(w, r, to, http.StatusSeeOther)
}

// translate opens the post's draft in the other language, creating it from
// this one (same slug, text and tags) when it does not exist yet.
func (h *Admin) translate(w http.ResponseWriter, r *http.Request) {
	p, ok := h.load(w, r)
	if !ok {
		return
	}
	to := r.FormValue("lang")
	if !i18n.IsSupported(to) || to == p.Lang {
		http.Error(w, "bad language", http.StatusBadRequest)
		return
	}
	res, err := h.db.Query(r.Context(), `SELECT id FROM entries WHERE translation_group_id = ? AND lang = ? AND kind = 'post'`, p.TranslationGroupID, to)
	if err != nil {
		h.fail(w, err)
		return
	}
	if rows, _ := store.Scan[struct {
		ID string `json:"id"`
	}](res); len(rows) == 1 {
		http.Redirect(w, r, "/admin/posts/"+rows[0].ID, http.StatusSeeOther)
		return
	}
	md, tags, err := h.stored(r.Context(), p)
	if err != nil {
		h.fail(w, err)
		return
	}
	n, err := h.repo.Create(r.Context(), Draft{
		Post: Post{Entry: content.Entry{Slug: p.Slug, Lang: to, TranslationGroupID: p.TranslationGroupID, Title: p.Title,
			Summary: p.Summary, SeriesID: p.SeriesID, SeriesOrder: p.SeriesOrder, Tone: p.Tone}},
		Markdown: md, Tags: tags,
	})
	if err != nil {
		h.fail(w, err)
		return
	}
	h.enqueue(r.Context(), n.ID, nil)
	http.Redirect(w, r, "/admin/posts/"+n.ID, http.StatusSeeOther)
}

// delete removes the post. With backlinks the slug must be typed (the
// button is disabled until it matches; the server checks too). A published
// post leaves a redirect, or a 410 tombstone, at its URL (CONTENT_MODEL.md).
func (h *Admin) delete(w http.ResponseWriter, r *http.Request) {
	p, ok := h.load(w, r)
	if !ok {
		return
	}
	lang := i18n.FromRequest(r)
	bl, err := content.Backlinks(r.Context(), h.db, p.ID, false)
	if err != nil {
		h.fail(w, err)
		return
	}
	to := strings.TrimSpace(r.FormValue("redirect_to"))
	if len(bl) > 0 && r.FormValue("confirm") != p.Slug {
		h.deleteError(w, r, p, i18n.T(lang, "admin.delete.mismatch"))
		return
	}
	if to != "" && (!strings.HasPrefix(to, "/") || strings.HasPrefix(to, "//") || strings.ContainsAny(to, " \\")) {
		h.deleteError(w, r, p, i18n.T(lang, "admin.redirect.bad_to"))
		return
	}
	prev, err := jobs.RefOf(r.Context(), h.db, p.ID)
	if err != nil {
		h.fail(w, err)
		return
	}
	var stmts []store.Stmt
	if p.Status == content.Published {
		from := "/" + p.Lang + "/posts/" + p.Slug
		code := http.StatusMovedPermanently
		if to == "" || to == from {
			code, to = http.StatusGone, ""
		}
		stmts = append(stmts, store.Stmt{SQL: `INSERT INTO redirects (from_path, to_path, code, created_at) VALUES (?, ?, ?, ?)
ON CONFLICT (from_path) DO UPDATE SET to_path = excluded.to_path, code = excluded.code`,
			Args: []any{from, to, code, content.Clock().Unix()}})
	}
	stmts = append(stmts, store.Stmt{SQL: `DELETE FROM entries WHERE id = ? AND kind = 'post'`, Args: []any{p.ID}})
	if _, err := h.db.Batch(r.Context(), stmts...); err != nil {
		h.fail(w, err)
		return
	}
	h.enqueue(r.Context(), p.ID, prev)
	http.Redirect(w, r, "/admin?deleted="+url.QueryEscape(p.Slug), http.StatusSeeOther)
}

func (h *Admin) deleteError(w http.ResponseWriter, r *http.Request, p Post, msg string) {
	md, tags, err := h.stored(r.Context(), p)
	if err != nil {
		h.fail(w, err)
		return
	}
	page, err := h.page(r, p, md, tags, nil)
	if err != nil {
		h.fail(w, err)
		return
	}
	page.OpenDelete, page.DeleteErr = true, msg
	h.ui.Render(w, http.StatusUnprocessableEntity, h.editor, page)
}

// preview renders Markdown for the live preview pane. Nothing is stored.
func (h *Admin) preview(w http.ResponseWriter, r *http.Request) {
	lang := r.FormValue("lang")
	if !i18n.IsSupported(lang) {
		lang = i18n.Default
	}
	out, err := render.Markdown([]byte(r.FormValue("markdown")), render.Options{Lang: lang})
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if t := strings.TrimSpace(r.FormValue("title")); t != "" {
		_, _ = w.Write([]byte(`<h1 class="ed-prev-title">` + template.HTMLEscapeString(t) + `</h1>`))
	}
	_, _ = w.Write([]byte(out.HTML))
}

// enqueue starts the render job for an entry write. A failed enqueue is
// logged, not shown: the write itself succeeded and the dashboard lists
// failed jobs to retry.
func (h *Admin) enqueue(ctx context.Context, id string, prev *jobs.Ref) {
	if err := jobs.Enqueue(ctx, h.jobs, id, prev); err != nil {
		slog.Error("enqueue entry.changed", "entry", id, "err", err)
	}
}

func (h *Admin) fail(w http.ResponseWriter, err error) {
	slog.Error("post admin", "err", err)
	http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
}

func title(lang string, p Post) string {
	if strings.TrimSpace(p.Title) != "" {
		return p.Title
	}
	return i18n.T(lang, "admin.untitled") + " · " + p.Slug
}

// errorList turns validation errors into catalog messages, field by field.
func errorList(lang string, v content.ValidationError) []string {
	var out []string
	for _, f := range []string{"title", "slug", "lang", "summary", "body", "published_at", "series", "canonical_url"} {
		msg, ok := v[f]
		if !ok {
			continue
		}
		key := "admin.err." + f
		if f == "slug" && strings.Contains(msg, "already used") {
			key = "admin.err.slug_taken"
		}
		if t := i18n.T(lang, key); t != key {
			msg = t
		}
		out = append(out, msg)
		delete(v, f)
	}
	for f, msg := range v {
		out = append(out, f+": "+msg)
	}
	return out
}
