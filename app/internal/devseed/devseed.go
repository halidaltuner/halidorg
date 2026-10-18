// Package devseed fills local and staging with sample posts so pages can
// be checked before real content exists. It is a 404 in prod. Seeding is
// idempotent: posts whose slug already exists are not created again, only
// re-rendered, so a new renderer (pages, OG cards) can be checked on them.
package devseed

import (
	"context"
	"errors"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"

	"halid.org/engine/internal/content"
	"halid.org/engine/internal/content/post"
	"halid.org/engine/internal/jobs"
	"halid.org/engine/internal/store"
	"halid.org/engine/internal/ui"
)

// Handlers serves /admin/dev/seed. Wrap it in auth.RequireSession.
type Handlers struct {
	db   store.DB
	jobs *jobs.Jobs
	env  string
}

// New returns the seed handlers.
func New(db store.DB, j *jobs.Jobs) *Handlers { return &Handlers{db: db, jobs: j, env: ui.Env()} }

var form = template.Must(template.New("seed").Parse(`<!doctype html><meta charset="utf-8">
<title>seed</title><link rel="stylesheet" href="/static/css/tokens.css"><link rel="stylesheet" href="/static/css/app.css">
<main class="admin-stub"><h1>Seed sample posts</h1>
<p>Adds three sample posts in English and their Turkish translations ({{.Env}}). Existing slugs are skipped.</p>
{{with .Result}}<p class="mono">{{.}}</p>{{end}}
<form method="post"><button class="btn" type="submit">Seed</button></form>
<a href="/admin">← admin</a></main>`))

func (h *Handlers) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.env == "prod" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	result := ""
	if r.Method == http.MethodPost {
		n, err := Seed(r.Context(), h.db, h.jobs)
		if err != nil {
			slog.Error("seed", "err", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		result = fmt.Sprintf("created %d posts; render jobs queued", n)
	}
	_ = form.Execute(w, map[string]string{"Env": h.env, "Result": result})
}

type sample struct {
	slug, title, summary, md string
	tags                     []string
	tr                       *sample
}

// Sample content is clearly fake and exercises every renderer feature.
var samples = []sample{
	{
		slug: "sample-renderer-tour", title: "Sample: a tour of the renderer", tags: []string{"sample", "markdown"},
		summary: "Side notes, callouts, code blocks and a terminal, as the Markdown renderer outputs them.",
		md:      "Every block the renderer knows, on one page.\n\n## Side notes\n\nA footnote on a top-level paragraph becomes a side note[^1].\n\n[^1]: It wraps under the paragraph on narrow screens.\n\n## Code\n\n```go file=main.go\nfunc main() {\n\tfmt.Println(\"hello\")\n}\n```\n\n```html file=index.html tabs\n<p>tab one</p>\n```\n\n```go file=handler.go tabs\nfunc handler() {}\n```\n\n```terminal cwd=~/halid\n$ go test ./...\nok  \thalid.org/engine\t0.4s\n```\n\n> [!TIP]\n> Explicit saves always create a revision.\n\n> [!WARNING]\n> This is sample content.\n",
		tr: &sample{slug: "ornek-renderer-turu", title: "Örnek: renderer turu", tags: []string{"sample", "markdown"},
			summary: "Yan notlar, uyarı kutuları, kod blokları ve terminal; renderer'ın ürettiği haliyle.",
			md:      "Renderer'ın bildiği her blok tek sayfada.\n\n## Yan notlar\n\nÜst düzey bir paragraftaki dipnot yan nota dönüşür[^1].\n\n[^1]: Dar ekranda paragrafın altına iner.\n\n## Kod\n\n```go file=main.go\nfunc main() {}\n```\n\n> [!NOTE]\n> Bu örnek içeriktir.\n"},
	},
	{
		slug: "sample-long-read", title: "Sample: a longer read with a table of contents", tags: []string{"sample"},
		summary: "Several sections so the table of contents and scroll spy have something to follow.",
		md:      "Intro paragraph.\n\n## First section\n\nText.\n\n### A subsection\n\nMore text.\n\n## Second section\n\nText.\n\n## Third section\n\nText.\n",
	},
	{
		slug: "sample-short-note", title: "Sample: a short post", tags: []string{"sample", "meta"},
		summary: "One paragraph, no headings.",
		md:      "A short post with a [link to the home page](/en/).\n",
	},
}

// Seed creates the samples that do not exist yet, publishes them and
// queues render jobs for every sample. It returns how many posts it created.
func Seed(ctx context.Context, db store.DB, j *jobs.Jobs) (int, error) {
	repo := post.NewRepo(db)
	n := 0
	create := func(s sample, lang, group string) (string, error) {
		if p, err := repo.GetPublished(ctx, lang, s.slug); err == nil {
			if j != nil {
				if err := jobs.Enqueue(ctx, j, p.ID, nil); err != nil {
					return "", err
				}
			}
			return p.TranslationGroupID, nil
		} else if !errors.Is(err, content.ErrNotFound) {
			return "", err
		}
		p, err := repo.Create(ctx, post.Draft{
			Post:     post.Post{Entry: content.Entry{Slug: s.slug, Lang: lang, Title: s.title, Summary: s.summary, TranslationGroupID: group}},
			Markdown: s.md, Tags: s.tags,
		})
		if err != nil {
			return "", err
		}
		if _, err := repo.Publish(ctx, p.ID); err != nil {
			return "", err
		}
		n++
		if j != nil {
			if err := jobs.Enqueue(ctx, j, p.ID, nil); err != nil {
				return "", err
			}
		}
		return p.TranslationGroupID, nil
	}
	for _, s := range samples {
		group, err := create(s, "en", "")
		if err != nil {
			return n, err
		}
		if s.tr != nil && group != "" {
			if _, err := create(*s.tr, "tr", group); err != nil {
				return n, err
			}
		}
	}
	return n, nil
}
