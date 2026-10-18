package photo_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"image"
	"image/jpeg"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"halid.org/engine/internal/admin"
	"halid.org/engine/internal/auth"
	"halid.org/engine/internal/content/photo"
	"halid.org/engine/internal/jobs"
	"halid.org/engine/internal/media"
	"halid.org/engine/internal/store/storetest"
)

// exifJPEG is a decodable JPEG with an Exif block naming the camera "X100V"
// (big-endian TIFF, IFD0 with Model only).
func exifJPEG(t *testing.T) []byte {
	t.Helper()
	var tf bytes.Buffer
	be := binary.BigEndian
	tf.WriteString("MM")
	_ = binary.Write(&tf, be, uint16(42))
	_ = binary.Write(&tf, be, uint32(8))
	_ = binary.Write(&tf, be, uint16(1))
	_ = binary.Write(&tf, be, uint16(0x0110))
	_ = binary.Write(&tf, be, uint16(2))
	_ = binary.Write(&tf, be, uint32(6))
	_ = binary.Write(&tf, be, uint32(8+2+12+4))
	_ = binary.Write(&tf, be, uint32(0))
	tf.WriteString("X100V\x00")
	seg := append([]byte("Exif\x00\x00"), tf.Bytes()...)
	var img bytes.Buffer
	if err := jpeg.Encode(&img, image.NewRGBA(image.Rect(0, 0, 8, 8)), nil); err != nil {
		t.Fatal(err)
	}
	var app1 bytes.Buffer
	app1.Write([]byte{0xFF, 0xE1})
	_ = binary.Write(&app1, be, uint16(len(seg)+2))
	app1.Write(seg)
	b := img.Bytes()
	return append(append(append([]byte(nil), b[:2]...), app1.Bytes()...), b[2:]...)
}

func TestAdmin(t *testing.T) {
	ctx := context.Background()
	s := storetest.Stores(t)
	a := auth.New(auth.Config{SessionSecret: strings.Repeat("k", 32)}, s.DB)
	h, err := photo.NewAdmin(s.DB, s.Blob, admin.NewUI(s.DB), a, jobs.New(s.DB, s.Queue))
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	h.Register(mux)
	fresh, _ := a.TestSession(ctx, "me@halid.org", time.Minute)
	do := func(method, target string, form url.Values) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, target, strings.NewReader(form.Encode()))
		if form != nil {
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}
		req.Header.Set("Sec-Fetch-Site", "same-origin")
		req.Header.Set("Accept-Language", "en")
		req.AddCookie(fresh)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec
	}
	m := media.NewRepo(s.DB, s.Blob)
	img, err := m.Upload(ctx, "x100v.jpg", exifJPEG(t))
	if err != nil {
		t.Fatal(err)
	}
	if rec := do("POST", "/admin/photos/albums", url.Values{"name_tr": {"Sokak"}, "name_en": {"Street"}}); rec.Code != http.StatusSeeOther {
		t.Fatalf("album %d %s", rec.Code, rec.Body.String())
	}
	albums, _ := photo.NewRepo(s.DB, nil).Albums(ctx)
	f := url.Values{"image": {img.ID}, "album": {albums[0].ID}, "title_en": {"Simit vendor"}, "row_span": {"2"}, "col_span": {"1"}, "published": {"1"}}
	// "Fill from EXIF" shows the camera without saving.
	f.Set("action", "exif")
	body := do("POST", "/admin/photos", f).Body.String()
	if !strings.Contains(body, `name="camera" value="X100V"`) || !strings.Contains(body, "remember to save") {
		t.Errorf("exif fill %s", body)
	}
	if all, _ := photo.NewRepo(s.DB, nil).All(ctx); len(all) != 0 {
		t.Error("exif fill saved")
	}
	f.Set("action", "save")
	f.Set("url_500px", "https://example.com/x")
	if rec := do("POST", "/admin/photos", f); rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "must start with https://500px.com/") {
		t.Errorf("bad link %d", rec.Code)
	}
	f.Del("url_500px")
	rec := do("POST", "/admin/photos", f)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("save %d %s", rec.Code, rec.Body.String())
	}
	all, _ := photo.NewRepo(s.DB, nil).All(ctx)
	if len(all) != 1 || all[0].Camera != "X100V" {
		t.Fatalf("saved %+v", all)
	}
	if !strings.Contains(do("GET", "/admin/photos", nil).Body.String(), `<span class="ent-title">Simit vendor</span>`) {
		t.Error("list")
	}
	if rec := do("POST", "/admin/photos/albums/"+albums[0].ID+"/delete", url.Values{}); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("album in use %d", rec.Code)
	}

	// Uploading from the album row adds drafts titled after the files, with
	// their EXIF; a PDF is refused and the rest still go in.
	upload := func(album string, publish bool, files map[string][]byte) *httptest.ResponseRecorder {
		var b bytes.Buffer
		mw := multipart.NewWriter(&b)
		for name, data := range files {
			fw, _ := mw.CreateFormFile("file", name)
			_, _ = fw.Write(data)
		}
		if publish {
			_ = mw.WriteField("publish", "1")
		}
		_ = mw.Close()
		req := httptest.NewRequest("POST", "/admin/photos/albums/"+album+"/upload", &b)
		req.Header.Set("Content-Type", mw.FormDataContentType())
		req.Header.Set("Sec-Fetch-Site", "same-origin")
		req.Header.Set("Accept-Language", "en")
		req.AddCookie(fresh)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec
	}
	if rec := upload(albums[0].ID, false, map[string][]byte{"ferry_wake.jpg": exifJPEG(t)}); rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/admin/photos?done=uploaded_draft" {
		t.Fatalf("upload %d %s", rec.Code, rec.Body.String())
	}
	if rec := upload(albums[0].ID, true, map[string][]byte{"balat-stairs.jpg": exifJPEG(t), "cv.pdf": []byte("%PDF-1.4\n%%EOF\n")}); rec.Code != http.StatusUnprocessableEntity ||
		!strings.Contains(rec.Body.String(), "cv.pdf: Must be a JPEG") {
		t.Errorf("pdf %d %s", rec.Code, rec.Body.String())
	}
	if rec := upload("nope", false, map[string][]byte{"x.jpg": exifJPEG(t)}); rec.Code != http.StatusNotFound {
		t.Errorf("unknown album %d", rec.Code)
	}
	got := map[string]photo.Photo{}
	all, _ = photo.NewRepo(s.DB, nil).All(ctx)
	for _, x := range all {
		got[x.TitleEN] = x
	}
	if x := got["ferry wake"]; x.Published || x.TitleTR != "ferry wake" || x.Camera != "X100V" || x.AlbumID != albums[0].ID {
		t.Errorf("draft upload %+v", x)
	}
	if x := got["balat stairs"]; !x.Published {
		t.Errorf("published upload %+v", x)
	}
	if len(all) != 3 {
		t.Errorf("%d photos", len(all))
	}
	if ms, _ := m.List(ctx, 50); len(ms) != 3 {
		t.Errorf("media %d: the pdf must not stay", len(ms))
	}
	if !strings.Contains(do("GET", "/admin/photos", nil).Body.String(), `action="/admin/photos/albums/`+albums[0].ID+`/upload" enctype="multipart/form-data"`) {
		t.Error("upload form")
	}
}
