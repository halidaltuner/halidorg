package public

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"

	"halid.org/engine/internal/i18n"
	"halid.org/engine/internal/render"
	"halid.org/engine/internal/store"
)

func decodeTOC(raw string) ([]render.Heading, error) {
	var out []render.Heading
	if raw == "" {
		return nil, nil
	}
	return out, json.Unmarshal([]byte(raw), &out)
}

type errorPage struct {
	page
	Code int
}

// fallback handles every GET no other route matched: a row in redirects
// wins (301/302, or 410 for a tombstone), otherwise the styled 404.
func (h *Handlers) fallback(w http.ResponseWriter, r *http.Request) {
	res, err := h.db.Query(r.Context(), `SELECT to_path, code FROM redirects WHERE from_path = ?`, r.URL.Path)
	if err != nil {
		slog.Error("redirect lookup", "path", r.URL.Path, "err", err)
	} else if rows, _ := store.Scan[struct {
		To   string `json:"to_path"`
		Code int    `json:"code"`
	}](res); len(rows) == 1 {
		if rows[0].Code == http.StatusGone {
			h.errorPage(w, r, http.StatusGone)
			return
		}
		w.Header().Set("Cache-Control", "public, max-age=3600")
		http.Redirect(w, r, localPath(rows[0].To, "/"), rows[0].Code)
		return
	}
	h.notFound(w, r)
}

func (h *Handlers) notFound(w http.ResponseWriter, r *http.Request) {
	h.errorPage(w, r, http.StatusNotFound)
}

// errorPage renders 404 and 410 in the language of the path, else the
// visitor's. Never cached at the edge: the Worker stores 200s only.
func (h *Handlers) errorPage(w http.ResponseWriter, r *http.Request, code int) {
	lang := i18n.FromRequest(r)
	if seg := firstSegment(r.URL.Path); i18n.IsSupported(seg) {
		lang = seg
	}
	key := "error.404"
	if code == http.StatusGone {
		key = "error.410"
	}
	p := errorPage{page: h.newPage(lang, "", i18n.T(lang, key+".title")+" · halid.org", "/", nil), Code: code}
	p.Head.Noindex = true
	p.Head.Canonical = ""
	p.Head.Alternates = nil
	var buf bytes.Buffer
	if err := h.tmpl["error"].ExecuteTemplate(&buf, "base", p); err != nil {
		slog.Error("render error page", "err", err)
		http.Error(w, http.StatusText(code), code)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_, _ = w.Write(buf.Bytes())
}

func firstSegment(path string) string {
	for i := 1; i < len(path); i++ {
		if path[i] == '/' {
			return path[1:i]
		}
	}
	if len(path) > 1 {
		return path[1:]
	}
	return ""
}
