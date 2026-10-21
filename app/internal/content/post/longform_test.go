package post

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"halid.org/engine/internal/content"
	"halid.org/engine/internal/store/storetest"
)

// Garden pages and runbooks share the post repo, each in its own kind.
func TestGardenAndRunbookKinds(t *testing.T) {
	tick(t)
	ctx := context.Background()
	r := NewRepo(storetest.NewDB(t))
	g := draft("tending", "en", "On tending notes", "Grows over time.")
	g.Kind = Garden
	gp, err := r.Create(ctx, g)
	if err != nil {
		t.Fatal(err)
	}
	if gp.Maturity != "seedling" {
		t.Errorf("default maturity %q", gp.Maturity)
	}
	rb := draft("zfs-scrub", "en", "Scrub a ZFS pool", "Run `zpool scrub`.")
	rb.Kind, rb.Platform, rb.Prerequisites, rb.Risk = Runbook, " TrueNAS 24 ", "- root shell\r\n\r\nA spare disk ", "high"
	day := int64(1789948800)
	rb.VerifiedOn = &day
	rp, err := r.Create(ctx, rb)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Create(ctx, draft("hello", "en", "A post", "Hi.")); err != nil {
		t.Fatal(err)
	}

	// Saving keeps the kind; bad stages and risks are refused.
	d := Draft{Post: gp, Markdown: "Grown."}
	d.Kind, d.Maturity = Kind, "budding"
	saved, _, err := r.Save(ctx, d, content.OriginManual)
	if err != nil || saved.Kind != Garden || saved.Maturity != "budding" {
		t.Fatalf("save garden %+v %v", saved, err)
	}
	var v content.ValidationError
	d.Maturity = "tree"
	if _, _, err := r.Save(ctx, d, content.OriginManual); !errors.As(err, &v) || v["maturity"] == "" {
		t.Errorf("bad maturity: %v", err)
	}
	x := Draft{Post: rp, Markdown: "Steps."}
	x.Risk = "extreme"
	if _, _, err := r.Save(ctx, x, content.OriginManual); !errors.As(err, &v) || v["risk"] == "" {
		t.Errorf("bad risk: %v", err)
	}

	// Publishing a garden page tends it.
	pub, err := r.Publish(ctx, gp.ID)
	if err != nil || pub.LastTendedAt == nil || *pub.LastTendedAt != *pub.PublishedAt {
		t.Fatalf("tended %+v %v", pub, err)
	}
	got, _ := r.Get(ctx, rp.ID)
	if got.Platform != "TrueNAS 24" || got.Risk != "high" || *got.VerifiedOn != day || strings.Join(got.Prereqs(), "|") != "root shell|A spare disk" {
		t.Errorf("runbook %+v %q", got, got.Prereqs())
	}
	if got.Path() != "/en/runbooks/zfs-scrub" || pub.Path() != "/en/garden/tending" {
		t.Errorf("paths %s %s", got.Path(), pub.Path())
	}

	// Lists, feeds and post lookups keep to their kind.
	for kind, want := range map[string]string{Kind: "hello", Garden: "tending", Runbook: "zfs-scrub"} {
		l, err := r.List(ctx, ListFilter{Kind: kind, Admin: true})
		if err != nil || len(l) != 1 || l[0].Slug != want {
			t.Errorf("%s list %+v %v", kind, l, err)
		}
	}
	if _, err := r.GetPublished(ctx, "en", "tending"); !errors.Is(err, content.ErrNotFound) {
		t.Errorf("a garden page found as a post: %v", err)
	}
	if p, err := r.GetPublishedKind(ctx, Garden, "en", "tending"); err != nil || p.Maturity != "budding" {
		t.Errorf("garden page %+v %v", p, err)
	}
	if f, _ := r.Feed(ctx, "en", "", 20, false); len(f) != 0 {
		t.Errorf("the posts feed has %d entries, want none published", len(f))
	}
}

func TestEditorForGarden(t *testing.T) {
	st := newAdminSite(t)
	rec := st.do(st.fresh, "POST", "/admin/posts", url.Values{"title": {"On tending"}, "lang": {"en"}, "kind": {"garden"}}, false)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("create %d %s", rec.Code, rec.Body.String())
	}
	id := strings.TrimPrefix(rec.Header().Get("Location"), "/admin/posts/")
	body := st.do(st.fresh, "GET", "/admin/posts/"+id, nil, false).Body.String()
	must(t, body, `<span class="on">garden</span>`, `name="maturity"`, `href="/admin/posts?kind=garden">← garden</a>`)
	if strings.Contains(body, `name="risk"`) {
		t.Error("runbook fields on a garden page")
	}
	f := editorForm("On tending", "on-tending", "Grows.")
	f.Set("maturity", "evergreen")
	if rec := st.do(st.fresh, "POST", "/admin/posts/"+id, f, false); rec.Code != http.StatusSeeOther {
		t.Fatalf("save %d %s", rec.Code, rec.Body.String())
	}
	must(t, st.do(st.fresh, "GET", "/admin/posts/"+id, nil, false).Body.String(), `<option value="evergreen" selected>`)
	if !strings.Contains(st.do(st.fresh, "GET", "/admin/posts?kind=garden", nil, false).Body.String(), "On tending") ||
		strings.Contains(st.do(st.fresh, "GET", "/admin/posts", nil, false).Body.String(), "On tending") {
		t.Error("the garden page is listed with posts, or not with the garden")
	}
	must(t, st.do(st.fresh, "GET", "/admin/posts/new?kind=runbook", nil, false).Body.String(), `name="kind" value="runbook"`, "New runbook")
}
