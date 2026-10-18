// OG cards (render/og in the Go app) are rendered by the entry.changed job
// and written to R2 under og/. The Worker serves them straight from R2 so
// social crawlers never wait on the container (CLAUDE.md: public pages must
// not depend on a warm container).

export interface OGEnv {
	MEDIA: R2Bucket;
}

const KEY = /^og\/[a-z]{2}\/(?:[a-z0-9-]+\/)?[a-z0-9-]+\.png$/;

/** Returns the card for /og/... paths, or null when the path is not one. */
export async function serveOG(req: Request, url: URL, env: OGEnv): Promise<Response | null> {
	if (!url.pathname.startsWith("/og/")) return null;
	if (req.method !== "GET" && req.method !== "HEAD") {
		return new Response("Method Not Allowed", { status: 405, headers: { Allow: "GET, HEAD" } });
	}
	const key = url.pathname.slice(1);
	if (!KEY.test(key)) return notFound();
	const obj = await env.MEDIA.get(key);
	if (!obj) return notFound();
	// ?v= is the card's hash: a new card gets a new URL, so a versioned URL
	// never changes and can be cached for good.
	const cache = url.searchParams.has("v") ? "public, max-age=31536000, immutable" : "public, max-age=3600";
	const headers = new Headers({
		"Content-Type": "image/png",
		"Cache-Control": cache,
		ETag: obj.httpEtag,
		"X-Content-Type-Options": "nosniff",
	});
	if (req.headers.get("If-None-Match") === obj.httpEtag) return new Response(null, { status: 304, headers });
	return new Response(req.method === "HEAD" ? null : obj.body, { headers });
}

function notFound(): Response {
	return new Response("Not Found", { status: 404, headers: { "Cache-Control": "public, max-age=60" } });
}
