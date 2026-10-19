package series_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"halid.org/engine/internal/admin"
	"halid.org/engine/internal/auth"
	"halid.org/engine/internal/content"
	"halid.org/engine/internal/content/post"
	"halid.org/engine/internal/content/series"
	"halid.org/engine/internal/jobs"
	"halid.org/engine/internal/store"
	"halid.org/engine/internal/store/storetest"
)

func TestSeriesAdmin(t *testing.T) {
	t.Setenv("APP_ENV", "staging")
	ctx := context.Background()
	s := storetest.Stores(t)
	a := auth.New(auth.Config{SessionSecret: strings.Repeat("k", 32)}, s.DB)
	h, err := series.NewAdmin(s.DB, admin.NewUI(s.DB), a, jobs.New(s.DB, s.Queue))
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
	if rec := do("POST", "/admin/series", url.Values{"title_en": {""}, "slug": {"Bad Slug"}}); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("bad series %d", rec.Code)
	}
	rec := do("POST", "/admin/series", url.Values{"title_en": {"Building kromatik"}, "title_tr": {"kromatik"}, "summary_en": {"Start to finish."}})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("create %d %s", rec.Code, rec.Body.String())
	}
	id := strings.TrimSuffix(strings.TrimPrefix(rec.Header().Get("Location"), "/admin/series/"), "?saved=1")
	got, err := series.NewRepo(s.DB).Get(ctx, id)
	if err != nil || got.Slug != "building-kromatik" || got.Status != "active" || got.Title("tr") != "kromatik" || got.Summary("tr") != "Start to finish." {
		t.Fatalf("saved %+v %v", got, err)
	}

	// A published part shows on the edit page; renaming re-renders the series
	// pages and the part.
	pr := post.NewRepo(s.DB)
	order := int64(1)
	p, err := pr.Create(ctx, post.Draft{Post: post.Post{Entry: content.Entry{Slug: "one", Lang: "en", Title: "Part one", SeriesID: &id, SeriesOrder: &order}}, Markdown: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pr.Publish(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	body := do("GET", "/admin/series/"+id, nil).Body.String()
	for _, want := range []string{"Part one", "#1", `href="/en/series/building-kromatik"`} {
		if !strings.Contains(body, want) {
			t.Errorf("edit page: missing %s", want)
		}
	}
	sent := len(s.Queue.(*storetest.Queue).Sent)
	if rec := do("POST", "/admin/series/"+id, url.Values{"title_en": {"Building kromatik"}, "slug": {"kromatik"}, "status": {"done"}}); rec.Code != http.StatusSeeOther {
		t.Fatalf("rename %d", rec.Code)
	}
	if len(s.Queue.(*storetest.Queue).Sent) == sent {
		t.Error("rename queued no pages job")
	}
	res, _ := s.DB.Query(ctx, `SELECT group_concat(payload, ' ') AS p FROM job_runs WHERE type = ?`, jobs.TypePagesChanged)
	all, _ := store.Scan[struct {
		P string `json:"p"`
	}](res)
	rows := all[0].P
	for _, want := range []string{"/en/series/kromatik", "/en/series/building-kromatik", "/tr/series", "/en/posts/one"} {
		if !strings.Contains(rows, want) {
			t.Errorf("pages job %s: no %s", rows, want)
		}
	}

	// Deleting keeps the part, out of the series.
	if rec := do("POST", "/admin/series/"+id+"/delete", url.Values{}); rec.Code != http.StatusSeeOther {
		t.Fatalf("delete %d", rec.Code)
	}
	if _, err := series.NewRepo(s.DB).Get(ctx, id); !errors.Is(err, content.ErrNotFound) {
		t.Errorf("series left: %v", err)
	}
	if left, err := pr.Get(ctx, p.ID); err != nil || left.SeriesID != nil || left.SeriesOrder != nil {
		t.Errorf("part after delete %+v %v", left, err)
	}
}
