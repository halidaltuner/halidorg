import { describe, expect, it } from "vitest";
import { cacheKey, etagMatches, isCacheableRequest, isCacheableResponse } from "../src/cache";

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
		for (const p of ["/admin", "/admin/login", "/x/search", "/healthz", "/__bridge/kv/a"]) {
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
