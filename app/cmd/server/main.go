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
	"halid.org/engine/internal/feeds"
	"halid.org/engine/internal/jobs"
	"halid.org/engine/internal/public"
	"halid.org/engine/internal/store"
	"halid.org/engine/internal/styleguide"
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
	mux, err := routes()
	if err != nil {
		return err
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           mux,
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
func routes() (*http.ServeMux, error) {
	stores := store.FromEnv()
	pub, err := public.New()
	if err != nil {
		return nil, err
	}
	adm, err := admin.New(auth.New(auth.ConfigFromEnv(), stores.DB))
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
	sg.Register(mux)

	// Background jobs (Phase 1.3). The warmer renders through this same mux,
	// so a purge re-renders pages exactly as visitors would get them.
	runner := jobs.New(stores.DB, stores.Queue)
	jobs.NewEntryJobs(stores.DB, stores.Search, jobs.NewWarmer(mux, stores.KV)).Register(runner)
	runner.Register(mux)
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
		w.Header().Set("Cache-Control", "public, max-age=3600")
		files.ServeHTTP(w, r)
	})
}
