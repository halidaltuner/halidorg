// Command server is the halid.org Go app, run inside a Cloudflare Container.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"halid.org/engine/internal/admin"
	"halid.org/engine/internal/auth"
	"halid.org/engine/internal/capture"
	"halid.org/engine/internal/content/buildlog"
	"halid.org/engine/internal/content/certificate"
	"halid.org/engine/internal/content/fav"
	"halid.org/engine/internal/content/link"
	"halid.org/engine/internal/content/note"
	"halid.org/engine/internal/content/page"
	"halid.org/engine/internal/content/photo"
	"halid.org/engine/internal/content/post"
	"halid.org/engine/internal/content/presentation"
	"halid.org/engine/internal/content/project"
	"halid.org/engine/internal/content/series"
	"halid.org/engine/internal/content/shortadmin"
	"halid.org/engine/internal/content/snippet"
	"halid.org/engine/internal/devseed"
	"halid.org/engine/internal/feeds"
	"halid.org/engine/internal/fetch"
	"halid.org/engine/internal/github"
	"halid.org/engine/internal/jobs"
	"halid.org/engine/internal/media"
	"halid.org/engine/internal/public"
	"halid.org/engine/internal/settings"
	"halid.org/engine/internal/store"
	"halid.org/engine/internal/styleguide"
	"halid.org/engine/internal/taxonomy"
	"halid.org/engine/internal/ui"
	"halid.org/engine/web"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	if err := run(); err != nil {
		slog.Error("server exited", "err", err)
		os.Exit(1)
	}
}

func run() error {
	start := time.Now()
	mux, err := routes(store.FromEnv())
	if err != nil {
		return err
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           ui.DeployHeader(mux),
		ReadHeaderTimeout: 10 * time.Second,
	}

	// Containers are stopped with SIGTERM when they sleep; drain in-flight requests.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
	defer stop()
	errc := make(chan error, 1)
	go func() { errc <- srv.ListenAndServe() }()
	slog.Info("listening", "port", port, "startup_ms", time.Since(start).Milliseconds())

	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// routes builds the full mux. ServeMux panics on conflicting patterns, so
// main_test.go calls this to catch them before a deploy does.
func routes(stores store.Stores) (*http.ServeMux, error) {
	pub, err := public.New(stores.DB)
	if err != nil {
		return nil, err
	}
	authn := auth.New(auth.ConfigFromEnv(), stores.DB)
	runner := jobs.New(stores.DB, stores.Queue)
	adminUI := admin.NewUI(stores.DB)
	adm, err := admin.New(authn, adminUI, stores.DB, stores.Search, runner)
	if err != nil {
		return nil, err
	}

	sg, err := styleguide.New()
	if err != nil {
		return nil, err
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("ok\n"))
	})
	pub.Register(mux)
	feeds.New(stores.DB).Register(mux)
	adm.Register(mux)
	mediaRepo := media.NewRepo(stores.DB, stores.Blob)
	postAdmin, err := post.NewAdmin(stores.DB, adminUI, authn, runner, mediaRepo)
	if err != nil {
		return nil, err
	}
	postAdmin.Register(mux)
	link.NewAdmin(stores.DB, authn, runner).Register(mux)
	for _, editor := range []func(store.DB, *admin.UI, *auth.Auth, *jobs.Jobs) (*shortadmin.Admin, error){
		note.NewEditor, snippet.NewEditor, link.NewEditor, fav.NewEditor, project.NewEditor, buildlog.NewEditor, page.NewEditor,
	} {
		e, err := editor(stores.DB, adminUI, authn, runner)
		if err != nil {
			return nil, err
		}
		e.Register(mux)
	}
	quick, err := capture.New(stores.DB, adminUI, authn, runner, fetch.New(5*time.Second))
	if err != nil {
		return nil, err
	}
	quick.Register(mux)
	tax, err := taxonomy.New(stores.DB, adminUI, authn, runner)
	if err != nil {
		return nil, err
	}
	tax.Register(mux)
	setAdmin, err := settings.NewAdmin(stores.DB, adminUI, authn, runner)
	if err != nil {
		return nil, err
	}
	setAdmin.Register(mux)
	// GitHub sync (Phase 4.4): the Worker sends the token with each job
	// delivery; GITHUB_TOKEN is the fallback. Without one the job ends at
	// once and /admin/github says why.
	github.NewSync(stores.DB, os.Getenv("GITHUB_TOKEN"), runner).Register(runner)
	page.RegisterDaily(runner)
	ghAdmin, err := github.NewAdmin(stores.DB, adminUI, authn, runner)
	if err != nil {
		return nil, err
	}
	ghAdmin.Register(mux)
	certAdmin, err := certificate.NewAdmin(stores.DB, adminUI, authn, runner)
	if err != nil {
		return nil, err
	}
	certAdmin.Register(mux)
	photoAdmin, err := photo.NewAdmin(stores.DB, stores.Blob, adminUI, authn, runner)
	if err != nil {
		return nil, err
	}
	photoAdmin.Register(mux)
	presAdmin, err := presentation.NewEditor(stores.DB, stores.Blob, adminUI, authn, runner)
	if err != nil {
		return nil, err
	}
	presAdmin.Register(mux)
	presentation.NewDecks(stores.DB, stores.Blob, authn, runner).Register(mux)
	seriesAdmin, err := series.NewAdmin(stores.DB, adminUI, authn, runner)
	if err != nil {
		return nil, err
	}
	seriesAdmin.Register(mux)
	mediaAdmin, err := media.New(mediaRepo, adminUI, authn)
	if err != nil {
		return nil, err
	}
	mediaAdmin.Register(mux)
	sg.Register(mux)

	// Background jobs (Phase 1.3). The warmer renders through this same mux,
	// so a purge re-renders pages exactly as visitors would get them.
	jobs.NewEntryJobs(stores.DB, stores.Search, stores.Blob, jobs.NewWarmer(mux, stores.KV)).
		WithFavicons(fetch.New(5 * time.Second)).Register(runner)
	runner.Register(mux)

	seed := authn.RequireSession(devseed.New(stores.DB, runner))
	mux.Handle("GET /admin/dev/seed", seed)
	mux.Handle("POST /admin/dev/seed", admin.Guard(seed))
	// Two segments, not /static/: that pattern overlaps /{lang}/{$} and ServeMux panics.
	mux.Handle("GET /static/{dir}/{file}", staticHandler())
	return mux, nil
}

// staticHandler serves embedded CSS, JS and fonts. The Worker does not store
// non-HTML in KV, so these reach the container; file names are not hashed yet,
// hence an hour rather than immutable.
func staticHandler() http.Handler {
	files := http.FileServerFS(web.Static)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Templates link CSS and JS with a content hash (web.Asset): a URL
		// whose hash matches the file never changes content. Any other ?v= is
		// an older page asking for an older file (KV keeps a page from the
		// previous deploy until it is rendered again): it gets today's file,
		// cached briefly, never pinned for a year under the old hash.
		// Unversioned paths (fonts from CSS) stay at an hour.
		switch {
		case r.URL.Query().Has("v") && web.Asset(r.URL.Path) == r.URL.RequestURI():
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		case r.URL.Query().Has("v"):
			w.Header().Set("Cache-Control", "public, max-age=60")
		default:
			w.Header().Set("Cache-Control", "public, max-age=3600")
		}
		files.ServeHTTP(w, r)
	})
}
