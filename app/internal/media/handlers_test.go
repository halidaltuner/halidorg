package media

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"halid.org/engine/internal/admin"
	"halid.org/engine/internal/auth"
	"halid.org/engine/internal/store"
	"halid.org/engine/internal/store/storetest"
)

func pngBytes(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	img.Set(0, 0, color.RGBA{1, 2, 3, 255})
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

type site struct {
	mux  *http.ServeMux
	s    store.Stores
	c    *http.Cookie
	repo *Repo
}

func newSite(t *testing.T) site {
	t.Helper()
	t.Setenv("APP_ENV", "staging")
	s := storetest.Stores(t)
	a := auth.New(auth.Config{SessionSecret: strings.Repeat("k", 32)}, s.DB)
	repo := NewRepo(s.DB, s.Blob)
	h, err := New(repo, admin.NewUI(s.DB), a)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	h.Register(mux)
	c, err := a.TestSession(context.Background(), "me@halid.org", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	return site{mux: mux, s: s, c: c, repo: repo}
}

func (st site) upload(t *testing.T, xhr bool, files map[string][]byte) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	for name, data := range files {
		fw, _ := mw.CreateFormFile("file", name)
		_, _ = fw.Write(data)
	}
	_ = mw.Close()
	req := httptest.NewRequest("POST", "/admin/media", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	req.Header.Set("Accept-Language", "en")
	if xhr {
		req.Header.Set("X-Upload", "1")
	}
	req.AddCookie(st.c)
	rec := httptest.NewRecorder()
	st.mux.ServeHTTP(rec, req)
	return rec
}

func (st site) post(target string, form url.Values, htmx bool) *httptest.ResponseRecorder {
	req := httptest.NewRequest("POST", target, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	req.Header.Set("Accept-Language", "en")
	if htmx {
		req.Header.Set("HX-Request", "true")
	}
	req.AddCookie(st.c)
	rec := httptest.NewRecorder()
	st.mux.ServeHTTP(rec, req)
	return rec
}

func TestUploadAltAndDelete(t *testing.T) {
	st := newSite(t)
	ctx := context.Background()
	blob := st.s.Blob.(*storetest.Blob)

	// Plain form, two files: the image is stored, the text file is refused
	// by its bytes even though its name says .png.
	rec := st.upload(t, false, map[string][]byte{"Ekran Görüntüsü 1.PNG": pngBytes(t, 40, 30), "evil.png": []byte("<svg onload=alert(1)>")})
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "evil.png: Not a PNG, JPEG, GIF or WebP image.") {
		t.Fatalf("mixed upload %d %s", rec.Code, rec.Body.String())
	}
	items, _ := st.repo.List(ctx, 0)
	if len(items) != 1 {
		t.Fatalf("items %+v", items)
	}
	it := items[0]
	if it.Mime != "image/png" || *it.Width != 40 || *it.Height != 30 || it.Name != "Ekran Görüntüsü 1.PNG" ||
		!strings.HasSuffix(it.R2Key, "/ekran-goruntusu-1.png") || it.Ready() {
		t.Fatalf("item %+v", it)
	}
	if obj, ok := blob.Objects[it.R2Key]; !ok || obj.ContentType != "image/png" {
		t.Fatalf("R2 object missing: %v", ok)
	}

	// The grid shows it as not usable yet: copy is disabled.
	body := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/admin/media", nil)
	req.AddCookie(st.c)
	req.Header.Set("Accept-Language", "en")
	st.mux.ServeHTTP(body, req)
	if !strings.Contains(body.Body.String(), `alt text missing`) || !strings.Contains(body.Body.String(), `]("/media/`) && !strings.Contains(body.Body.String(), "disabled title=") {
		t.Fatalf("grid:\n%s", body.Body.String())
	}

	// XHR upload (JS island): 201 with the card fragment.
	rec = st.upload(t, true, map[string][]byte{"b.png": pngBytes(t, 2, 2)})
	if rec.Code != http.StatusCreated || !strings.HasPrefix(rec.Body.String(), `<article class="md-card"`) {
		t.Fatalf("xhr upload %d %s", rec.Code, rec.Body.String())
	}

	// One alt is not enough; both make it ready.
	st.post("/admin/media/"+it.ID+"/alt", url.Values{"alt_tr": {"ekran"}}, true)
	if err := st.repo.CheckCover(ctx, it.ID); err != ErrNotReady {
		t.Fatalf("half alt: %v", err)
	}
	rec = st.post("/admin/media/"+it.ID+"/alt", url.Values{"alt_tr": {"ekran"}, "alt_en": {"screen"}}, true)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "ready to use") || strings.Contains(rec.Body.String(), "disabled") {
		t.Fatalf("alt save %d %s", rec.Code, rec.Body.String())
	}
	if err := st.repo.CheckCover(ctx, it.ID); err != nil {
		t.Fatal(err)
	}

	// A cover cannot be deleted.
	if _, err := st.s.DB.Query(ctx, `INSERT INTO entries (id, kind, slug, lang, translation_group_id, title, status, visibility, created_at, updated_at, cover_media_id)
VALUES ('E1', 'post', 'p', 'en', 'G1', 'P', 'draft', 'public', 1, 1, ?)`, it.ID); err != nil {
		t.Fatal(err)
	}
	if rec := st.post("/admin/media/"+it.ID+"/delete", url.Values{}, false); rec.Code != http.StatusConflict {
		t.Fatalf("delete in use %d", rec.Code)
	}
	if _, err := st.s.DB.Query(ctx, `UPDATE entries SET cover_media_id = NULL`); err != nil {
		t.Fatal(err)
	}
	if rec := st.post("/admin/media/"+it.ID+"/delete", url.Values{}, false); rec.Code != http.StatusSeeOther {
		t.Fatalf("delete %d", rec.Code)
	}
	if _, ok := blob.Objects[it.R2Key]; ok {
		t.Error("R2 object left behind")
	}
}

func TestUploadTooLarge(t *testing.T) {
	st := newSite(t)
	big := append(pngBytes(t, 1, 1), make([]byte, MaxBytes)...)
	if rec := st.upload(t, true, map[string][]byte{"big.png": big}); rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("too large %d", rec.Code)
	}
}

// The block editor uploads one file and inserts it from the JSON reply.
func TestUploadJSON(t *testing.T) {
	st := newSite(t)
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, _ := mw.CreateFormFile("file", "Shot.png")
	_, _ = fw.Write(pngBytes(t, 4, 3))
	_ = mw.Close()
	req := httptest.NewRequest("POST", "/admin/media", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	req.Header.Set("X-Upload", "1")
	req.Header.Set("Accept", "application/json")
	req.AddCookie(st.c)
	rec := httptest.NewRecorder()
	st.mux.ServeHTTP(rec, req)
	var got struct {
		ID, URL, Name, Mime string
		Image               bool
	}
	if rec.Code != http.StatusCreated || json.Unmarshal(rec.Body.Bytes(), &got) != nil {
		t.Fatalf("json upload %d %s", rec.Code, rec.Body.String())
	}
	if got.ID == "" || !strings.HasPrefix(got.URL, "/media/") || got.Name != "Shot.png" || got.Mime != "image/png" || !got.Image {
		t.Fatalf("reply %+v", got)
	}
}
