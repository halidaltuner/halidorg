package public

import (
	"bytes"
	"context"
	stdimage "image"
	stdpng "image/png"
	"testing"

	"halid.org/engine/internal/content"
	"halid.org/engine/internal/content/post"
	"halid.org/engine/internal/media"
	"halid.org/engine/internal/store/storetest"
)

// A post's cover shows on its page, its card in the list and home's
// featured card; without one they draw the tone pattern.
func TestPostCover(t *testing.T) {
	mux, db := newSite(t)
	seed(t, db)
	ctx := context.Background()
	var png bytes.Buffer
	_ = stdpng.Encode(&png, stdimage.NewRGBA(stdimage.Rect(0, 0, 12, 6)))
	mr := media.NewRepo(db, storetest.NewBlob())
	img, err := mr.Upload(ctx, "flags.png", png.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mr.SetAlt(ctx, img.ID, "Duvarda bayraklar", "Flags on a wall"); err != nil {
		t.Fatal(err)
	}
	repo := post.NewRepo(db)
	p, err := repo.GetPublished(ctx, "en", "htmx-admin")
	if err != nil {
		t.Fatal(err)
	}
	p.CoverMediaID = &img.ID
	if _, _, err := repo.Save(ctx, post.Draft{Post: p, Markdown: "Body."}, content.OriginManual); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Publish(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	want := `<img src="/` + img.R2Key + `" alt="Flags on a wall"`
	for _, path := range []string{"/en/posts/htmx-admin", "/en/posts", "/en/"} {
		_, body := get(t, mux, path)
		must(t, body, want)
	}
	_, body := get(t, mux, "/en/posts/ghost")
	must(t, body, `class="cover-stripes"`)
}
