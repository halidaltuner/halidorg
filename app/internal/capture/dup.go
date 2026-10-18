package capture

import (
	"context"
	"strings"

	"halid.org/engine/internal/i18n"
	"halid.org/engine/internal/store"
)

// duplicate is an entry that already holds the pasted address.
type duplicate struct {
	ID, Kind, KindLabel, Title, Status string
}

// dupKey is the form of an address compared for duplicates: no fragment,
// no trailing slash, lower case. Lowering the path too can match two pages
// that differ only in case; for a warning that is the cheaper mistake.
func dupKey(u string) string {
	u = strings.TrimSpace(u)
	if i := strings.IndexByte(u, '#'); i >= 0 {
		u = u[:i]
	}
	return strings.ToLower(strings.TrimRight(u, "/"))
}

// keySQL is dupKey in SQL, for column c. Stored addresses keep how they were
// typed, so both sides are folded.
func keySQL(c string) string {
	return "lower(rtrim(CASE WHEN instr(" + c + ", '#') > 0 THEN substr(" + c + ", 1, instr(" + c + ", '#') - 1) ELSE " + c + " END, '/'))"
}

// findDuplicate returns the first link, fav or note (as its source) that
// already has the address, newest first, or nil.
func (h *Handlers) findDuplicate(ctx context.Context, lang, u string) (*duplicate, error) {
	k := dupKey(u)
	if k == "" {
		return nil, nil
	}
	res, err := h.db.Query(ctx, `SELECT e.id, e.kind, e.title, e.summary, e.status FROM entries e
LEFT JOIN links l ON l.entry_id = e.id LEFT JOIN favs f ON f.entry_id = e.id LEFT JOIN notes n ON n.entry_id = e.id
WHERE `+keySQL("l.url")+` = ? OR `+keySQL("f.url")+` = ? OR `+keySQL("n.source_url")+` = ?
ORDER BY e.created_at DESC LIMIT 1`, k, k, k)
	if err != nil {
		return nil, err
	}
	rows, err := store.Scan[struct {
		ID      string `json:"id"`
		Kind    string `json:"kind"`
		Title   string `json:"title"`
		Summary string `json:"summary"`
		Status  string `json:"status"`
	}](res)
	if err != nil || len(rows) == 0 {
		return nil, err
	}
	d := rows[0]
	title := d.Title
	if title == "" {
		title = d.Summary
	}
	return &duplicate{ID: d.ID, Kind: d.Kind, KindLabel: i18n.T(lang, "palette.kind."+d.Kind), Title: title,
		Status: i18n.T(lang, "admin.status."+d.Status)}, nil
}
