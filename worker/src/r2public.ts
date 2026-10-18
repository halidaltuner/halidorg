// Public R2 objects served straight from the edge, never through the
// container (CLAUDE.md: public pages must not depend on a warm container):
//   /og/...    OG cards rendered by the entry.changed job (render/og)
//   /media/... images from the admin media library (internal/media)

export interface R2PublicEnv {
	MEDIA: R2Bucket;
}

const KEYS = [
	/^og\/[a-z]{2}\/(?:[a-z0-9-]+\/)?[a-z0-9-]+\.png$/,
	/^media\/[0-9A-Z]{26}\/[a-z0-9-]+\.(?:png|jpg|gif|webp)$/,
];

const TYPES: Record<string, string> = { png: "image/png", jpg: "image/jpeg", gif: "image/gif", webp: "image/webp" };

/** Returns the object for /og/ and /media/ paths, or null for other paths. */
export async function serveR2Public(req: Request, url: URL, env: R2PublicEnv): Promise<Response | null> {
	if (!url.pathname.startsWith("/og/") && !url.pathname.startsWith("/media/")) return null;
	if (req.method !== "GET" && req.method !== "HEAD") {
		return new Response("Method Not Allowed", { status: 405, headers: { Allow: "GET, HEAD" } });
	}
	const key = url.pathname.slice(1);
	if (!KEYS.some((k) => k.test(key))) return notFound();
	const obj = await env.MEDIA.get(key);
	if (!obj) return notFound();
	// Media keys carry a ULID and never change; OG cards are versioned by ?v=.
	const immutable = key.startsWith("media/") || url.searchParams.has("v");
	const ext = key.slice(key.lastIndexOf(".") + 1);
	const headers = new Headers({
		// The type comes from the extension the app chose after sniffing, not
		// from what an uploader claimed.
		"Content-Type": TYPES[ext] ?? "application/octet-stream",
		"Cache-Control": immutable ? "public, max-age=31536000, immutable" : "public, max-age=3600",
		ETag: obj.httpEtag,
		"X-Content-Type-Options": "nosniff",
		"Content-Security-Policy": "default-src 'none'; sandbox",
	});
	if (req.headers.get("If-None-Match") === obj.httpEtag) return new Response(null, { status: 304, headers });
	return new Response(req.method === "HEAD" ? null : obj.body, { headers });
}

function notFound(): Response {
	return new Response("Not Found", { status: 404, headers: { "Cache-Control": "public, max-age=60" } });
}
