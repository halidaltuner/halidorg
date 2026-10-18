package page

import (
	"context"
	"errors"
	"testing"
	"time"

	"halid.org/engine/internal/content"
	"halid.org/engine/internal/store/storetest"
)

func TestPageRepo(t *testing.T) {
	ctx := context.Background()
	db := storetest.NewDB(t)
	r := NewRepo(db)
	now := Draft{Page: Page{Entry: content.Entry{Lang: "en", Title: "Now"}, Template: Now,
		Data: Data{Place: " istanbul ", Name: "dropped", Building: []Build{{Title: "admin", Pct: 65, Tone: "green"}, {}},
			Learning: []Learning{{Title: "Zig", Level: 3, Hours: 26}}, NotDoing: []string{" side projects ", ""}}}}
	p, err := r.Create(ctx, now)
	if err != nil {
		t.Fatal(err)
	}
	if p.Slug != "now" || p.Data.Place != "istanbul" || p.Data.Name != "" || len(p.Data.Building) != 1 || len(p.Data.NotDoing) != 1 {
		t.Fatalf("normalized %+v", p)
	}
	// One Now per language.
	if _, err := r.Create(ctx, now); err == nil {
		t.Error("second Now in en")
	}
	now.Lang = "tr"
	if _, err := r.Create(ctx, now); err != nil {
		t.Errorf("tr Now: %v", err)
	}
	bad := Draft{Page: Page{Entry: content.Entry{Lang: "en", Title: "About"}, Template: About,
		Data: Data{CTAs: []Link{{Label: "x", URL: "javascript:alert(1)"}}, Portrait: "missing"}}}
	var v content.ValidationError
	if _, err := r.Create(ctx, bad); !errors.As(err, &v) || v["ctas.0.url"] == "" {
		t.Errorf("bad about: %v", err)
	}

	// Two publishes are two versions; the public page reads the published one.
	clock := content.Clock
	t.Cleanup(func() { content.Clock = clock })
	content.Clock = func() time.Time { return time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC) }
	if p, err = r.Publish(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	d := Draft{Page: p, Markdown: ""}
	d.Data.Place = "ankara"
	content.Clock = func() time.Time { return time.Date(2026, 10, 13, 9, 0, 0, 0, time.UTC) }
	if _, err := r.Save(ctx, d, content.OriginManual); err != nil {
		t.Fatal(err)
	}
	if pub, err := r.GetPublished(ctx, "en", Now); err != nil || pub.Data.Place != "istanbul" {
		t.Log(err)
		t.Errorf("unpublished edit is live: %q", pub.Data.Place)
	}
	if _, err := r.Publish(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	vs, err := r.Versions(ctx, p.ID, 10)
	if err != nil || len(vs) != 2 || vs[0].PublishedAt <= vs[1].PublishedAt {
		t.Fatalf("versions %+v %v", vs, err)
	}
	old, err := r.VersionData(ctx, vs[1].RevisionID)
	if err != nil || old.Place != "istanbul" {
		t.Errorf("old version %+v %v", old, err)
	}
	if pub, _ := r.GetPublished(ctx, "en", Now); pub.Data.Place != "ankara" {
		t.Errorf("published %q", pub.Data.Place)
	}
}

func TestLinkURLAndLevels(t *testing.T) {
	for s, want := range map[string]bool{"https://x.dev": true, "mailto:me@halid.org": true, "/en/now": true,
		"//evil.dev": false, "http://x.dev": false, "javascript:x": false, "mailto:": false} {
		if LinkURL(s) != want {
			t.Errorf("%q: %v", s, !want)
		}
	}
	if LevelKey(3) != "now.level.beginner" || LevelKey(6) != "now.level.intermediate" || LevelKey(10) != "now.level.expert" {
		t.Error("levels")
	}
}
