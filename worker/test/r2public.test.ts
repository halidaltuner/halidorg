import { describe, expect, it } from "vitest";
import { type R2PublicEnv, serveR2Public } from "../src/r2public";

function env(objects: Record<string, string>): R2PublicEnv {
	return {
		MEDIA: {
			get: async (k: string) =>
				k in objects ? { body: new TextEncoder().encode(objects[k]), httpEtag: `"${k}"` } : null,
		},
	} as unknown as R2PublicEnv;
}

const req = (path: string, init?: RequestInit) => new Request("https://halid.org" + path, init);
const serve = (r: Request, e: R2PublicEnv) => serveR2Public(r, new URL(r.url), e);

describe("serveR2Public", () => {
	const e = env({ "og/en/posts/htmx.png": "png", "og/tr/site-abc123.png": "site" });

	it("ignores other paths", async () => {
		expect(await serve(req("/en/posts/htmx"), e)).toBeNull();
	});

	it("serves versioned cards as immutable", async () => {
		const res = (await serve(req("/og/en/posts/htmx.png?v=abc"), e))!;
		expect(res.status).toBe(200);
		expect(res.headers.get("Content-Type")).toBe("image/png");
		expect(res.headers.get("Cache-Control")).toContain("immutable");
		expect(await res.text()).toBe("png");
	});

	it("caches unversioned cards briefly and serves site cards", async () => {
		const res = (await serve(req("/og/tr/site-abc123.png"), e))!;
		expect(res.status).toBe(200);
		expect(res.headers.get("Cache-Control")).toBe("public, max-age=3600");
	});

	it("answers 304 for a matching ETag", async () => {
		const res = (await serve(req("/og/en/posts/htmx.png", { headers: { "If-None-Match": '"og/en/posts/htmx.png"' } }), e))!;
		expect(res.status).toBe(304);
	});

	it("404s missing cards and keys outside og/", async () => {
		for (const p of ["/og/en/posts/nope.png", "/og/en/../x.png", "/og/en/posts/x.jpg", "/og/EN/posts/x.png"]) {
			expect((await serve(req(p), e))!.status, p).toBe(404);
		}
	});

	it("serves media by key with the type from the extension", async () => {
		const m = env({ "media/01JABCDEFGHJKMNPQRSTVWXYZ0/photo.webp": "img" });
		const res = (await serve(req("/media/01JABCDEFGHJKMNPQRSTVWXYZ0/photo.webp"), m))!;
		expect(res.status).toBe(200);
		expect(res.headers.get("Content-Type")).toBe("image/webp");
		expect(res.headers.get("Cache-Control")).toContain("immutable");
		expect(res.headers.get("Content-Security-Policy")).toContain("sandbox");
		for (const p of ["/media/x/photo.webp", "/media/01JABCDEFGHJKMNPQRSTVWXYZ0/photo.svg", "/media/01JABCDEFGHJKMNPQRSTVWXYZ0/a/b.png"]) {
			expect((await serve(req(p), m))!.status, p).toBe(404);
		}
	});

	it("rejects writes", async () => {
		expect((await serve(req("/og/en/posts/htmx.png", { method: "POST" }), e))!.status).toBe(405);
	});
});
