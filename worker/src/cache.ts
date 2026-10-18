// KV page cache rules. Pure functions so they can be tested without the runtime.

const LANGS = new Set(["tr", "en"]);

// Never served from or written to KV: admin and htmx fragments are per-user or
// short-lived, healthz must reach the container, /lang sets a cookie.
const BYPASS_PREFIXES = ["/admin", "/x", "/healthz", "/lang", "/internal"];

// Types the Go app may mark public: pages, feeds, sitemap, robots.txt.
// Mirrored by Cacheable in app/internal/jobs/warm.go.
const CACHEABLE_TYPES = [
	"text/html",
	"application/rss+xml",
	"application/atom+xml",
	"application/feed+json",
	"application/xml",
	"text/plain",
];

/** KV key for a public page: `page:{lang}:{path}`, lang `-` when the path has none. */
export function cacheKey(url: URL): string {
	const first = url.pathname.split("/")[1] ?? "";
	const lang = LANGS.has(first) ? first : "-";
	return `page:${lang}:${url.pathname}`;
}

export function isCacheableRequest(req: Request): boolean {
	if (req.method !== "GET" && req.method !== "HEAD") return false;
	const url = new URL(req.url);
	// The query string is not part of the key, so a page with one must not share it.
	if (url.search !== "") return false;
	// Cookies are not checked: the admin session cookie will be scoped to /admin,
	// and bypassing on any cookie would lose hits to analytics cookies.
	if (req.headers.has("Authorization")) return false;
	return !BYPASS_PREFIXES.some((p) => url.pathname === p || url.pathname.startsWith(p + "/"));
}

/** The Go app opts in with `Cache-Control: public`; anything else stays out of KV. */
export function isCacheableResponse(res: Response): boolean {
	if (res.status !== 200) return false;
	if (res.headers.has("Set-Cookie")) return false;
	const ct = res.headers.get("Content-Type") ?? "";
	if (!CACHEABLE_TYPES.some((t) => ct.startsWith(t))) return false;
	const cc = (res.headers.get("Cache-Control") ?? "").toLowerCase();
	const directives = cc.split(",").map((d) => d.trim());
	return directives.includes("public") && !directives.includes("no-store") && !directives.includes("private");
}

export interface PageMeta {
	etag: string;
	contentType: string;
	storedAt: number;
	/** Commit whose templates rendered the page. Pages from an older deploy are stale. */
	deploy?: string;
}

/**
 * A stored page is stale when it was rendered by another deploy than the one
 * serving now: template and CSS changes must reach pages whose content did
 * not change. Without a DEPLOY_SHA (local dev) nothing is stale.
 */
export function isStale(meta: PageMeta, deploy: string | undefined): boolean {
	return !!deploy && meta.deploy !== deploy;
}

/** Weak or strong If-None-Match match against a stored ETag. */
export function etagMatches(ifNoneMatch: string | null, etag: string): boolean {
	if (!ifNoneMatch) return false;
	if (ifNoneMatch.trim() === "*") return true;
	const strip = (t: string) => t.trim().replace(/^W\//, "");
	return ifNoneMatch.split(",").some((t) => strip(t) === strip(etag));
}
