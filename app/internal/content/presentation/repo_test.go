package presentation

import (
	"context"
	"errors"
	"testing"

	"halid.org/engine/internal/content"
	"halid.org/engine/internal/store/storetest"
)

func ptr[T any](v T) *T { return &v }

const deck = `<!doctype html><html><body>
<section class="slide"><h1>htmx in production</h1></section>
<section class="slide"><h2>Why leave the SPA</h2></section>
<section class="slide"><p>picture</p></section>
</body></html>`

func TestPresentationRowsDeckAndRevisions(t *testing.T) {
	ctx := context.Background()
	db := storetest.NewDB(t)
	blob := storetest.NewBlob()
	r := NewRepo(db)

	var v content.ValidationError
	_, err := r.Create(ctx, Draft{Presentation: Presentation{Entry: content.Entry{Slug: "htmx", Lang: "en", Title: "htmx"},
		Type: "keynote", RecordingURL: ptr("http://youtube.com/x"), Links: []Link{{Label: "x", URL: "javascript:alert(1)"}}}})
	if !errors.As(err, &v) || v["pres_type"] == "" || v["recording_url"] == "" || v["links.0.url"] == "" {
		t.Fatalf("bad fields: %v", err)
	}

	p, err := r.Create(ctx, Draft{
		Presentation: Presentation{Entry: content.Entry{Slug: "htmx", Lang: "en", Title: "htmx in production"},
			Event: " GoTürkiye 2026 ", City: "İstanbul", HeldOn: ptr[int64](1790035200), RecordingURL: ptr(" "),
			Links: []Link{{}, {Label: "notes", URL: "/en/notes/htmx"}}},
		Markdown: "A year of hypermedia.",
	})
	if err != nil {
		t.Fatal(err)
	}
	got, _ := r.Get(ctx, p.ID)
	if got.Type != "talk" || got.TalkLang != "en" || got.Event != "GoTürkiye 2026" || got.RecordingURL != nil ||
		len(got.Links) != 1 || got.Links[0].Sort != 0 || got.DeckURL() != "" {
		t.Fatalf("created %+v", got)
	}

	// The deck fills the slide count and an empty outline.
	t.Setenv("DECKS_ORIGIN", "https://decks.example/")
	if _, err := r.SetDeck(ctx, blob, p.ID, []byte("%PDF-1.4")); !errors.Is(err, ErrDeckType) {
		t.Errorf("pdf deck: %v", err)
	}
	got, err = r.SetDeck(ctx, blob, p.ID, []byte(deck))
	if err != nil {
		t.Fatal(err)
	}
	if got.SlideCount != 3 || len(got.Outline) != 2 || got.Outline[1] != (Slide{2, "Why leave the SPA"}) ||
		blob.Objects[DeckKey(p.ID)].ContentType != "text/html; charset=utf-8" {
		t.Fatalf("deck %+v", got)
	}
	if u := got.DeckURL(); u != "https://decks.example/"+p.ID+"/?v="+*got.DeckVersion || len(*got.DeckVersion) != 12 {
		t.Errorf("deck url %q", u)
	}
	if u := got.DownloadURL(); u != "https://decks.example/"+p.ID+"/htmx.html?v="+*got.DeckVersion {
		t.Errorf("download url %q", u)
	}

	// Saving keeps the deck; an edited outline survives the next upload.
	d := Draft{Presentation: got, Markdown: "A year of hypermedia."}
	d.Outline = []Slide{{3, "The picture"}, {1, "Intro"}}
	if _, err := r.Save(ctx, d, content.OriginManual); err != nil {
		t.Fatal(err)
	}
	d.Outline = []Slide{{4, "past the end"}}
	if _, err := r.Save(ctx, d, content.OriginManual); !errors.As(err, &v) || v["outline.0.n"] == "" {
		t.Errorf("slide past the deck: %v", err)
	}
	got, _ = r.SetDeck(ctx, blob, p.ID, []byte(deck+"<!-- v2 -->"))
	if got.DeckKey == nil || len(got.Outline) != 2 || got.Outline[0] != (Slide{1, "Intro"}) || got.Outline[1].N != 3 {
		t.Fatalf("after save and upload %+v", got)
	}

	// The public page shows the published revision's outline and links.
	if _, err := r.Publish(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	pub, err := r.GetPublished(ctx, "en", "htmx")
	if err != nil || pub.HTML == "" || len(pub.Outline) != 2 || len(pub.Links) != 1 || pub.SlideCount != 3 {
		t.Fatalf("published %+v %v", pub, err)
	}
	list, _ := r.List(ctx, ListFilter{Lang: "en"})
	if len(list) != 1 || list[0].Event != "GoTürkiye 2026" {
		t.Errorf("list %+v", list)
	}
	if err := DeleteDeck(ctx, blob, p.ID); err != nil || len(blob.Objects) != 0 {
		t.Errorf("delete deck %v %d", err, len(blob.Objects))
	}
}
