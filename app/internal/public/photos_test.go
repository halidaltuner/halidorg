package public

import (
	"bytes"
	"context"
	stdimage "image"
	"image/png"
	"strings"
	"testing"

	"halid.org/engine/internal/content/photo"
	"halid.org/engine/internal/media"
	"halid.org/engine/internal/settings"
	"halid.org/engine/internal/store"
	"halid.org/engine/internal/store/storetest"
)

// seedPhotos adds two albums and three published photos (and a draft).
func seedPhotos(t *testing.T, db store.DB) []photo.Photo {
	t.Helper()
	ctx := context.Background()
	blob := storetest.NewBlob()
	m := media.NewRepo(db, blob)
	var buf bytes.Buffer
	if err := png.Encode(&buf, stdimage.NewRGBA(stdimage.Rect(0, 0, 4, 3))); err != nil {
		t.Fatal(err)
	}
	img, err := m.Upload(ctx, "frame.png", buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.SetAlt(ctx, img.ID, "Mavi saatte Galata", "Galata at blue hour"); err != nil {
		t.Fatal(err)
	}
	r := photo.NewRepo(db, blob)
	ist, _ := r.SaveAlbum(ctx, photo.Album{NameTR: "İstanbul", NameEN: "Istanbul"})
	mnt, _ := r.SaveAlbum(ctx, photo.Album{NameTR: "Dağlar", NameEN: "Mountains", Slug: "mountains", Sort: 1})
	url := "https://500px.com/photo/1/galata"
	var out []photo.Photo
	for _, p := range []photo.Photo{
		{AlbumID: ist.ID, MediaID: img.ID, TitleEN: "Galata at blue hour", TitleTR: "Mavi saatte Galata", URL500px: &url, Camera: "X-T5",
			Lens: "23mm f/1.4", Exposure: "1/30 · f/2 · ISO 800", Place: "Karaköy", RowSpan: 3, Published: true,
			DescEN: "Shot from the Karaköy pier.\nTripod, 30 seconds."},
		{AlbumID: ist.ID, MediaID: img.ID, TitleEN: "Ferry wake", Published: true},
		{AlbumID: mnt.ID, MediaID: img.ID, TitleEN: "Fog over Uludağ", RowSpan: 2, ColSpan: 2, Published: true},
		{AlbumID: mnt.ID, MediaID: img.ID, TitleEN: "Draft frame"},
	} {
		saved, err := r.Save(ctx, p)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, saved)
	}
	return out
}

func TestPhotosPage(t *testing.T) {
	mux, db := newSite(t)
	_, body := get(t, mux, "/en/photos")
	must(t, body, `No photos published yet.`)
	if strings.Contains(body, "ph-profile") {
		t.Error("profile button without a profile")
	}
	ps := seedPhotos(t, db)
	if err := settings.Set(context.Background(), db, settings.PhotosKey, settings.Photos{Profile: "https://500px.com/p/halid"}); err != nil {
		t.Fatal(err)
	}
	code, body := get(t, mux, "/en/photos")
	if code != 200 {
		t.Fatalf("photos %d", code)
	}
	must(t, body,
		`<h1>Streets, mountains and the workbench.</h1>`, `<a class="ph-profile" href="https://500px.com/p/halid" rel="noopener">500px profile ↗</a>`,
		`<a class="chip tone-violet" href="/en/photos" data-filter-key="" aria-current="true">all · 3</a>`,
		`<a class="chip tone-violet" href="/en/photos?album=istanbul" data-filter-key="istanbul">Istanbul · 2</a>`,
		`<figure class="ph-tile r3 c1" data-filter-value="istanbul">`, `<figure class="ph-tile r2 c2" data-filter-value="mountains">`,
		`alt="Galata at blue hour"`, `<a class="ph-open mono" href="#p-`+ps[0].ID+`">Galata at blue hour ↗</a>`,
		`<div class="ph-box" id="p-`+ps[0].ID+`" role="dialog" aria-modal="true" aria-label="Galata at blue hour">`,
		`<h2>Galata at blue hour</h2><p class="ph-desc">Shot from the Karaköy pier.
Tripod, 30 seconds.</p>`,
		`<dt>camera</dt><dd>X-T5</dd><dt>lens</dt><dd>23mm f/1.4</dd><dt>exposure</dt><dd>1/30 · f/2 · ISO 800</dd><dt>place</dt><dd>Karaköy</dd>`,
		`<a class="ph-500" href="https://500px.com/photo/1/galata" rel="noopener">view on 500px ↗</a>`,
		`/static/js/photos.js`, `href="/en/photos/feed.xml"`,
	)
	if strings.Contains(body, "Draft frame") || strings.Contains(body, "<iframe") || strings.Contains(body, "500px.com/embed") {
		t.Error("draft shown or a 500px embed on the page")
	}
	_, body = get(t, mux, "/tr/photos?album=mountains")
	must(t, body, `data-filtered="mountains"`, `Dağlar · 1`, `<meta name="robots" content="noindex">`)
	if strings.Contains(body, "Mavi saatte Galata ↗") {
		t.Error("filter kept another album")
	}
	_, body = get(t, mux, "/tr/photos")
	must(t, body, `Mavi saatte Galata ↗`, `alt="Mavi saatte Galata"`, `Ferry wake ↗`, `Shot from the Karaköy pier.`) // tr falls back to en

	// An image without alt text (uploaded from an album) takes the title.
	if _, err := media.NewRepo(db, nil).SetAlt(context.Background(), ps[0].MediaID, "", ""); err != nil {
		t.Fatal(err)
	}
	_, body = get(t, mux, "/en/photos")
	must(t, body, `alt="Ferry wake"`, `alt="Galata at blue hour"`)
}
