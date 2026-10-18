package feeds

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"net/http"
	"strings"
	"testing"

	"halid.org/engine/internal/content/photo"
	"halid.org/engine/internal/media"
	"halid.org/engine/internal/store/storetest"
)

func TestPhotoFeed(t *testing.T) {
	t.Setenv("PUBLIC_ORIGIN", "https://halid.org")
	ctx := context.Background()
	db, blob := storetest.NewDB(t), storetest.NewBlob()
	var buf bytes.Buffer
	_ = png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 2, 2)))
	m := media.NewRepo(db, blob)
	img, _ := m.Upload(ctx, "f.png", buf.Bytes())
	_, _ = m.SetAlt(ctx, img.ID, "kare", "frame")
	r := photo.NewRepo(db, blob)
	al, _ := r.SaveAlbum(ctx, photo.Album{NameTR: "Sokak", NameEN: "Street"})
	p, err := r.Save(ctx, photo.Photo{AlbumID: al.ID, MediaID: img.ID, TitleEN: "Simit vendor", Place: "Eminönü", Published: true})
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	New(db).Register(mux)
	f := get(t, mux, "/en/photos/feed.xml").Body.String()
	for _, want := range []string{"<title>Simit vendor</title>", "<link>https://halid.org/en/photos#p-" + p.ID + "</link>", "https://halid.org/" + img.R2Key} {
		if !strings.Contains(f, want) {
			t.Errorf("feed misses %s", want)
		}
	}
	if sm := get(t, mux, "/sitemap.xml").Body.String(); !strings.Contains(sm, "<loc>https://halid.org/en/photos</loc>") {
		t.Error("sitemap misses photos")
	}
}
