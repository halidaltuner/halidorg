// KV page cache rules. Pure functions so they can be tested without the runtime.

const LANGS = new Set(["tr", "en"]);

// Never served from or written to KV: admin and htmx fragments are per-user or
// short-lived, healthz must reach the container, bridge/spike are internal.
const BYPASS_PREFIXES = ["/admin", "/x", "/healthz", "/__bridge"];

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
	if (!ct.startsWith("text/html")) return false;
	const cc = (res.headers.get("Cache-Control") ?? "").toLowerCase();
	const directives = cc.split(",").map((d) => d.trim());
	return directives.includes("public") && !directives.includes("no-store") && !directives.includes("private");
}

export interface PageMeta {
	etag: string;
	contentType: string;
	storedAt: number;
}

/** Weak or strong If-None-Match match against a stored ETag. */
export function etagMatches(ifNoneMatch: string | null, etag: string): boolean {
	if (!ifNoneMatch) return false;
	if (ifNoneMatch.trim() === "*") return true;
	const strip = (t: string) => t.trim().replace(/^W\//, "");
	return ifNoneMatch.split(",").some((t) => strip(t) === strip(etag));
}
