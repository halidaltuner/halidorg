package post

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"halid.org/engine/internal/admin"
	"halid.org/engine/internal/content"
	"halid.org/engine/internal/diff"
	"halid.org/engine/internal/i18n"
	"halid.org/engine/internal/jobs"
	"halid.org/engine/internal/store"
)

// Revisions (derive): a timeline on the left, a side-by-side diff of two
// revisions on the right, and restore, which saves the old text as a new
// revision so nothing is ever overwritten (ADR-006).

type timelineRow struct {
	ID, At, Origin, Note string
	Chars                int
	Current, Published   bool
	A, B                 bool // chosen as the old (A) or new (B) side
	Href                 string
}

type diffRow struct {
	diff.Row
	Skip int // > 0: a folded run of unchanged lines
}

type revisionsPage struct {
	admin.Shell
	Post           Post
	Timeline       []timelineRow
	A, B           *timelineRow
	Rows           []diffRow
	Added, Removed int
	Identical      bool
}

type revRec struct {
	ID        string `json:"id"`
	Origin    string `json:"origin"`
	CreatedAt int64  `json:"created_at"`
	Note      string `json:"note"`
	Chars     int    `json:"chars"`
}

func (h *Admin) revisionsPage(w http.ResponseWriter, r *http.Request) {
	p, ok := h.load(w, r)
	if !ok {
		return
	}
	lang := i18n.FromRequest(r)
	res, err := h.db.Query(r.Context(), `SELECT id, origin, created_at, note, length(body_md) AS chars
FROM revisions WHERE entry_id = ? ORDER BY id DESC LIMIT 200`, p.ID)
	if err != nil {
		h.fail(w, err)
		return
	}
	revs, err := store.Scan[revRec](res)
	if err != nil {
		h.fail(w, err)
		return
	}
	pg := revisionsPage{Shell: h.ui.Shell(r, i18n.T(lang, "admin.rev.title")+" · "+title(lang, p), nav(p.Kind)), Post: p}
	if len(revs) == 0 {
		h.ui.Render(w, http.StatusOK, h.revs, pg)
		return
	}
	// Default: the newest revision against the one before it.
	q := r.URL.Query()
	bID, aID := q.Get("b"), q.Get("a")
	idx := map[string]int{}
	for i, rv := range revs {
		idx[rv.ID] = i
	}
	if _, ok := idx[bID]; !ok {
		bID = revs[0].ID
	}
	if _, ok := idx[aID]; !ok || aID == bID {
		aID = ""
		if i := idx[bID] + 1; i < len(revs) {
			aID = revs[i].ID
		}
	}
	for _, rv := range revs {
		row := timelineRow{
			ID: rv.ID, At: h.ui.Time(rv.CreatedAt, "01-02 15:04"), Origin: i18n.T(lang, "admin.rev."+rv.Origin),
			Note: rv.Note, Chars: rv.Chars, A: rv.ID == aID, B: rv.ID == bID,
			Current:   p.CurrentRevisionID != nil && *p.CurrentRevisionID == rv.ID,
			Published: p.PublishedRevisionID != nil && *p.PublishedRevisionID == rv.ID,
			Href:      "/admin/posts/" + p.ID + "/revisions?b=" + url.QueryEscape(rv.ID),
		}
		pg.Timeline = append(pg.Timeline, row)
	}
	for i := range pg.Timeline {
		if pg.Timeline[i].A {
			pg.A = &pg.Timeline[i]
		}
		if pg.Timeline[i].B {
			pg.B = &pg.Timeline[i]
		}
	}
	newRev, err := content.GetRevision(r.Context(), h.db, bID)
	if err != nil {
		h.fail(w, err)
		return
	}
	oldMD := ""
	if aID != "" {
		oldRev, err := content.GetRevision(r.Context(), h.db, aID)
		if err != nil {
			h.fail(w, err)
			return
		}
		oldMD = oldRev.BodyMD
	}
	rows := diff.SideBySide(oldMD, newRev.BodyMD)
	pg.Added, pg.Removed = diff.Stats(rows)
	pg.Identical = pg.Added == 0 && pg.Removed == 0
	pg.Rows = fold(rows, 3)
	h.ui.Render(w, http.StatusOK, h.revs, pg)
}

// fold replaces long runs of unchanged lines with one marker row, keeping
// ctx lines of context around each change.
func fold(rows []diff.Row, ctx int) []diffRow {
	var out []diffRow
	for i := 0; i < len(rows); {
		if rows[i].Kind != "eq" {
			out = append(out, diffRow{Row: rows[i]})
			i++
			continue
		}
		j := i
		for j < len(rows) && rows[j].Kind == "eq" {
			j++
		}
		head, tail := ctx, ctx
		if i == 0 {
			head = 0
		}
		if j == len(rows) {
			tail = 0
		}
		if j-i <= head+tail+1 {
			for _, r := range rows[i:j] {
				out = append(out, diffRow{Row: r})
			}
		} else {
			for _, r := range rows[i : i+head] {
				out = append(out, diffRow{Row: r})
			}
			out = append(out, diffRow{Skip: j - i - head - tail})
			for _, r := range rows[j-tail : j] {
				out = append(out, diffRow{Row: r})
			}
		}
		i = j
	}
	return out
}

// restore saves an old revision's text as a new manual revision with a
// note. Fields other than the body (title, slug, tags) stay as they are now.
func (h *Admin) restore(w http.ResponseWriter, r *http.Request) {
	p, ok := h.load(w, r)
	if !ok {
		return
	}
	rev, err := content.GetRevision(r.Context(), h.db, r.PathValue("rev"))
	if errors.Is(err, content.ErrNotFound) || err == nil && rev.EntryID != p.ID {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		h.fail(w, err)
		return
	}
	_, tags, err := h.stored(r.Context(), p)
	if err != nil {
		h.fail(w, err)
		return
	}
	prev, _ := jobs.RefOf(r.Context(), h.db, p.ID)
	saved, err := h.write(r.Context(), Draft{Post: p, Markdown: rev.BodyMD, Tags: tags}, content.OriginManual)
	if err != nil {
		h.fail(w, err)
		return
	}
	note := fmt.Sprintf(i18n.T(i18n.FromRequest(r), "admin.rev.restored_from"), h.ui.Time(rev.CreatedAt, "2006-01-02 15:04"))
	if saved.CurrentRevisionID != nil {
		if _, err := h.db.Query(r.Context(), `UPDATE revisions SET note = ? WHERE id = ?`, note, *saved.CurrentRevisionID); err != nil {
			h.fail(w, err)
			return
		}
	}
	h.enqueue(r.Context(), p.ID, prev)
	http.Redirect(w, r, "/admin/posts/"+p.ID, http.StatusSeeOther)
}
