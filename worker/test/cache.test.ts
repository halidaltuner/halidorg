import { describe, expect, it } from "vitest";
import { cacheKey, etagMatches, isCacheableRequest, isCacheableResponse, isStale, type PageMeta, renderedBy } from "../src/cache";

const req = (path: string, init?: RequestInit) => new Request(`https://halid.org${path}`, init);
const html = (headers: Record<string, string>, status = 200) =>
	new Response("<p>x</p>", { status, headers: { "Content-Type": "text/html; charset=utf-8", ...headers } });

describe("cacheKey", () => {
	it("keys by lang and path", () => {
		expect(cacheKey(new URL("https://halid.org/en/"))).toBe("page:en:/en/");
		expect(cacheKey(new URL("https://halid.org/tr/posts/x"))).toBe("page:tr:/tr/posts/x");
		expect(cacheKey(new URL("https://halid.org/"))).toBe("page:-:/");
		expect(cacheKey(new URL("https://halid.org/de/"))).toBe("page:-:/de/");
	});
});

describe("isCacheableRequest", () => {
	it("accepts public GET and HEAD", () => {
		expect(isCacheableRequest(req("/en/"))).toBe(true);
		expect(isCacheableRequest(req("/en/", { method: "HEAD" }))).toBe(true);
	});
	it("rejects writes, query strings and auth", () => {
		expect(isCacheableRequest(req("/en/", { method: "POST" }))).toBe(false);
		expect(isCacheableRequest(req("/en/?q=1"))).toBe(false);
		expect(isCacheableRequest(req("/en/", { headers: { Authorization: "Bearer x" } }))).toBe(false);
	});
	it("bypasses internal and dynamic prefixes", () => {
		for (const p of ["/admin", "/admin/login", "/x/search", "/healthz", "/lang/en", "/internal/jobs"]) {
			expect(isCacheableRequest(req(p)), p).toBe(false);
		}
	});
	it("does not treat lookalike paths as bypass", () => {
		expect(isCacheableRequest(req("/administrator"))).toBe(true);
		expect(isCacheableRequest(req("/xray"))).toBe(true);
	});
});

describe("isCacheableResponse", () => {
	it("needs 200 html with public opt-in", () => {
		expect(isCacheableResponse(html({ "Cache-Control": "public, max-age=0" }))).toBe(true);
		expect(isCacheableResponse(html({}))).toBe(false);
		expect(isCacheableResponse(html({ "Cache-Control": "private, no-store" }))).toBe(false);
		expect(isCacheableResponse(html({ "Cache-Control": "public" }, 404))).toBe(false);
		expect(isCacheableResponse(html({ "Cache-Control": "public", "Set-Cookie": "a=b" }))).toBe(false);
		expect(isCacheableResponse(new Response("{}", { headers: { "Content-Type": "application/json", "Cache-Control": "public" } }))).toBe(false);
	});
});

describe("etagMatches", () => {
	it("matches strong, weak, lists and wildcard", () => {
		expect(etagMatches('"abc"', '"abc"')).toBe(true);
		expect(etagMatches('W/"abc"', '"abc"')).toBe(true);
		expect(etagMatches('"x", "abc"', '"abc"')).toBe(true);
		expect(etagMatches("*", '"abc"')).toBe(true);
		expect(etagMatches('"x"', '"abc"')).toBe(false);
		expect(etagMatches(null, '"abc"')).toBe(false);
	});
});

describe("isStale", () => {
	const meta = (deploy?: string): PageMeta => ({ etag: '"x"', contentType: "text/html", storedAt: 1, deploy });
	it("is stale when another deploy rendered the page", () => {
		expect(isStale(meta("old"), "new")).toBe(true);
		expect(isStale(meta(undefined), "new")).toBe(true); // stored before deploy tags existed
	});
	it("is fresh for the current deploy and without a deploy (local dev)", () => {
		expect(isStale(meta("new"), "new")).toBe(false);
		expect(isStale(meta("old"), undefined)).toBe(false);
		expect(isStale(meta("old"), "")).toBe(false);
	});
});

describe("renderedBy", () => {
	const res = (sha?: string) => new Response("x", { headers: sha === undefined ? {} : { "X-Build-Sha": sha } });
	it("names the container's build when it sends one", () => {
		expect(renderedBy(res("ee45ac5c8bf76bcf861d06e8138cb3a2de485f59"), "bc5bbc1")).toBe("ee45ac5c8bf76bcf861d06e8138cb3a2de485f59");
	});
	it("falls back to the deploy for missing or bad values", () => {
		expect(renderedBy(res(), "bc5bbc1")).toBe("bc5bbc1");
		expect(renderedBy(res("not a sha"), "bc5bbc1")).toBe("bc5bbc1");
	});
	it("keeps an old container's page stale under the new deploy", () => {
		const meta = { etag: "", contentType: "text/html", storedAt: 0, deploy: renderedBy(res("ee45ac5"), "bc5bbc1") };
		expect(isStale(meta, "bc5bbc1")).toBe(true);
	});
});
