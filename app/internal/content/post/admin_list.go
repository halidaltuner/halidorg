package post

import (
	"errors"
	"net/http"
	"net/url"
	"strings"

	"halid.org/engine/internal/admin"
	"halid.org/engine/internal/content"
	"halid.org/engine/internal/i18n"
	"halid.org/engine/internal/jobs"
	"halid.org/engine/internal/store"
)

// Entry list (derive: dashboard density, DESIGN.md §6 and §7). Filters are
// GET parameters, so the list works and links without JS; bulk actions post
// the checked rows.

type listRow struct {
	admin.EntryRow
	ID, Lang, Status, StatusKey, Updated, Translations string
}

type listFilter struct {
	Status, Lang, Tag string
}

type listPage struct {
	admin.Shell
	Rows     []listRow
	Filter   listFilter
	Statuses []string
	Tags     []string
	Notice   string
	Error    string
}

var listStatuses = []string{content.Draft, content.Scheduled, content.Published, content.Archived}

func (h *Admin) listPage(w http.ResponseWriter, r *http.Request) {
	h.renderList(w, r, http.StatusOK, "")
}

func (h *Admin) renderList(w http.ResponseWriter, r *http.Request, status int, errMsg string) {
	lang := i18n.FromRequest(r)
	q := r.URL.Query()
	f := listFilter{Status: q.Get("status"), Lang: q.Get("lang"), Tag: content.NormalizeTag(q.Get("tag"))}
	if !contains(listStatuses, f.Status) {
		f.Status = ""
	}
	if !i18n.IsSupported(f.Lang) {
		f.Lang = ""
	}
	p := listPage{Shell: h.ui.Shell(r, i18n.T(lang, "admin.posts.title"), "posts"), Filter: f, Statuses: listStatuses, Error: errMsg}
	if d := q.Get("done"); d == "publish" || d == "archive" {
		p.Notice = i18n.T(lang, "admin.bulk."+d+"ed")
	}
	posts, err := h.repo.List(r.Context(), ListFilter{Status: f.Status, Lang: f.Lang, Tag: f.Tag, Limit: 100, Admin: true})
	if err != nil {
		h.fail(w, err)
		return
	}
	trs, err := h.translationMarks(r, posts)
	if err != nil {
		h.fail(w, err)
		return
	}
	now := content.Clock().Unix()
	for _, x := range posts {
		p.Rows = append(p.Rows, listRow{
			EntryRow: admin.EntryRow{Kind: Kind, Title: title(lang, x), Href: "/admin/posts/" + x.ID},
			ID:       x.ID, Lang: x.Lang, StatusKey: x.Status, Status: i18n.T(lang, "admin.status."+x.Status),
			Updated: admin.Ago(lang, x.UpdatedAt, now), Translations: trs[x.ID],
		})
	}
	if p.Tags, err = h.tagSlugs(r); err != nil {
		h.fail(w, err)
		return
	}
	h.ui.Render(w, status, h.list, p)
}

// translationMarks returns "tr ✓" style marks for each post's other
// languages, from one query over the translation groups on the page.
func (h *Admin) translationMarks(r *http.Request, posts []Post) (map[string]string, error) {
	out := map[string]string{}
	if len(posts) == 0 {
		return out, nil
	}
	args := make([]any, len(posts))
	for i, p := range posts {
		args[i] = p.TranslationGroupID
	}
	res, err := h.db.Query(r.Context(), `SELECT translation_group_id, lang, status FROM entries
WHERE kind = 'post' AND translation_group_id IN (?`+strings.Repeat(", ?", len(posts)-1)+`)`, args...)
	if err != nil {
		return nil, err
	}
	rows, err := store.Scan[struct {
		Group  string `json:"translation_group_id"`
		Lang   string `json:"lang"`
		Status string `json:"status"`
	}](res)
	if err != nil {
		return nil, err
	}
	have := map[string]map[string]string{}
	for _, x := range rows {
		if have[x.Group] == nil {
			have[x.Group] = map[string]string{}
		}
		have[x.Group][x.Lang] = x.Status
	}
	for _, p := range posts {
		var marks []string
		for _, l := range i18n.Supported {
			if l == p.Lang {
				continue
			}
			switch have[p.TranslationGroupID][l] {
			case content.Published:
				marks = append(marks, l+" ✓")
			case "":
				marks = append(marks, l+" ✕")
			default:
				marks = append(marks, l+" …") // a draft exists
			}
		}
		out[p.ID] = strings.Join(marks, " · ")
	}
	return out, nil
}

func (h *Admin) tagSlugs(r *http.Request) ([]string, error) {
	res, err := h.db.Query(r.Context(), `SELECT DISTINCT t.slug FROM tags t JOIN entry_tags et ON et.tag_id = t.id
JOIN entries e ON e.id = et.entry_id WHERE e.kind = 'post' ORDER BY t.slug`)
	if err != nil {
		return nil, err
	}
	rows, err := store.Scan[struct {
		Slug string `json:"slug"`
	}](res)
	if err != nil {
		return nil, err
	}
	out := make([]string, len(rows))
	for i, x := range rows {
		out[i] = x.Slug
	}
	return out, nil
}

// bulk publishes or archives the checked posts. It sits behind
// RequireRecentAuth like publish and delete. Each post gets its render job;
// posts that cannot be published (no title, nothing written) are reported.
func (h *Admin) bulk(w http.ResponseWriter, r *http.Request) {
	lang := i18n.FromRequest(r)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	action := r.PostForm.Get("action")
	ids := r.PostForm["id"]
	back := "/admin/posts"
	if q := r.PostForm.Get("back"); strings.HasPrefix(q, "?") {
		back += q
	}
	if action != "publish" && action != "archive" || len(ids) == 0 {
		h.listAfterPost(w, r, back, i18n.T(lang, "admin.bulk.none"))
		return
	}
	done, failed := 0, []string{}
	for _, id := range ids {
		prev, err := jobs.RefOf(r.Context(), h.db, id)
		if err != nil {
			h.fail(w, err)
			return
		}
		if prev == nil || prev.Kind != Kind {
			continue
		}
		switch action {
		case "publish":
			_, err = h.repo.Publish(r.Context(), id)
		case "archive":
			err = h.repo.Archive(r.Context(), id)
		}
		var v content.ValidationError
		if errors.As(err, &v) {
			failed = append(failed, prev.Slug)
			continue
		}
		if err != nil {
			h.fail(w, err)
			return
		}
		h.enqueue(r.Context(), id, prev)
		done++
	}
	if len(failed) > 0 {
		h.listAfterPost(w, r, back, i18n.T(lang, "admin.bulk.failed")+" "+strings.Join(failed, ", "))
		return
	}
	sep := "?"
	if strings.Contains(back, "?") {
		sep = "&"
	}
	http.Redirect(w, r, back+sep+"done="+action, http.StatusSeeOther)
}

// listAfterPost re-renders the list (with its filters) and an error.
func (h *Admin) listAfterPost(w http.ResponseWriter, r *http.Request, back, msg string) {
	u, _ := url.Parse(back)
	r2 := r.Clone(r.Context())
	r2.URL = u
	r2.Method = http.MethodGet
	h.renderList(w, r2, http.StatusUnprocessableEntity, msg)
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}
