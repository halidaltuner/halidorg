package public

import (
	"bytes"
	"context"
	stdimage "image"
	stdpng "image/png"
	"strings"
	"testing"

	"halid.org/engine/internal/content"
	"halid.org/engine/internal/content/presentation"
	"halid.org/engine/internal/media"
	"halid.org/engine/internal/store"
	"halid.org/engine/internal/store/storetest"
)

// seedPresentations publishes a talk with a deck, outline and links, and a
// lightning talk without a deck; a draft workshop stays off the site.
func seedPresentations(t *testing.T, db store.DB) (talk, lightning presentation.Presentation) {
	t.Helper()
	ctx := context.Background()
	r := presentation.NewRepo(db)
	day := int64(1789948800) // 2026-09-21
	mins := 32
	rec := "https://youtube.com/watch?v=x"
	add := func(d presentation.Draft, deck string) presentation.Presentation {
		p, err := r.Create(ctx, d)
		if err != nil {
			t.Fatal(err)
		}
		if deck != "" {
			// The upload fills the outline in the editor; saving puts it in
			// the revision that publish makes public.
			up, err := r.SetDeck(ctx, storetest.NewBlob(), p.ID, []byte(deck))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := r.Save(ctx, presentation.Draft{Presentation: up, Markdown: d.Markdown}, content.OriginManual); err != nil {
				t.Fatal(err)
			}
		}
		if p, err = r.Publish(ctx, p.ID); err != nil {
			t.Fatal(err)
		}
		return p
	}
	talk = add(presentation.Draft{Presentation: presentation.Presentation{
		Entry: content.Entry{Slug: "htmx", Lang: "en", Title: "htmx in production", Summary: "What a year of hypermedia taught me."},
		Event: "GoTürkiye 2026", City: "Istanbul", HeldOn: &day, RecordingURL: &rec, RecordingMinutes: &mins,
		Links: []presentation.Link{{Label: "Notes on an htmx-first admin", URL: "/en/posts/htmx-admin"}, {Label: "htmx.org", URL: "https://htmx.org"}}},
		Markdown: "A year ago I replaced a React admin."},
		`<html><body><section class="slide"><h1>htmx in production</h1></section><section class="slide"><h2>Why leave the SPA</h2></section></body></html>`)
	lightning = add(presentation.Draft{Presentation: presentation.Presentation{
		Entry: content.Entry{Slug: "homelab", Lang: "en", Title: "A homelab you can live with"}, Type: "lightning", Event: "DevFest"}}, "")
	if _, err := r.Create(ctx, presentation.Draft{Presentation: presentation.Presentation{
		Entry: content.Entry{Slug: "radar", Lang: "en", Title: "Draft workshop"}, Type: "workshop"}}); err != nil {
		t.Fatal(err)
	}
	return talk, lightning
}

func TestPresentationsPages(t *testing.T) {
	t.Setenv("DECKS_ORIGIN", "https://decks.halid.org")
	mux, db := newSite(t)
	_, body := get(t, mux, "/en/presentations")
	must(t, body, `No presentations published yet.`)
	talk, _ := seedPresentations(t, db)
	talk, _ = presentation.NewRepo(db).Get(context.Background(), talk.ID)

	code, body := get(t, mux, "/en/presentations")
	if code != 200 {
		t.Fatalf("list %d", code)
	}
	must(t, body,
		`<h1>Talks, workshops and lightning sessions.</h1>`,
		`<a class="chip tone-rose" href="/en/presentations?type=talk" data-filter-key="talk">talk</a>`,
		`<a class="pr-card" href="/en/presentations/htmx" data-filter-value="talk">`,
		`<span class="pr-badge mono">2 slides · html</span>`,
		`<span class="pr-event">GoTürkiye 2026</span><span>en</span>`,
		`What a year of hypermedia taught me.`, `2026-09-21 · Istanbul`,
		`<div class="pr-slide tone-rose" aria-hidden="true"><span class="pr-slide-event mono">DevFest</span><span class="pr-slide-title">A homelab you can live with</span></div>`,
		`/static/js/filter.js`, `href="/en/presentations/feed.xml"`,
	)
	if strings.Contains(body, "Draft workshop") || strings.Contains(body, "?type=workshop") {
		t.Error("a draft is listed")
	}
	_, body = get(t, mux, "/en/presentations?type=lightning")
	must(t, body, `data-filtered="lightning"`, `<meta name="robots" content="noindex">`)
	if strings.Contains(body, "htmx in production") {
		t.Error("filter kept a talk")
	}

	code, body = get(t, mux, "/en/presentations/htmx")
	if code != 200 {
		t.Fatalf("detail %d", code)
	}
	deck := "https://decks.halid.org/" + talk.ID + "/?v=" + *talk.DeckVersion
	must(t, body,
		`<span class="prd-meta mono">GoTürkiye 2026 · 2026-09-21 · Istanbul · en</span>`,
		`<div class="prd-deck" data-deck data-deck-origin="https://decks.halid.org" data-total="2">`,
		`<iframe src="`+deck+`" title="Deck: htmx in production" sandbox="allow-scripts allow-same-origin allow-popups allow-popups-to-escape-sandbox" allow="fullscreen" referrerpolicy="no-referrer"></iframe>`,
		`<span class="prd-count" data-deck-count aria-live="polite">1 / 2</span>`,
		`<h2>Abstract</h2>`, `A year ago I replaced a React admin.`,
		`<button type="button" data-deck-go="2"><span class="prd-n mono">02</span><span>Why leave the SPA</span><span class="prd-s mono">slide 2</span></button>`,
		`<a class="prd-primary" href="`+deck+`" rel="noopener">open deck ↗</a>`,
		`href="https://decks.halid.org/`+talk.ID+`/htmx.html?v=`+*talk.DeckVersion+`" download>download .html</a>`,
		`watch recording · 32 min</a>`,
		`<a href="/en/posts/htmx-admin"><span aria-hidden="true">¶</span> Notes on an htmx-first admin</a>`,
		`<a href="https://htmx.org" rel="noopener"><span aria-hidden="true">↗</span> htmx.org</a>`,
		`/static/js/deck.js`, `og:image`,
	)
	if strings.Contains(body, `src="/presentations`) || strings.Contains(body, "halid.org/decks") {
		t.Error("deck served from the main origin")
	}

	// Without a deck: the title card, no controls, no deck script.
	_, body = get(t, mux, "/en/presentations/homelab")
	must(t, body, `<div class="prd-deck prd-nodeck">`, `The deck for this presentation is not uploaded yet.`)
	if strings.Contains(body, "<iframe") || strings.Contains(body, "deck.js") || strings.Contains(body, "data-deck-go") {
		t.Error("deck controls without a deck")
	}
	if code, _ := get(t, mux, "/en/presentations/radar"); code != 404 {
		t.Errorf("draft page %d", code)
	}

	// A cover without alt text (uploaded straight to media) takes the title.
	var png bytes.Buffer
	_ = stdpng.Encode(&png, stdimage.NewRGBA(stdimage.Rect(0, 0, 16, 9)))
	img, err := media.NewRepo(db, storetest.NewBlob()).Upload(context.Background(), "title.png", png.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	r := presentation.NewRepo(db)
	talk.CoverMediaID = &img.ID
	if _, err := r.Save(context.Background(), presentation.Draft{Presentation: talk, Markdown: "A year ago I replaced a React admin."}, content.OriginManual); err != nil {
		t.Fatalf("cover without alt: %v", err)
	}
	if _, err := r.Publish(context.Background(), talk.ID); err != nil {
		t.Fatal(err)
	}
	_, body = get(t, mux, "/en/presentations")
	must(t, body, `<img src="/`+img.R2Key+`" alt="htmx in production"`)

	// Home and About get the talks card.
	_, body = get(t, mux, "/en/")
	must(t, body, `href="/en/presentations"`, `2 talks`)
}
