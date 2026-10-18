package content

import (
	"context"
	"errors"
	"slices"
	"strings"

	"halid.org/engine/internal/store"
)

// Category groups links, snippets, favs, homelab builds and awesome lists
// (CONTENT_MODEL.md, categories). The admin for it comes in Phase 2.5.
type Category struct {
	ID       string  `json:"id"`
	Kind     string  `json:"kind"`
	Slug     string  `json:"slug"`
	NameTR   string  `json:"name_tr"`
	NameEN   string  `json:"name_en"`
	Tone     *string `json:"tone"`
	ParentID *string `json:"parent_id"`
	Sort     int     `json:"sort"`
}

// Name is the category name in lang, falling back to the other one, then the slug.
func (c Category) Name(lang string) string {
	a, b := c.NameEN, c.NameTR
	if lang == "tr" {
		a, b = b, a
	}
	switch {
	case a != "":
		return a
	case b != "":
		return b
	}
	return c.Slug
}

var categoryKinds = map[string]bool{"link": true, "snippet": true, "fav": true, "homelab": true, "awesome": true}

const categoryColumns = `id, kind, slug, name_tr, name_en, tone, parent_id, sort`

// CreateCategory validates and inserts c, filling its id.
func CreateCategory(ctx context.Context, db store.DB, c *Category) error {
	v := ValidationError{}
	if !categoryKinds[c.Kind] {
		v["kind"] = "unknown category kind"
	}
	if !ValidSlug(c.Slug) {
		v["slug"] = "lowercase letters, digits and hyphens, 1 to 80 characters"
	}
	if strings.TrimSpace(c.NameTR) == "" && strings.TrimSpace(c.NameEN) == "" {
		v["name"] = "a TR or EN name is required"
	}
	if c.Tone != nil && !tones[*c.Tone] {
		v["tone"] = "unknown tone"
	}
	if len(v) > 0 {
		return v
	}
	if c.ID == "" {
		c.ID = NewID()
	}
	_, err := db.Query(ctx, `INSERT INTO categories (`+categoryColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		c.ID, c.Kind, c.Slug, c.NameTR, c.NameEN, c.Tone, c.ParentID, c.Sort)
	if err != nil && strings.Contains(strings.ToLower(err.Error()), "unique constraint") {
		return ValidationError{"slug": "already used by another category of this kind"}
	}
	return err
}

// Categories lists the categories of kind in display order.
func Categories(ctx context.Context, db store.DB, kind string) ([]Category, error) {
	res, err := db.Query(ctx, `SELECT `+categoryColumns+` FROM categories WHERE kind = ? ORDER BY sort, slug`, kind)
	if err != nil {
		return nil, err
	}
	return store.Scan[Category](res)
}

// CheckCategory confirms that id is a category of kind. A nil id is fine.
func CheckCategory(ctx context.Context, db store.DB, id *string, kind string) error {
	if id == nil {
		return nil
	}
	res, err := db.Query(ctx, `SELECT kind FROM categories WHERE id = ?`, *id)
	if err != nil {
		return err
	}
	rows, err := store.Scan[struct {
		Kind string `json:"kind"`
	}](res)
	if err != nil {
		return err
	}
	if len(rows) == 0 || rows[0].Kind != kind {
		return ValidationError{"category": "no such " + kind + " category"}
	}
	return nil
}

// Tones lists the DESIGN.md section tones in picker order.
var Tones = []string{"cyan", "indigo", "amber", "green", "orange", "rose", "violet", "navy"}

// GetCategory loads one category.
func GetCategory(ctx context.Context, db store.DB, id string) (Category, error) {
	res, err := db.Query(ctx, `SELECT `+categoryColumns+` FROM categories WHERE id = ?`, id)
	if err != nil {
		return Category{}, err
	}
	rows, err := store.Scan[Category](res)
	if err != nil {
		return Category{}, err
	}
	if len(rows) == 0 {
		return Category{}, NotFound("category", id)
	}
	return rows[0], nil
}

// UpdateCategory renames c, sets its tone and parent. Kind and slug stay:
// the slug is in awesome list URLs. A parent must be a top-level category
// of the same kind, so lists nest one level.
func UpdateCategory(ctx context.Context, db store.DB, c Category) error {
	v := ValidationError{}
	if strings.TrimSpace(c.NameTR) == "" && strings.TrimSpace(c.NameEN) == "" {
		v["name"] = "a TR or EN name is required"
	}
	if c.Tone != nil && !tones[*c.Tone] {
		v["tone"] = "unknown tone"
	}
	if c.ParentID != nil {
		p, err := GetCategory(ctx, db, *c.ParentID)
		switch {
		case errors.Is(err, ErrNotFound):
			v["parent"] = "no such category"
		case err != nil:
			return err
		case p.ID == c.ID || p.Kind != c.Kind || p.ParentID != nil:
			v["parent"] = "a top-level category of the same kind, not itself"
		}
		if v["parent"] == "" {
			res, err := db.Query(ctx, `SELECT COUNT(*) AS n FROM categories WHERE parent_id = ?`, c.ID)
			if err != nil {
				return err
			}
			if n, _ := store.Scan[struct {
				N int `json:"n"`
			}](res); len(n) == 1 && n[0].N > 0 {
				v["parent"] = "has categories under it, so it stays top-level"
			}
		}
	}
	if len(v) > 0 {
		return v
	}
	res, err := db.Query(ctx, `UPDATE categories SET name_tr = ?, name_en = ?, tone = ?, parent_id = ? WHERE id = ?`,
		strings.TrimSpace(c.NameTR), strings.TrimSpace(c.NameEN), c.Tone, c.ParentID, c.ID)
	if err != nil {
		return err
	}
	if res.Changes == 0 {
		return NotFound("category", c.ID)
	}
	return nil
}

// MoveCategory moves a category one place up (-1) or down (+1) among its
// kind, renumbering the kind's sort as 10, 20, 30 in one batch.
func MoveCategory(ctx context.Context, db store.DB, id string, dir int) error {
	c, err := GetCategory(ctx, db, id)
	if err != nil {
		return err
	}
	cats, err := Categories(ctx, db, c.Kind)
	if err != nil {
		return err
	}
	i := slices.IndexFunc(cats, func(x Category) bool { return x.ID == id })
	j := i + dir
	if j < 0 || j >= len(cats) {
		return nil
	}
	cats[i], cats[j] = cats[j], cats[i]
	stmts := make([]store.Stmt, len(cats))
	for k, x := range cats {
		stmts[k] = store.Stmt{SQL: `UPDATE categories SET sort = ? WHERE id = ?`, Args: []any{(k + 1) * 10, x.ID}}
	}
	_, err = db.Batch(ctx, stmts...)
	return err
}

// CategoryEntries returns the ids of the entries in a category (any kind
// table that has one), for the render jobs a change needs.
func CategoryEntries(ctx context.Context, db store.DB, id string) ([]string, error) {
	res, err := db.Query(ctx, `SELECT entry_id AS id FROM links WHERE category_id = ?
UNION SELECT entry_id FROM snippets WHERE category_id = ?
UNION SELECT entry_id FROM favs WHERE category_id = ?`, id, id, id)
	if err != nil {
		return nil, err
	}
	return ids(res)
}

func ids(res store.Result) ([]string, error) {
	rows, err := store.Scan[struct {
		ID string `json:"id"`
	}](res)
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.ID
	}
	return out, err
}
