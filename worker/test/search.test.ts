import { describe, expect, it } from "vitest";
import { entryHref, fold, parse, search, type SearchEnv, serveSearch } from "../src/search";

describe("parse", () => {
	// Same cases as app/internal/search TestParse: the index is folded by Go.
	const cases: [string, string, string][] = [
		["", "", ""],
		["   ", "", ""],
		["htmx", `"htmx"*`, ""],
		["go htm", `"go" "htm"*`, ""],
		["pr: radar", `"radar"*`, "project"],
		["h:rack", `"rack"*`, "project"],
		["S: fold", `"fold"*`, "snippet"],
		["n: loop", `"loop"*`, "note"],
		["l: grug", `"grug"*`, "link"],
		["f: kleppmann", `"kleppmann"*`, "fav"],
		["x: foo", `"x" "foo"*`, ""],
		[`ISTANBUL "OR" NEAR(a b)`, `"istanbul" "or" "near" "a" "b"*`, ""],
		["p:", "", "post"],
		["Işık*", `"işik"*`, ""],
	];
	for (const [input, match, kind] of cases) {
		it(JSON.stringify(input), () => expect(parse(input)).toEqual({ match, kind }));
	}
	it("folds Turkish i like Go", () => {
		expect(fold("İSTANBUL ılık Işık")).toBe("istanbul ilik işik");
		expect(fold("İ".normalize("NFD"))).toBe("i");
	});
});

describe("entryHref", () => {
	it("opens pages, link URLs and the favs page", () => {
		expect(entryHref({ kind: "note", lang: "en", slug: "x", url: null })).toBe("/en/notes/x");
		expect(entryHref({ kind: "link", lang: "en", slug: "x", url: "https://grugbrain.dev/" })).toBe("https://grugbrain.dev/");
		expect(entryHref({ kind: "fav", lang: "tr", slug: "ddia", url: null })).toBe("/tr/favs#ddia");
		expect(entryHref({ kind: "project", lang: "en", slug: "x", url: null })).toBe("/en/projects/x");
		expect(entryHref({ kind: "buildlog", lang: "en", slug: "x", url: null })).toBe("");
	});
});

// A D1 stand-in that records statements and answers by table.
function d1(answer: (sql: string, args: unknown[]) => unknown[], log: { sql: string; args: unknown[] }[]): D1Database {
	return {
		prepare(sql: string) {
			let args: unknown[] = [];
			const st = {
				bind(...a: unknown[]) {
					args = a;
					return st;
				},
				async all() {
					log.push({ sql, args });
					return { results: answer(sql, args) };
				},
			};
			return st;
		},
	} as unknown as D1Database;
}

describe("search", () => {
	const log: { sql: string; args: unknown[] }[] = [];
	const env: SearchEnv = {
		SEARCH: d1(() => [{ entry_id: "a" }, { entry_id: "b" }, { entry_id: "gone" }, { entry_id: "c" }], log),
		DB: d1(
			() => [
				{ id: "a", kind: "post", lang: "en", slug: "htmx-admin", title: "Notes on an htmx-first admin", summary: "", url: null },
				{ id: "b", kind: "note", lang: "tr", slug: "not", title: "", summary: "Başlıksız not", url: null },
				{ id: "c", kind: "link", lang: "tr", slug: "htmx", title: "htmx", summary: "", url: "https://htmx.org/" },
			],
			log,
		),
	};

	it("puts the page language first and drops unpublished hits", async () => {
		const rows = await search(env, "n: htm", "tr");
		expect(rows).toEqual([
			{ href: "/tr/notes/not", title: "Başlıksız not", kind: "note", lang: "tr" },
			{ href: "https://htmx.org/", title: "htmx", kind: "link", lang: "tr" },
			{ href: "/en/posts/htmx-admin", title: "Notes on an htmx-first admin", kind: "post", lang: "en" },
		]);
		const fts = log.find((l) => l.sql.includes("search_fts"))!;
		expect(fts.args).toEqual([`"htm"*`, "note"]);
		expect(fts.sql).toContain("status = 'published' AND visibility = 'public'");
	});

	it("answers JSON, privately, and only on /x/search", async () => {
		const res = (await serveSearch(new Request("https://halid.org/x/search?q=htmx&lang=en"), new URL("https://halid.org/x/search?q=htmx&lang=en"), env))!;
		expect(res.headers.get("Cache-Control")).toBe("private, max-age=30");
		expect((await res.json()) as unknown[]).toHaveLength(3);
		expect(await serveSearch(new Request("https://halid.org/en/"), new URL("https://halid.org/en/"), env)).toBeNull();
		const empty = (await serveSearch(new Request("https://halid.org/x/search?q=%22%22"), new URL("https://halid.org/x/search?q=%22%22"), env))!;
		expect(await empty.json()).toEqual([]);
	});
});
