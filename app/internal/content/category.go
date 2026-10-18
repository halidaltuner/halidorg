package content

import (
	"context"
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
