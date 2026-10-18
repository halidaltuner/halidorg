package content

import (
	"context"
	"strings"

	"halid.org/engine/internal/store"
)

// TagRow is a tag with how many entries use it (the tags admin).
type TagRow struct {
	Tag
	Entries int `json:"entries"`
}

// AllTags lists every tag by slug with its entry count.
func AllTags(ctx context.Context, db store.DB) ([]TagRow, error) {
	res, err := db.Query(ctx, `SELECT t.id, t.slug, t.name_tr, t.name_en, t.tone,
  (SELECT COUNT(*) FROM entry_tags et WHERE et.tag_id = t.id) AS entries
FROM tags t ORDER BY t.slug`)
	if err != nil {
		return nil, err
	}
	return store.Scan[TagRow](res)
}

// UpdateTag renames a tag and sets its tone. The slug stays: it is in URLs.
func UpdateTag(ctx context.Context, db store.DB, t Tag) error {
	v := ValidationError{}
	if strings.TrimSpace(t.NameTR) == "" && strings.TrimSpace(t.NameEN) == "" {
		v["name"] = "a TR or EN name is required"
	}
	if t.Tone != nil && !tones[*t.Tone] {
		v["tone"] = "unknown tone"
	}
	if len(v) > 0 {
		return v
	}
	res, err := db.Query(ctx, `UPDATE tags SET name_tr = ?, name_en = ?, tone = ? WHERE id = ?`,
		strings.TrimSpace(t.NameTR), strings.TrimSpace(t.NameEN), t.Tone, t.ID)
	if err != nil {
		return err
	}
	if res.Changes == 0 {
		return NotFound("tag", t.ID)
	}
	return nil
}

// TagEntries returns the ids of the entries with a tag.
func TagEntries(ctx context.Context, db store.DB, tagID string) ([]string, error) {
	res, err := db.Query(ctx, `SELECT entry_id AS id FROM entry_tags WHERE tag_id = ?`, tagID)
	if err != nil {
		return nil, err
	}
	return ids(res)
}

// MergeTags moves every entry of from onto into and deletes from, in one
// batch. Entries that had both keep one.
func MergeTags(ctx context.Context, db store.DB, from, into string) error {
	if from == into || from == "" || into == "" {
		return ValidationError{"merge": "pick two different tags"}
	}
	res, err := db.Query(ctx, `SELECT COUNT(*) AS n FROM tags WHERE id IN (?, ?)`, from, into)
	if err != nil {
		return err
	}
	if n, _ := store.Scan[struct {
		N int `json:"n"`
	}](res); len(n) != 1 || n[0].N != 2 {
		return NotFound("tag", from+" or "+into)
	}
	_, err = db.Batch(ctx,
		store.Stmt{SQL: `INSERT OR IGNORE INTO entry_tags (entry_id, tag_id) SELECT entry_id, ? FROM entry_tags WHERE tag_id = ?`, Args: []any{into, from}},
		store.Stmt{SQL: `DELETE FROM tags WHERE id = ?`, Args: []any{from}},
	)
	return err
}
