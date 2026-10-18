package photo

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"image"
	"image/jpeg"
	"testing"

	"halid.org/engine/internal/content"
	"halid.org/engine/internal/media"
	"halid.org/engine/internal/store/storetest"
)

// realJPEG is a decodable JPEG carrying the EXIF of buildJPEG.
func realJPEG(t *testing.T) []byte {
	t.Helper()
	var img bytes.Buffer
	if err := jpeg.Encode(&img, image.NewRGBA(image.Rect(0, 0, 8, 8)), nil); err != nil {
		t.Fatal(err)
	}
	exif := buildJPEG(binary.BigEndian)
	app1 := exif[2:bytes.Index(exif, []byte{0xFF, 0xDA})]
	b := img.Bytes()
	return append(append(append([]byte(nil), b[:2]...), app1...), b[2:]...)
}

func TestRepo(t *testing.T) {
	ctx := context.Background()
	db, blob := storetest.NewDB(t), storetest.NewBlob()
	m := media.NewRepo(db, blob)
	img, err := m.Upload(ctx, "galata.jpg", realJPEG(t))
	if err != nil {
		t.Fatal(err)
	}
	r := NewRepo(db, blob)
	al, err := r.SaveAlbum(ctx, Album{NameTR: "İstanbul", NameEN: "Istanbul"})
	if err != nil || al.Slug != "istanbul" {
		t.Fatalf("album %+v %v", al, err)
	}
	var v content.ValidationError
	p := Photo{AlbumID: al.ID, MediaID: img.ID, TitleEN: "Galata at blue hour", Place: "Karaköy", Published: true}
	if _, err := r.Save(ctx, p); !errors.As(err, &v) || v["image"] == "" {
		t.Fatalf("image without alt: %v", err)
	}
	if _, err := m.SetAlt(ctx, img.ID, "Mavi saatte Galata", "Galata at blue hour"); err != nil {
		t.Fatal(err)
	}
	bad := "https://evil.dev/x"
	p.URL500px = &bad
	if _, err := r.Save(ctx, p); !errors.As(err, &v) || v["url_500px"] == "" {
		t.Errorf("non-500px link: %v", err)
	}
	good := "https://500px.com/photo/1/galata"
	p.URL500px = &good
	saved, err := r.Save(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	// Empty camera fields came from the JPEG; typed ones are kept.
	if saved.Camera != "X-T5" || saved.Exposure != "1/250 · f/8 · ISO 160" || Date(saved.TakenAt) != "2026-09-14" || saved.RowSpan != 2 || saved.ColSpan != 1 {
		t.Errorf("exif fill %+v", saved)
	}
	saved.Camera = "X100V"
	if again, _ := r.Save(ctx, saved); again.Camera != "X100V" {
		t.Error("typed camera overwritten")
	}
	if saved.Title("tr") != "Galata at blue hour" {
		t.Error("tr title falls back to en")
	}
	draft := Photo{AlbumID: al.ID, MediaID: img.ID, TitleTR: "taslak"}
	if _, err := r.Save(ctx, draft); err != nil {
		t.Fatal(err)
	}
	if pub, _ := r.Published(ctx); len(pub) != 1 {
		t.Errorf("published %d", len(pub))
	}
	if err := r.DeleteAlbum(ctx, al.ID); err == nil {
		t.Error("deleted an album with photos")
	}
	if err := m.Delete(ctx, img.ID); !errors.Is(err, media.ErrInUse) {
		t.Errorf("media in use: %v", err)
	}
}
