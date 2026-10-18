package public

import (
	"net/http"
	"strconv"

	"halid.org/engine/internal/content/photo"
	"halid.org/engine/internal/i18n"
	"halid.org/engine/internal/settings"
)

// Photos (design/Photos.dc.html), Phase 5.3: the violet hero with the 500px
// profile button, album chips with counts (?album=, filter.js), the dense
// grid of local images (ADR-009) and a lightbox per photo. The lightbox is
// a :target panel, so it works without JS; photos.js adds Esc.

type photoTile struct {
	ID, Title, Description, Album, AlbumKey, Camera, Lens, Exposure, Place, Date, URL500px string
	Rows, Cols                                                                             int
	Image                                                                                  image
}

type photosPage struct {
	page
	Profile string
	Chips   []filterChip
	Tiles   []photoTile
	Album   string // server-side filter (no-JS path); "" on the cached page
}

func (h *Handlers) photos(w http.ResponseWriter, r *http.Request, lang string) {
	ctx := r.Context()
	repo := photo.NewRepo(h.db, nil)
	albums, err := repo.Albums(ctx)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	list, err := repo.Published(ctx)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	var s settings.Photos
	if err := settings.Get(ctx, h.db, settings.PhotosKey, &s); err != nil {
		h.fail(w, r, err)
		return
	}
	ids := make([]string, 0, len(list))
	for _, x := range list {
		ids = append(ids, x.MediaID)
	}
	imgs, err := h.images(ctx, lang, ids)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	byID := map[string]photo.Album{}
	for _, a := range albums {
		byID[a.ID] = a
	}
	p := photosPage{page: h.newPage(lang, "photos", i18n.T(lang, "photo.title")+" · halid.org", "/photos", nil), Profile: s.Profile}
	p.Head.Description = i18n.T(lang, "photo.sub")
	p.Head.Feeds = []feedLink{{Title: i18n.T(lang, "feed.photos"), Href: "/" + lang + "/photos/feed.xml"}}
	count := map[string]int{}
	for _, x := range list {
		im, ok := imgs[x.MediaID]
		if !ok {
			continue
		}
		if im.Alt == "" {
			im.Alt = x.Title(lang) // photos uploaded from an album carry no alt text of their own
		}
		a := byID[x.AlbumID]
		count[a.Slug]++
		t := photoTile{ID: x.ID, Title: x.Title(lang), Description: x.Description(lang), Album: a.Name(lang), AlbumKey: a.Slug, Camera: x.Camera, Lens: x.Lens,
			Exposure: x.Exposure, Place: x.Place, Date: photo.Date(x.TakenAt), Rows: x.RowSpan, Cols: x.ColSpan, Image: im}
		if x.URL500px != nil {
			t.URL500px = *x.URL500px
		}
		p.Tiles = append(p.Tiles, t)
	}
	var keys []string
	names := map[string]string{}
	for _, a := range albums {
		if count[a.Slug] > 0 {
			keys = append(keys, a.Slug)
			names[a.Slug] = a.Name(lang)
		}
	}
	p.Album = r.URL.Query().Get("album")
	if count[p.Album] == 0 {
		p.Album = ""
	}
	p.Chips = chips(lang, "/"+lang+"/photos", "album", p.Album, keys, func(k string) string {
		return names[k] + " · " + strconv.Itoa(count[k])
	})
	if len(p.Chips) > 0 {
		p.Chips[0].Label += " · " + strconv.Itoa(len(p.Tiles))
	}
	if p.Album != "" {
		kept := p.Tiles[:0]
		for _, t := range p.Tiles {
			if t.AlbumKey == p.Album {
				kept = append(kept, t)
			}
		}
		p.Tiles = kept
		p.Head.Noindex = true
	}
	p.Head.Scripts = []string{"/static/js/filter.js", "/static/js/photos.js"}
	h.render(w, r, "photos", p)
}
