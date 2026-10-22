package webmention

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"

	"golang.org/x/net/html"

	"halid.org/engine/internal/content"
	"halid.org/engine/internal/fetch"
	"halid.org/engine/internal/jobs"
	"halid.org/engine/internal/store"
)

// Poster fetches pages and posts forms with the SSRF guard (fetch.Client).
type Poster interface {
	GetPrefix(ctx context.Context, raw string, max int64) (*fetch.Response, error)
	PostForm(ctx context.Context, raw string, form url.Values, max int64) (*fetch.Response, error)
}

// MaxTargets bounds how many outside links one send notifies.
const MaxTargets = 50

// maxDiscover is how much of a target page is read to find its endpoint.
const maxDiscover = 512 << 10

// Sender sends webmentions for published entries (Phase 8.2).
type Sender struct {
	db     store.DB
	web    Poster
	origin string // PUBLIC_ORIGIN, no trailing slash
}

// NewSender wires the send job; register it with Register.
func NewSender(db store.DB, web Poster) *Sender {
	return &Sender{db: db, web: web, origin: strings.TrimRight(os.Getenv("PUBLIC_ORIGIN"), "/")}
}

// WithOrigin sets the site origin (tests).
func (s *Sender) WithOrigin(origin string) *Sender {
	s.origin = strings.TrimRight(origin, "/")
	return s
}

// Register adds the send handler to j.
func (s *Sender) Register(j *jobs.Jobs) {
	j.Handle(jobs.TypeWebmentionSend, s.send)
}

func (s *Sender) send(ctx context.Context, raw json.RawMessage) error {
	var m jobs.WebmentionSend
	if err := json.Unmarshal(raw, &m); err != nil || m.EntryID == "" {
		return jobs.Permanent{Err: fmt.Errorf("webmention.send: bad payload: %s", raw)}
	}
	return s.Send(ctx, m.EntryID)
}

// Result is what one target's endpoint answered.
type Result struct {
	Target   string `json:"target_url"`
	Endpoint string `json:"endpoint"`
	Status   int    `json:"status"`
	Error    string `json:"error"`
	retry    bool   // the target or its endpoint was unreachable or failed
}

type published struct {
	Kind   string  `json:"kind"`
	Lang   string  `json:"lang"`
	Slug   string  `json:"slug"`
	Rev    *string `json:"published_revision_id"`
	HTML   *string `json:"body_html"`
	Listed int     `json:"listed"`
	Parent *string `json:"parent"`
}

// Send notifies the targets of one entry. A public entry tells every
// outside page its published body links to, and the ones its last sent
// revision linked to and this one no longer does. An entry that is gone or
// no longer public tells all of its last targets. Targets that cannot be
// reached make the job fail with their list, so the queue retries it; the
// state is saved only once every target has answered.
func (s *Sender) Send(ctx context.Context, entryID string) error {
	if s.origin == "" {
		return jobs.Permanent{Err: errors.New("webmention.send: PUBLIC_ORIGIN is not set")}
	}
	res, err := s.db.Query(ctx, `SELECT e.kind, e.lang, e.slug, e.published_revision_id, r.body_html,
  e.status = 'published' AND e.visibility = 'public' AS listed,
  (SELECT pe.slug FROM buildlogs b JOIN entries pe ON pe.id = b.project_entry_id WHERE b.entry_id = e.id) AS parent
FROM entries e LEFT JOIN revisions r ON r.id = e.published_revision_id WHERE e.id = ?`, entryID)
	if err != nil {
		return err
	}
	ents, err := store.Scan[published](res)
	if err != nil {
		return err
	}
	res, err = s.db.Query(ctx, `SELECT source_url, revision_id FROM webmention_sources WHERE entry_id = ?`, entryID)
	if err != nil {
		return err
	}
	last, err := store.Scan[struct {
		Source string `json:"source_url"`
		Rev    string `json:"revision_id"`
	}](res)
	if err != nil {
		return err
	}
	res, err = s.db.Query(ctx, `SELECT target_url FROM webmention_sends WHERE entry_id = ?`, entryID)
	if err != nil {
		return err
	}
	old, err := store.Scan[struct {
		Target string `json:"target_url"`
	}](res)
	if err != nil {
		return err
	}

	source, rev, links := "", "", []string(nil)
	if len(ents) == 1 && ents[0].Listed == 1 && ents[0].Rev != nil {
		e := ents[0]
		ref := jobs.Ref{Kind: e.Kind, Lang: e.Lang, Slug: e.Slug}
		if e.Parent != nil {
			ref.Parent = *e.Parent
		}
		if p := jobs.ItemPath(ref); p != "" {
			source, rev = s.origin+p, *e.Rev
			if e.HTML != nil {
				links = Outbound(*e.HTML, source, s.origin)
			}
		}
	}
	if source == "" {
		// Gone or unlisted: the old targets will find the source missing.
		if len(last) == 0 {
			return nil
		}
		var targets []string
		for _, o := range old {
			targets = append(targets, o.Target)
		}
		results := s.notify(ctx, last[0].Source, targets)
		if err := failed(results); err != nil {
			return err
		}
		_, err := s.db.Batch(ctx,
			store.Stmt{SQL: `DELETE FROM webmention_sends WHERE entry_id = ?`, Args: []any{entryID}},
			store.Stmt{SQL: `DELETE FROM webmention_sources WHERE entry_id = ?`, Args: []any{entryID}})
		return err
	}
	if len(last) == 1 && last[0].Source == source && last[0].Rev == rev {
		return nil // already sent for this revision
	}

	// This revision's links, then the ones it dropped.
	targets := append([]string(nil), links...)
	keep := map[string]bool{}
	for _, l := range links {
		keep[l] = true
	}
	for _, o := range old {
		if !keep[o.Target] {
			targets = append(targets, o.Target)
		}
	}
	if len(targets) > MaxTargets {
		targets = targets[:MaxTargets]
	}
	results := s.notify(ctx, source, targets)
	if err := failed(results); err != nil {
		return err
	}
	now := content.Clock().Unix()
	stmts := []store.Stmt{{SQL: `DELETE FROM webmention_sends WHERE entry_id = ?`, Args: []any{entryID}}}
	for _, r := range results {
		if !keep[r.Target] {
			continue // a dropped link: told once, not kept
		}
		stmts = append(stmts, store.Stmt{SQL: `INSERT INTO webmention_sends (entry_id, target_url, endpoint, status, error, sent_at)
VALUES (?, ?, ?, ?, ?, ?)`, Args: []any{entryID, r.Target, r.Endpoint, r.Status, r.Error, now}})
	}
	stmts = append(stmts, store.Stmt{SQL: `INSERT INTO webmention_sources (entry_id, source_url, revision_id, sent_at) VALUES (?, ?, ?, ?)
ON CONFLICT (entry_id) DO UPDATE SET source_url = excluded.source_url, revision_id = excluded.revision_id, sent_at = excluded.sent_at`,
		Args: []any{entryID, source, rev, now}})
	_, err = s.db.Batch(ctx, stmts...)
	return err
}

// failed is the job error when any target should be tried again.
func failed(results []Result) error {
	var bad []string
	for _, r := range results {
		if r.retry {
			bad = append(bad, r.Target+": "+r.Error)
		}
	}
	if len(bad) == 0 {
		return nil
	}
	return fmt.Errorf("webmention.send: %d of %d failed: %s", len(bad), len(results), strings.Join(bad, "; "))
}

func (s *Sender) notify(ctx context.Context, source string, targets []string) []Result {
	out := make([]Result, 0, len(targets))
	for _, t := range targets {
		out = append(out, s.one(ctx, source, t))
	}
	return out
}

// one discovers the target's endpoint and posts source and target to it.
// A target without an endpoint is not an error.
func (s *Sender) one(ctx context.Context, source, target string) Result {
	r := Result{Target: target}
	page, err := s.web.GetPrefix(ctx, target, maxDiscover) // the endpoint is in the head
	switch {
	case errors.Is(err, fetch.ErrBlocked):
		r.Error = "not allowed"
		return r
	case err != nil:
		// A domain that no longer exists will not come back on a retry.
		var dns *net.DNSError
		r.Error, r.retry = err.Error(), !(errors.As(err, &dns) && dns.IsNotFound)
		return r
	case page.Status >= 400:
		r.Error = fmt.Sprintf("target %d", page.Status)
		r.retry = page.Status >= 500 || page.Status == http.StatusTooManyRequests
		return r
	}
	r.Endpoint = Discover(page)
	if r.Endpoint == "" {
		return r
	}
	ep, err := url.Parse(r.Endpoint)
	if err != nil || fetch.Check(ep) != nil {
		r.Error = "endpoint not allowed"
		return r
	}
	if strings.EqualFold(ep.Host, hostOf(s.origin)) {
		r.Error = "endpoint is this site"
		return r
	}
	ans, err := s.web.PostForm(ctx, r.Endpoint, url.Values{"source": {source}, "target": {target}}, 4<<10)
	switch {
	case errors.Is(err, fetch.ErrBlocked):
		r.Error = "endpoint not allowed"
	case err != nil:
		r.Error, r.retry = err.Error(), true
	default:
		r.Status = ans.Status
		if ans.Status >= 300 {
			r.Error = strings.TrimSpace(clean(string(ans.Body), 200))
			r.retry = ans.Status >= 500 || ans.Status == http.StatusTooManyRequests
		}
	}
	return r
}

func hostOf(origin string) string {
	u, err := url.Parse(origin)
	if err != nil {
		return ""
	}
	return u.Host
}

// Outbound are the outside pages an entry's HTML links to: absolute http(s)
// links off this site, without fragments, in order and once each.
func Outbound(body, source, origin string) []string {
	doc, err := html.Parse(strings.NewReader(body))
	if err != nil {
		return nil
	}
	base, _ := url.Parse(source)
	site := hostOf(origin)
	seen := map[string]bool{}
	var out []string
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "a" {
			for _, a := range n.Attr {
				if a.Key != "href" {
					continue
				}
				u, err := base.Parse(strings.TrimSpace(a.Val))
				if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || strings.EqualFold(u.Host, site) {
					continue
				}
				u.Fragment = ""
				if v := u.String(); len(v) <= 2048 && !seen[v] {
					seen[v] = true
					out = append(out, v)
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return out
}

// Discover finds a page's webmention endpoint (Webmention §3.1.2): the
// first Link header with rel webmention, else the first <link> or <a>
// with it in document order, resolved against the page URL. "" for none.
func Discover(page *fetch.Response) string {
	resolve := func(ref string) string {
		u, err := page.URL.Parse(strings.TrimSpace(ref))
		if err != nil {
			return ""
		}
		return u.String()
	}
	for _, h := range page.Header.Values("Link") {
		for _, l := range splitLinks(h) {
			if l.rels["webmention"] {
				return resolve(l.ref)
			}
		}
	}
	if !strings.Contains(strings.ToLower(page.ContentType), "html") {
		return ""
	}
	z := html.NewTokenizer(bytes.NewReader(page.Body))
	for {
		switch z.Next() {
		case html.ErrorToken:
			return ""
		case html.StartTagToken, html.SelfClosingTagToken:
			name, hasAttr := z.TagName()
			if (string(name) != "link" && string(name) != "a") || !hasAttr {
				continue
			}
			var rel, href string
			hasHref := false
			for more := true; more; {
				var k, v []byte
				k, v, more = z.TagAttr()
				switch string(k) {
				case "rel":
					rel = string(v)
				case "href":
					href, hasHref = string(v), true
				}
			}
			if hasHref && relHas(rel, "webmention") {
				return resolve(href) // href="" is the page itself
			}
		}
	}
}

type linkValue struct {
	ref  string
	rels map[string]bool
}

// splitLinks parses an HTTP Link header: <url>; rel="a b", <url2>; rel=c.
func splitLinks(h string) []linkValue {
	var out []linkValue
	for h = strings.TrimSpace(h); strings.HasPrefix(h, "<"); {
		end := strings.IndexByte(h, '>')
		if end < 0 {
			break
		}
		l := linkValue{ref: h[1:end], rels: map[string]bool{}}
		h = h[end+1:]
		// parameters up to the next comma outside quotes
		i, quoted := 0, false
		for ; i < len(h); i++ {
			if h[i] == '"' {
				quoted = !quoted
			} else if h[i] == ',' && !quoted {
				break
			}
		}
		for _, p := range strings.Split(h[:i], ";") {
			k, v, ok := strings.Cut(strings.TrimSpace(p), "=")
			if ok && strings.EqualFold(strings.TrimSpace(k), "rel") {
				for _, r := range strings.Fields(strings.Trim(strings.TrimSpace(v), `"`)) {
					l.rels[strings.ToLower(r)] = true
				}
			}
		}
		out = append(out, l)
		if i >= len(h) {
			break
		}
		h = strings.TrimSpace(h[i+1:])
	}
	return out
}

func relHas(rel, want string) bool {
	for _, r := range strings.Fields(strings.ToLower(rel)) {
		if r == want {
			return true
		}
	}
	return false
}
