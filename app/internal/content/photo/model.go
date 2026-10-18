// Package photo holds photos and albums (Phase 5.3, CONTENT_MODEL.md §3):
// gallery data without entries, one list for both languages with tr/en
// titles. Each photo shows an image from media and links to its 500px page
// (ADR-009); camera details can be read from the JPEG's EXIF.
package photo

// Album groups photos; it has a name per language.
type Album struct {
	ID     string `json:"id"`
	Slug   string `json:"slug"`
	NameTR string `json:"name_tr"`
	NameEN string `json:"name_en"`
	Sort   int    `json:"sort"`
}

// Name is the album's name in lang.
func (a Album) Name(lang string) string {
	if lang == "tr" {
		return a.NameTR
	}
	return a.NameEN
}

// Photo is one frame.
type Photo struct {
	ID        string  `json:"id"`
	AlbumID   string  `json:"album_id"`
	MediaID   string  `json:"media_id"`
	TitleTR   string  `json:"title_tr"`
	TitleEN   string  `json:"title_en"`
	DescTR    string  `json:"description_tr"`
	DescEN    string  `json:"description_en"`
	URL500px  *string `json:"url_500px"`
	Camera    string  `json:"camera"`
	Lens      string  `json:"lens"`
	Exposure  string  `json:"exposure"`
	Place     string  `json:"place"`
	TakenAt   *int64  `json:"taken_at"`
	RowSpan   int     `json:"row_span"`
	ColSpan   int     `json:"col_span"`
	Sort      int     `json:"sort"`
	Published bool    `json:"-"`
	CreatedAt int64   `json:"created_at"`
	UpdatedAt int64   `json:"updated_at"`
}

// Title is the title in lang, else the other language's.
func (p Photo) Title(lang string) string {
	if lang == "tr" && p.TitleTR != "" || p.TitleEN == "" {
		return p.TitleTR
	}
	return p.TitleEN
}

// Description is the plain-text description in lang, else the other
// language's.
func (p Photo) Description(lang string) string {
	if lang == "tr" && p.DescTR != "" || p.DescEN == "" {
		return p.DescTR
	}
	return p.DescEN
}
