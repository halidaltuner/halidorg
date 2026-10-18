package devseed

import (
	"context"

	"halid.org/engine/internal/content"
	"halid.org/engine/internal/content/fav"
	"halid.org/engine/internal/content/link"
	"halid.org/engine/internal/content/note"
	"halid.org/engine/internal/content/snippet"
	"halid.org/engine/internal/jobs"
	"halid.org/engine/internal/store"
)

// Short-form samples (Phase 2): notes, snippets, links and favs, so their
// pages, filters, favicons and the awesome lists can be checked. Titles say
// "sample"; links point at real sites so the favicon job has something to
// fetch.

type sampleCategory struct {
	slug, en, tr, tone, parent string
}

var sampleCategories = []sampleCategory{
	{slug: "sample-architecture", en: "architecture", tr: "mimari", tone: "green"},
	{slug: "sample-architecture-books", en: "books", tr: "kitaplar", parent: "sample-architecture"},
	{slug: "sample-web", en: "web", tr: "web"},
}

var sampleNotes = []note.Draft{
	{Note: note.Note{Entry: content.Entry{Slug: "sample-til-loopvar", Lang: "en"}, TIL: true, SourceURL: "https://go.dev/blog/loopvar-preview"},
		Markdown: "Sample TIL: since Go 1.22 each loop iteration gets its own variable, so closures over `i` no longer share one.", Tags: []string{"sample", "go"}},
	{Note: note.Note{Entry: content.Entry{Slug: "sample-note-titled", Lang: "en", Title: "Sample: a note with a title"}},
		Markdown: "The title stays in body size on the card.\n\nA second paragraph with `inline code`.", Tags: []string{"sample"}},
	{Note: note.Note{Entry: content.Entry{Slug: "ornek-not", Lang: "tr"}},
		Markdown: "Örnek not: başlıksız, tek paragraf.", Tags: []string{"sample"}},
}

var sampleSnippets = []snippet.Draft{
	{Snippet: snippet.Snippet{Entry: content.Entry{Slug: "sample-graceful-shutdown", Lang: "en", Title: "Sample: go graceful shutdown"},
		Files: []snippet.File{{Filename: "main.go", Code: "ctx, stop := signal.NotifyContext(context.Background(),\n    syscall.SIGINT, syscall.SIGTERM)\ndefer stop()\n\ngo func() { _ = srv.ListenAndServe() }()\n<-ctx.Done()\n\n// give in-flight requests 10 seconds\nshutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)\ndefer cancel()\n_ = srv.Shutdown(shutdownCtx)\n"}}},
		Markdown: "Stop on SIGTERM, give requests 10 seconds, then exit.", Tags: []string{"sample", "go"}},
	{Snippet: snippet.Snippet{Entry: content.Entry{Slug: "sample-two-files", Lang: "en", Title: "Sample: a snippet with two files"},
		Files: []snippet.File{
			{Filename: "schema.sql", Code: "CREATE TABLE notes (\n  id   TEXT PRIMARY KEY,\n  body TEXT NOT NULL -- the note\n);\n"},
			{Filename: "prune.sh", Code: "#!/usr/bin/env sh\nset -eu\ndocker image prune -af --filter \"until=168h\"\n"},
		}},
		Markdown: "Tabs, the language badge and the raw link follow the file.", Tags: []string{"sample"}},
}

type sampleLink struct {
	draft    link.Draft
	category string
	read     bool
}

var sampleLinks = []sampleLink{
	{draft: link.Draft{Link: link.Link{Entry: content.Entry{Slug: "sample-link-grug", Lang: "en", Title: "Sample: The Grug Brained Developer", Summary: "Complexity is the enemy."},
		URL: "https://grugbrain.dev/"}}, category: "sample-architecture", read: true},
	{draft: link.Draft{Link: link.Link{Entry: content.Entry{Slug: "sample-link-boring", Lang: "en", Title: "Sample: Choose Boring Technology", Summary: "Innovation tokens are still the clearest framing."},
		URL: "https://boringtechnology.club/"}}, category: "sample-architecture"},
	{draft: link.Draft{Link: link.Link{Entry: content.Entry{Slug: "sample-link-hypermedia", Lang: "en", Title: "Sample: Hypermedia Systems", Summary: "The book behind the admin."},
		URL: "https://hypermedia.systems/"}}, category: "sample-architecture-books", read: true},
	{draft: link.Draft{Link: link.Link{Entry: content.Entry{Slug: "sample-link-htmx", Lang: "en", Title: "Sample: htmx documentation", Summary: "hx-swap and friends."},
		URL: "https://htmx.org/docs/"}}, category: "sample-web"},
	{draft: link.Draft{Link: link.Link{Entry: content.Entry{Slug: "sample-link-go", Lang: "en", Title: "Sample: The Go blog", Summary: "No category: shows under other."},
		URL: "https://go.dev/blog/"}}},
}

func rating(n int) *int { return &n }

var sampleFavs = []fav.Draft{
	{Fav: fav.Fav{Entry: content.Entry{Slug: "sample-fav-ddia", Lang: "en", Title: "Sample: Designing Data-Intensive Applications"}, Type: "book", Author: "Martin Kleppmann", Rating: rating(5)}},
	{Fav: fav.Fav{Entry: content.Entry{Slug: "sample-fav-helix", Lang: "en", Title: "Sample: Helix"}, Type: "tool", Author: "modal editor", URL: "https://helix-editor.com/", Rating: rating(4)}},
	{Fav: fav.Fav{Entry: content.Entry{Slug: "sample-fav-sqlc", Lang: "en", Title: "Sample: sqlc"}, Type: "repo", Author: "sqlc-dev", URL: "https://github.com/sqlc-dev/sqlc", Rating: rating(3)}},
	{Fav: fav.Fav{Entry: content.Entry{Slug: "sample-fav-place", Lang: "en", Title: "Sample: a place without a rating"}, Type: "place"}},
}

// existing returns the id of the entry of kind with lang and slug, or "".
func existing(ctx context.Context, db store.DB, kind, lang, slug string) (string, error) {
	res, err := db.Query(ctx, `SELECT id FROM entries WHERE kind = ? AND lang = ? AND slug = ?`, kind, lang, slug)
	if err != nil {
		return "", err
	}
	rows, err := store.Scan[struct {
		ID string `json:"id"`
	}](res)
	if err != nil || len(rows) == 0 {
		return "", err
	}
	return rows[0].ID, nil
}

// seedCategories creates the sample link categories that do not exist and
// returns their ids by slug.
func seedCategories(ctx context.Context, db store.DB) (map[string]string, error) {
	cats, err := content.Categories(ctx, db, link.Kind)
	if err != nil {
		return nil, err
	}
	ids := map[string]string{}
	for _, c := range cats {
		ids[c.Slug] = c.ID
	}
	for i, s := range sampleCategories {
		if ids[s.slug] != "" {
			continue
		}
		c := content.Category{Kind: link.Kind, Slug: s.slug, NameEN: s.en, NameTR: s.tr, Sort: 100 + i}
		if s.tone != "" {
			tone := s.tone
			c.Tone = &tone
		}
		if s.parent != "" {
			parent := ids[s.parent]
			c.ParentID = &parent
		}
		if err := content.CreateCategory(ctx, db, &c); err != nil {
			return nil, err
		}
		ids[s.slug] = c.ID
	}
	return ids, nil
}

// seedShort creates the short-form samples that do not exist yet and
// queues a render job for every one of them. It returns how many it created.
func seedShort(ctx context.Context, db store.DB, j *jobs.Jobs) (int, error) {
	n := 0
	// ensure runs create (and publish) unless kind/lang/slug exists, then
	// queues the render job either way.
	ensure := func(kind, lang, slug string, create func() (string, error)) error {
		id, err := existing(ctx, db, kind, lang, slug)
		if err != nil {
			return err
		}
		if id == "" {
			if id, err = create(); err != nil {
				return err
			}
			n++
		}
		if j == nil {
			return nil
		}
		return jobs.Enqueue(ctx, j, id, nil)
	}

	notes := note.NewRepo(db)
	for _, d := range sampleNotes {
		err := ensure(note.Kind, d.Lang, d.Slug, func() (string, error) {
			x, err := notes.Create(ctx, d)
			if err != nil {
				return "", err
			}
			_, err = notes.Publish(ctx, x.ID)
			return x.ID, err
		})
		if err != nil {
			return n, err
		}
	}
	snippets := snippet.NewRepo(db)
	for _, d := range sampleSnippets {
		err := ensure(snippet.Kind, d.Lang, d.Slug, func() (string, error) {
			x, err := snippets.Create(ctx, d)
			if err != nil {
				return "", err
			}
			_, err = snippets.Publish(ctx, x.ID)
			return x.ID, err
		})
		if err != nil {
			return n, err
		}
	}
	cats, err := seedCategories(ctx, db)
	if err != nil {
		return n, err
	}
	links := link.NewRepo(db)
	for _, s := range sampleLinks {
		d := s.draft
		if s.category != "" {
			id := cats[s.category]
			d.CategoryID = &id
		}
		err := ensure(link.Kind, d.Lang, d.Slug, func() (string, error) {
			x, err := links.Create(ctx, d)
			if err != nil {
				return "", err
			}
			if _, err := links.Publish(ctx, x.ID); err != nil {
				return "", err
			}
			return x.ID, links.SetRead(ctx, x.ID, s.read)
		})
		if err != nil {
			return n, err
		}
	}
	favs := fav.NewRepo(db)
	for _, d := range sampleFavs {
		err := ensure(fav.Kind, d.Lang, d.Slug, func() (string, error) {
			x, err := favs.Create(ctx, d)
			if err != nil {
				return "", err
			}
			_, err = favs.Publish(ctx, x.ID)
			return x.ID, err
		})
		if err != nil {
			return n, err
		}
	}
	return n, nil
}
