package jobs

import (
	"context"
	"log/slog"
	"net/url"
	"regexp"
	"strings"

	"halid.org/engine/internal/content"
	"halid.org/engine/internal/store"
)

// Favicons finds a site's icon (fetch.Client.Favicon): bytes and the
// extension they sniffed as.
type Favicons interface {
	Favicon(ctx context.Context, host string) ([]byte, string, error)
}

// WithFavicons makes link changes fetch their site's favicon into R2.
func (e *EntryJobs) WithFavicons(f Favicons) *EntryJobs {
	e.icons = f
	return e
}

// FaviconPrefix is where favicons are served from R2 by the Worker
// (worker/src/r2public.ts), one object per domain.
const FaviconPrefix = "/favicons/"

var iconMIME = map[string]string{"png": "image/png", "jpg": "image/jpeg", "gif": "image/gif", "webp": "image/webp", "ico": "image/x-icon"}

// domainRE matches what r2public.ts accepts in a favicon key.
var domainRE = regexp.MustCompile(`^[a-z0-9-]+(\.[a-z0-9-]+)+$`)

// favicon gives a published link its site's icon: reused when another link
// of the domain already has one, else fetched once and stored for all of
// them. A site without a usable icon keeps none (the links page shows the
// domain's initial); that is not a job failure.
func (e *EntryJobs) favicon(ctx context.Context, row *entryRow) error {
	if e.icons == nil || row == nil || row.Kind != "link" || row.Status != content.Published {
		return nil
	}
	res, err := e.db.Query(ctx, `SELECT url, domain, favicon_url FROM links WHERE entry_id = ?`, row.ID)
	if err != nil {
		return err
	}
	rows, err := store.Scan[struct {
		URL     string  `json:"url"`
		Domain  string  `json:"domain"`
		Favicon *string `json:"favicon_url"`
	}](res)
	if err != nil || len(rows) == 0 {
		return err
	}
	l := rows[0]
	if !domainRE.MatchString(l.Domain) || (l.Favicon != nil && strings.HasPrefix(*l.Favicon, FaviconPrefix+l.Domain+".")) {
		return nil
	}
	res, err = e.db.Query(ctx, `SELECT favicon_url FROM links WHERE domain = ? AND favicon_url LIKE ? LIMIT 1`, l.Domain, FaviconPrefix+"%")
	if err != nil {
		return err
	}
	known, err := store.Scan[struct {
		Favicon string `json:"favicon_url"`
	}](res)
	if err != nil {
		return err
	}
	path := ""
	if len(known) == 1 {
		path = known[0].Favicon
	} else {
		u, err := url.Parse(l.URL)
		if err != nil {
			return nil
		}
		b, ext, err := e.icons.Favicon(ctx, u.Hostname())
		if err != nil || iconMIME[ext] == "" {
			slog.Info("favicon", "domain", l.Domain, "err", err)
			return nil
		}
		key := strings.TrimPrefix(FaviconPrefix, "/") + l.Domain + "." + ext
		if err := e.blob.Put(ctx, key, b, iconMIME[ext]); err != nil {
			return err
		}
		path = "/" + key
	}
	_, err = e.db.Query(ctx, `UPDATE links SET favicon_url = ? WHERE domain = ? AND (favicon_url IS NULL OR favicon_url NOT LIKE ?)`,
		path, l.Domain, FaviconPrefix+"%")
	return err
}
