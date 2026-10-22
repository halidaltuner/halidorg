package admin

import (
	"context"
	"errors"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"halid.org/engine/internal/content"
	"halid.org/engine/internal/i18n"
	"halid.org/engine/internal/jobs"
	"halid.org/engine/internal/search"
	"halid.org/engine/internal/store"
	"halid.org/engine/internal/webmention"
)

// EntryRow is one entry in a dashboard panel or search result.
type EntryRow struct {
	Kind, Title, Href, Meta string
}

// editors are the kinds with an admin editor, by their URL segment.
var editors = map[string]string{"post": "posts", "note": "notes", "snippet": "snippets", "link": "links", "fav": "favs",
	"project": "projects", "buildlog": "buildlogs", "presentation": "presentations",
	"term": "glossary", "book": "reading"}

// EditorHref is the admin editor of an entry, or "" for kinds without one yet.
func EditorHref(kind, id string) string {
	if seg, ok := editors[kind]; ok {
		return "/admin/" + seg + "/" + id
	}
	return ""
}

type jobRow struct {
	Lang, ID, Name, Detail string
	Queued                 bool
}

type notFoundRow struct {
	Path string `json:"path"`
	Hits int    `json:"hits"`
}

type redirectForm struct {
	From, To, Error, Saved string
	Gone                   bool
}

type notFoundPanel struct {
	Lang     string
	NotFound []notFoundRow
	Redirect redirectForm
}

type dashPage struct {
	Shell
	Drafts, Scheduled, Recent []EntryRow
	Week                      int
	Failed                    []jobRow
	NotFound                  []notFoundRow
	Redirect                  redirectForm
	Deleted                   string // slug of an entry just deleted from its editor
	Bookmarklet               template.URL
	Mentions                  []mentionRow
	MentionCount              int
}

// mentionRow is a webmention waiting in the moderation panel.
type mentionRow struct {
	Lang, ID, Source, Host, Kind, Target, Text string
}

type entryRec struct {
	ID          string  `json:"id"`
	Kind        string  `json:"kind"`
	Lang        string  `json:"lang"`
	Title       string  `json:"title"`
	Slug        string  `json:"slug"`
	UpdatedAt   int64   `json:"updated_at"`
	PublishedAt *int64  `json:"published_at"`
	Langs       *string `json:"langs"`
}

func (r entryRec) title(lang string) string {
	if strings.TrimSpace(r.Title) != "" {
		return r.Title
	}
	return i18n.T(lang, "admin.untitled") + " · " + r.Slug
}

func (h *Handlers) entries(ctx context.Context, where, order string, limit int, args ...any) ([]entryRec, error) {
	res, err := h.db.Query(ctx, `SELECT e.id, e.kind, e.lang, e.title, e.slug, e.updated_at, e.published_at,
  (SELECT group_concat(t.lang) FROM entries t WHERE t.translation_group_id = e.translation_group_id AND t.id <> e.id AND t.status = 'published') AS langs
FROM entries e WHERE `+where+` ORDER BY `+order+` LIMIT ?`, append(args, limit)...)
	if err != nil {
		return nil, err
	}
	return store.Scan[entryRec](res)
}

// translationMeta is "en · tr ✓": the entry's language, then each other
// language with whether a published translation exists.
func translationMeta(r entryRec) string {
	have := map[string]bool{}
	if r.Langs != nil {
		for _, l := range strings.Split(*r.Langs, ",") {
			have[l] = true
		}
	}
	parts := []string{r.Lang}
	for _, l := range i18n.Supported {
		if l == r.Lang {
			continue
		}
		mark := "✕"
		if have[l] {
			mark = "✓"
		}
		parts = append(parts, l+" "+mark)
	}
	return strings.Join(parts, " · ")
}

func (h *Handlers) dashboard(w http.ResponseWriter, r *http.Request) {
	h.renderDash(w, r, redirectForm{}, http.StatusOK)
}

// renderDash renders the dashboard; f carries the redirect form back after
// a plain (no-JS) post that failed validation.
func (h *Handlers) renderDash(w http.ResponseWriter, r *http.Request, f redirectForm, status int) {
	ctx := r.Context()
	p := dashPage{Shell: h.ui.Shell(r, i18n.T(i18n.FromRequest(r), "admin.dash.title"), "dashboard"), Redirect: f, Bookmarklet: Bookmarklet()}
	if d := r.URL.Query().Get("deleted"); content.ValidSlug(d) {
		p.Deleted = d
	}
	lang := p.Lang
	now := content.Clock().Unix()

	drafts, err := h.entries(ctx, `e.status = 'draft'`, `e.updated_at DESC`, 8)
	if err != nil {
		h.fail(w, err)
		return
	}
	for _, d := range drafts {
		p.Drafts = append(p.Drafts, EntryRow{Kind: d.Kind, Title: d.title(lang), Href: EditorHref(d.Kind, d.ID),
			Meta: fmt.Sprintf(i18n.T(lang, "admin.dash.edited"), Ago(lang, d.UpdatedAt, now))})
	}
	sched, err := h.entries(ctx, `e.status = 'scheduled'`, `e.published_at ASC`, 8)
	if err != nil {
		h.fail(w, err)
		return
	}
	for _, s := range sched {
		meta := ""
		if s.PublishedAt != nil {
			meta = h.ui.Time(*s.PublishedAt, "01-02 15:04")
		}
		p.Scheduled = append(p.Scheduled, EntryRow{Kind: s.Kind, Title: s.title(lang), Href: EditorHref(s.Kind, s.ID), Meta: meta})
	}
	recent, err := h.entries(ctx, `e.status = 'published'`, `e.published_at DESC`, 6)
	if err != nil {
		h.fail(w, err)
		return
	}
	for _, s := range recent {
		p.Recent = append(p.Recent, EntryRow{Kind: s.Kind, Title: s.title(lang), Href: EditorHref(s.Kind, s.ID), Meta: translationMeta(s)})
	}
	res, err := h.db.Query(ctx, `SELECT COUNT(*) AS n FROM entries WHERE status = 'published' AND published_at >= ?`, now-7*86400)
	if err != nil {
		h.fail(w, err)
		return
	}
	if rows, _ := store.Scan[struct {
		N int `json:"n"`
	}](res); len(rows) == 1 {
		p.Week = rows[0].N
	}

	failed, err := h.jobs.Failed(ctx, 8)
	if err != nil {
		h.fail(w, err)
		return
	}
	for _, f := range failed {
		p.Failed = append(p.Failed, jobRow{Lang: lang, ID: f.ID, Name: f.Type + " " + shortID(f.ID), Detail: jobDetail(lang, f)})
	}
	if p.NotFound, err = h.notFound(ctx); err != nil {
		h.fail(w, err)
		return
	}
	queued, n, err := h.mentions.Pending(ctx, 8)
	if err != nil {
		h.fail(w, err)
		return
	}
	p.MentionCount = n
	for _, m := range queued {
		text := m.Content
		if m.Author != "" {
			text = m.Author + ": " + text
		}
		p.Mentions = append(p.Mentions, mentionRow{Lang: lang, ID: m.ID, Source: m.SourceURL, Host: webmention.Host(m.SourceURL),
			Kind: i18n.T(lang, "admin.wm."+m.Type), Target: m.Title, Text: strings.TrimSuffix(text, ": ")})
	}
	h.ui.Render(w, status, h.dash, p)
}

func shortID(id string) string {
	if len(id) > 6 {
		return "#" + strings.ToLower(id[len(id)-6:])
	}
	return "#" + id
}

func jobDetail(lang string, f jobs.RunRow) string {
	msg := f.Error
	if r := []rune(msg); len(r) > 90 {
		msg = string(r[:90]) + "…"
	}
	return fmt.Sprintf(i18n.T(lang, "admin.jobs.detail"), f.Status, f.Attempt, f.MaxAttempts) + " · " + msg
}

func (h *Handlers) notFound(ctx context.Context) ([]notFoundRow, error) {
	res, err := h.db.Query(ctx, `SELECT path, hits FROM not_found_log WHERE last_seen_at >= ?
  AND path NOT IN (SELECT from_path FROM redirects) ORDER BY hits DESC, path LIMIT 10`, content.Clock().Unix()-7*86400)
	if err != nil {
		return nil, err
	}
	return store.Scan[notFoundRow](res)
}

// retryJob re-queues a failed job. htmx gets the row back in its queued
// state; a plain form post returns to the dashboard.
func (h *Handlers) retryJob(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	err := h.jobs.Retry(r.Context(), id)
	if errors.Is(err, content.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		h.fail(w, err)
		return
	}
	if !IsHTMX(r) {
		http.Redirect(w, r, "/admin", http.StatusSeeOther)
		return
	}
	lang := i18n.FromRequest(r)
	row := jobRow{Lang: lang, ID: id, Name: shortID(id), Queued: true}
	res, err := h.db.Query(r.Context(), `SELECT type FROM job_runs WHERE id = ?`, id)
	if err == nil {
		if rows, _ := store.Scan[struct {
			Type string `json:"type"`
		}](res); len(rows) == 1 {
			row.Name = rows[0].Type + " " + shortID(id)
		}
	}
	h.ui.Fragment(w, http.StatusOK, h.dash, "job-row", row)
}

// moderate approves or rejects a webmention. Either way it leaves the
// panel: htmx gets an empty row and the new count; a plain post returns to
// the dashboard. Approving re-renders the pages that show it.
func (h *Handlers) moderate(status string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		paths, err := h.mentions.Moderate(ctx, r.PathValue("id"), status)
		if errors.Is(err, content.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			h.fail(w, err)
			return
		}
		if len(paths) > 0 {
			if err := jobs.EnqueuePages(ctx, h.jobs, paths...); err != nil {
				slog.Error("webmention: enqueue pages", "err", err) // the job row stays queued for a retry
			}
		}
		if !IsHTMX(r) {
			http.Redirect(w, r, "/admin", http.StatusSeeOther)
			return
		}
		_, n, err := h.mentions.Pending(ctx, 0)
		if err != nil {
			h.fail(w, err)
			return
		}
		h.ui.Fragment(w, http.StatusOK, h.dash, "wm-count", n)
	}
}

// addRedirect stores a redirect (or a 410 tombstone) for a path that 404s.
func (h *Handlers) addRedirect(w http.ResponseWriter, r *http.Request) {
	lang := i18n.FromRequest(r)
	f := redirectForm{
		From: strings.TrimSpace(r.FormValue("from")),
		To:   strings.TrimSpace(r.FormValue("to")),
		Gone: r.FormValue("gone") == "1",
	}
	if msg := validateRedirect(lang, &f); msg != "" {
		f.Error = msg
		h.redirectResult(w, r, f, http.StatusUnprocessableEntity)
		return
	}
	code := 301
	if f.Gone {
		code, f.To = 410, ""
	}
	_, err := h.db.Batch(r.Context(),
		store.Stmt{SQL: `INSERT INTO redirects (from_path, to_path, code, created_at) VALUES (?, ?, ?, ?)
ON CONFLICT (from_path) DO UPDATE SET to_path = excluded.to_path, code = excluded.code`,
			Args: []any{f.From, f.To, code, content.Clock().Unix()}},
		store.Stmt{SQL: `DELETE FROM not_found_log WHERE path = ?`, Args: []any{f.From}},
	)
	if err != nil {
		h.fail(w, err)
		return
	}
	saved := f.From + " → " + f.To
	if f.Gone {
		saved = f.From + " → 410"
	}
	h.redirectResult(w, r, redirectForm{Saved: saved}, http.StatusOK)
}

// validateRedirect normalizes f and returns a user-facing error, or "".
func validateRedirect(lang string, f *redirectForm) string {
	if u, err := url.Parse(f.From); err == nil && u.Host != "" {
		f.From = u.RequestURI() // pasted a full URL from the 404 log or a browser
	}
	if !strings.HasPrefix(f.From, "/") || strings.HasPrefix(f.From, "//") || strings.ContainsAny(f.From, " \\") {
		return i18n.T(lang, "admin.redirect.bad_from")
	}
	if f.From == "/" || f.From == "/admin" || strings.HasPrefix(f.From, "/admin/") || strings.HasPrefix(f.From, "/static/") || strings.HasPrefix(f.From, "/og/") {
		return i18n.T(lang, "admin.redirect.reserved")
	}
	if f.Gone {
		return ""
	}
	if !strings.HasPrefix(f.To, "/") || strings.HasPrefix(f.To, "//") || strings.ContainsAny(f.To, " \\") {
		return i18n.T(lang, "admin.redirect.bad_to")
	}
	if f.To == f.From {
		return i18n.T(lang, "admin.redirect.loop")
	}
	return ""
}

func (h *Handlers) redirectResult(w http.ResponseWriter, r *http.Request, f redirectForm, status int) {
	if !IsHTMX(r) {
		if f.Error != "" {
			// The plain form re-renders the dashboard with the error open.
			h.renderDash(w, r, f, status)
			return
		}
		http.Redirect(w, r, "/admin", http.StatusSeeOther)
		return
	}
	rows, err := h.notFound(r.Context())
	if err != nil {
		h.fail(w, err)
		return
	}
	if status != http.StatusOK {
		// htmx swaps 2xx only by default; keep the form and show the error.
		status = http.StatusOK
	}
	h.ui.Fragment(w, status, h.dash, "not-found", notFoundPanel{Lang: i18n.FromRequest(r), NotFound: rows, Redirect: f})
}

type searchPage struct {
	Shell
	Hits []EntryRow
}

// search queries the SEARCH database with drafts included (ADR-010).
func (h *Handlers) search(w http.ResponseWriter, r *http.Request) {
	lang := i18n.FromRequest(r)
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	var hits []EntryRow
	if st, ok := search.Parse(q).Stmt(search.Filter{Drafts: true, Limit: 12}); ok {
		res, err := h.searchDB.Query(r.Context(), st.SQL, st.Args...)
		if err != nil {
			h.fail(w, err)
			return
		}
		found, err := store.Scan[search.Hit](res)
		if err != nil {
			h.fail(w, err)
			return
		}
		if hits, err = h.hitRows(r.Context(), lang, found); err != nil {
			h.fail(w, err)
			return
		}
	}
	p := searchPage{Hits: hits}
	if IsHTMX(r) {
		p.Lang, p.Query = lang, q
		h.ui.Fragment(w, http.StatusOK, h.searchT, "search-results", p)
		return
	}
	p.Shell = h.ui.Shell(r, i18n.T(lang, "admin.search.title"), "")
	p.Query = q
	h.ui.Render(w, http.StatusOK, h.searchT, p)
}

// hitRows looks up titles in the main database, in rank order. Hits for
// entries deleted since indexing are dropped.
func (h *Handlers) hitRows(ctx context.Context, lang string, hits []search.Hit) ([]EntryRow, error) {
	if len(hits) == 0 {
		return nil, nil
	}
	args := make([]any, len(hits))
	for i, hit := range hits {
		args[i] = hit.EntryID
	}
	res, err := h.db.Query(ctx, `SELECT e.id, e.kind, e.lang, e.title, e.slug, e.updated_at, e.published_at, NULL AS langs
FROM entries e WHERE e.id IN (?`+strings.Repeat(", ?", len(hits)-1)+`)`, args...)
	if err != nil {
		return nil, err
	}
	recs, err := store.Scan[entryRec](res)
	if err != nil {
		return nil, err
	}
	byID := map[string]entryRec{}
	for _, e := range recs {
		byID[e.ID] = e
	}
	var out []EntryRow
	for _, hit := range hits {
		e, ok := byID[hit.EntryID]
		if !ok {
			continue
		}
		out = append(out, EntryRow{Kind: e.Kind, Title: e.title(lang), Href: EditorHref(e.Kind, e.ID),
			Meta: e.Lang + " · " + i18n.T(lang, "admin.status."+hit.Status)})
	}
	return out, nil
}

func (h *Handlers) fail(w http.ResponseWriter, err error) {
	slog.Error("admin", "err", err)
	http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
}
