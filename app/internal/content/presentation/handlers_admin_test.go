package presentation_test

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"halid.org/engine/internal/admin"
	"halid.org/engine/internal/auth"
	"halid.org/engine/internal/content/presentation"
	"halid.org/engine/internal/jobs"
	"halid.org/engine/internal/store/storetest"
)

func TestEditorAndDeckUpload(t *testing.T) {
	t.Setenv("APP_ENV", "staging")
	t.Setenv("DECKS_ORIGIN", "https://decks.halid.org")
	ctx := context.Background()
	s := storetest.Stores(t)
	a := auth.New(auth.Config{SessionSecret: strings.Repeat("k", 32)}, s.DB)
	j := jobs.New(s.DB, s.Queue)
	ed, err := presentation.NewEditor(s.DB, s.Blob, admin.NewUI(s.DB), a, j)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	ed.Register(mux)
	presentation.NewDecks(s.DB, s.Blob, a, j).Register(mux)
	fresh, _ := a.TestSession(ctx, "me@halid.org", time.Minute)
	send := func(req *http.Request) *httptest.ResponseRecorder {
		req.Header.Set("Sec-Fetch-Site", "same-origin")
		req.Header.Set("Accept-Language", "en")
		req.AddCookie(fresh)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec
	}
	form := func(target string, f url.Values) *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", target, strings.NewReader(f.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		return send(req)
	}
	get := func(target string) string { return send(httptest.NewRequest("GET", target, nil)).Body.String() }
	upload := func(id, name string, data []byte) string {
		var b bytes.Buffer
		mw := multipart.NewWriter(&b)
		fw, _ := mw.CreateFormFile("deck", name)
		_, _ = fw.Write(data)
		_ = mw.Close()
		req := httptest.NewRequest("POST", "/admin/presentations/"+id+"/deck", &b)
		req.Header.Set("Content-Type", mw.FormDataContentType())
		rec := send(req)
		if rec.Code != http.StatusSeeOther {
			t.Fatalf("upload %s: %d %s", name, rec.Code, rec.Body.String())
		}
		return rec.Header().Get("Location")
	}

	f := url.Values{"title": {"htmx in production"}, "lang": {"en"}, "pres_type": {"talk"}, "event": {"GoTürkiye 2026"},
		"held_on": {"2026-09-21"}, "talk_lang": {"en"}, "recording_minutes": {"half an hour"},
		"link_label": {"htmx.org", ""}, "link_url": {"https://htmx.org", ""}, "link_order": {"0", "1"}, "action": {"save"}}
	if rec := form("/admin/presentations", f); rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "1 to 1000 minutes") {
		t.Fatalf("bad minutes: %d", rec.Code)
	}
	f.Set("recording_minutes", "32")
	rec := form("/admin/presentations", f)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("create %d %s", rec.Code, rec.Body.String())
	}
	id := strings.TrimPrefix(strings.SplitN(rec.Header().Get("Location"), "?", 2)[0], "/admin/presentations/")
	body := get("/admin/presentations/" + id)
	for _, want := range []string{`action="/admin/presentations/` + id + `/deck" enctype="multipart/form-data"`, `No deck yet.`,
		`&lt;script&gt;`, `window.halidDeck`, `name="held_on" value="2026-09-21"`} {
		if !strings.Contains(body, want) {
			t.Errorf("editor: missing %s", want)
		}
	}

	if loc := upload(id, "cv.pdf", []byte("%PDF-1.4")); !strings.HasSuffix(loc, "?error=deck_bad_type") {
		t.Errorf("pdf: %s", loc)
	} else if !strings.Contains(get(loc), "The deck must be one HTML file.") {
		t.Error("pdf error not shown")
	}
	deck := `<!doctype html><body><div class="slide"><h1>Intro</h1></div><div class="slide"><h2>Why leave the SPA</h2></div><div class="slide"></div></body>`
	loc := upload(id, "deck.html", []byte(deck))
	if !strings.HasSuffix(loc, "?notice=deck_done") {
		t.Fatalf("deck: %s", loc)
	}
	body = get(loc)
	for _, want := range []string{"Deck uploaded and live.", `name="outline_title" value="Why leave the SPA"`, `name="slide_count" value="3"`,
		`uploaded · 3 slides · v`, `href="https://decks.halid.org/` + id + `/?v=`} {
		if !strings.Contains(body, want) {
			t.Errorf("after upload: missing %s", want)
		}
	}
	obj, err := s.Blob.Get(ctx, presentation.DeckKey(id))
	if err != nil {
		t.Fatal(err)
	}
	if string(obj.Body) != deck {
		t.Error("stored deck differs")
	}
	if rec := send(httptest.NewRequest("GET", "/admin/presentations/"+id+"?error=../x", nil)); strings.Contains(rec.Body.String(), "../x") {
		t.Error("an error key outside [a-z_] reached the page")
	}

	// Deleting the presentation takes its deck out of R2.
	if rec := form("/admin/presentations/"+id+"/delete", url.Values{}); rec.Code != http.StatusSeeOther {
		t.Fatalf("delete %d", rec.Code)
	}
	if _, err := s.Blob.Get(ctx, presentation.DeckKey(id)); err == nil {
		t.Error("deck left in R2")
	}
}
