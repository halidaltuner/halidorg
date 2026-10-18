import { describe, expect, it } from "vitest";
import { type DecksEnv, isDecksHost, serveDeck } from "../src/decks";

const ID = "01JABCDEFGHJKMNPQRSTVWXYZ0";

function env(objects: Record<string, string>): DecksEnv {
	return {
		PUBLIC_ORIGIN: "https://halid.org",
		DECKS_ORIGIN: "https://decks.halid.org",
		MEDIA: {
			get: async (k: string) => (k in objects ? { body: new TextEncoder().encode(objects[k]), httpEtag: `"${k}"` } : null),
		},
	} as unknown as DecksEnv;
}

const req = (path: string, init?: RequestInit) => new Request("https://decks.halid.org" + path, init);
const serve = (r: Request, e: DecksEnv) => serveDeck(r, new URL(r.url), e);

describe("decks origin", () => {
	const e = env({ [`decks/${ID}/index.html`]: "<!doctype html><title>deck</title>" });

	it("matches only the decks host", () => {
		expect(isDecksHost(new URL("https://decks.halid.org/x"), e)).toBe(true);
		expect(isDecksHost(new URL("https://halid.org/x"), e)).toBe(false);
		expect(isDecksHost(new URL("https://decks.halid.org/x"), { ...e, DECKS_ORIGIN: undefined })).toBe(false);
	});

	it("serves a versioned deck as immutable HTML only the site may frame", async () => {
		const res = await serve(req(`/${ID}/?v=abc123`), e);
		expect(res.status).toBe(200);
		expect(res.headers.get("Content-Type")).toBe("text/html; charset=utf-8");
		expect(res.headers.get("Cache-Control")).toContain("immutable");
		expect(res.headers.get("Content-Security-Policy")).toBe("frame-ancestors 'self' https://halid.org");
		expect(res.headers.get("Content-Disposition")).toBeNull();
		expect(await res.text()).toContain("<title>deck</title>");
	});

	it("serves the download under the slug's name", async () => {
		const res = await serve(req(`/${ID}/htmx-in-production.html`), e);
		expect(res.status).toBe(200);
		expect(res.headers.get("Content-Disposition")).toBe('attachment; filename="htmx-in-production.html"');
		expect(res.headers.get("Cache-Control")).toBe("public, max-age=300");
	});

	it("redirects the bare id and 404s anything else", async () => {
		const bare = await serve(req(`/${ID}?v=1`), e);
		expect(bare.status).toBe(301);
		expect(bare.headers.get("Location")).toBe(`https://decks.halid.org/${ID}/?v=1`);
		for (const p of ["/", "/en/posts/x", `/${ID}/../x.html`, `/${ID}/X.html`, `/${ID}/a/b.html`, "/01JABCDEFGHJKMNPQRSTVWXYZ1/", "/og/en/x.png"]) {
			expect((await serve(req(p), e)).status, p).toBe(404);
		}
	});

	it("answers 304 and refuses writes", async () => {
		expect((await serve(req(`/${ID}/`, { headers: { "If-None-Match": `"decks/${ID}/index.html"` } }), e)).status).toBe(304);
		expect((await serve(req(`/${ID}/`, { method: "POST" }), e)).status).toBe(405);
	});
});
