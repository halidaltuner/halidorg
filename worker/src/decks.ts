// The decks origin (ADR-007): presentation decks are self-contained HTML in
// R2 (decks/{entry id}/index.html), served only on their own host, never on
// the main origin, so a deck's scripts cannot reach the site's cookies or
// pages. The main site embeds them in a sandboxed iframe.
//   /{id}/            the deck
//   /{id}/{slug}.html the same deck as a download named after the slug

export interface DecksEnv {
	MEDIA: R2Bucket;
	PUBLIC_ORIGIN: string;
	DECKS_ORIGIN?: string;
}

/** Whether the request is for the decks host (DECKS_ORIGIN). */
export function isDecksHost(url: URL, env: DecksEnv): boolean {
	if (!env.DECKS_ORIGIN) return false;
	try {
		return new URL(env.DECKS_ORIGIN).host === url.host;
	} catch {
		return false;
	}
}

const DECK = /^\/([0-9A-Z]{26})\/(?:([a-z0-9-]{1,80})\.html)?$/;

/** Serves a deck. Every path on the decks host ends here, found or not. */
export async function serveDeck(req: Request, url: URL, env: DecksEnv): Promise<Response> {
	if (req.method !== "GET" && req.method !== "HEAD") {
		return new Response("Method Not Allowed", { status: 405, headers: { Allow: "GET, HEAD" } });
	}
	const bare = /^\/([0-9A-Z]{26})$/.exec(url.pathname);
	if (bare) return Response.redirect(`${url.origin}/${bare[1]}/${url.search}`, 301);
	const m = DECK.exec(url.pathname);
	if (!m) return notFound();
	const obj = await env.MEDIA.get(`decks/${m[1]}/index.html`);
	if (!obj) return notFound();
	const headers = new Headers({
		"Content-Type": "text/html; charset=utf-8",
		// The iframe URL carries ?v= (the deck's hash): a new upload is a new URL.
		"Cache-Control": url.searchParams.has("v") ? "public, max-age=31536000, immutable" : "public, max-age=300",
		ETag: obj.httpEtag,
		"X-Content-Type-Options": "nosniff",
		"Referrer-Policy": "no-referrer",
		// Only the site may frame a deck; what the deck itself loads is its own
		// business on this isolated origin.
		"Content-Security-Policy": `frame-ancestors 'self' ${env.PUBLIC_ORIGIN}`,
	});
	if (m[2]) headers.set("Content-Disposition", `attachment; filename="${m[2]}.html"`);
	if (req.headers.get("If-None-Match") === obj.httpEtag) return new Response(null, { status: 304, headers });
	return new Response(req.method === "HEAD" ? null : obj.body, { headers });
}

function notFound(): Response {
	return new Response("Not Found", { status: 404, headers: { "Cache-Control": "public, max-age=60", "Content-Type": "text/plain; charset=utf-8" } });
}
