// Command palette search at the edge (ADR-010): GET /x/search?q=…&lang=…
// queries the SEARCH database (FTS5) and the main database for titles,
// so search never waits on a container. The answer is JSON; palette.js
// renders it with labels from the page (the i18n catalog stays in Go).
// Parsing and folding mirror app/internal/search (Parse, Fold): the index
// is written folded by Go, so queries must fold the same way.

export interface SearchEnv {
	DB: D1Database;
	SEARCH: D1Database;
}

/** Palette prefix filters (design/README.md), as in search.kindPrefixes. */
const PREFIXES: Record<string, string> = {
	p: "post",
	n: "note",
	s: "snippet",
	l: "link",
	f: "fav",
	pr: "project",
	h: "homelab", // homelab builds: projects indexed under their own kind
	t: "presentation",
};

const LANGS = ["tr", "en"];

/** Item pages per kind; links open their URL, favs the favs page. */
const SECTIONS: Record<string, string> = {
	post: "posts",
	note: "notes",
	snippet: "snippets",
	project: "projects",
	homelab: "projects",
	presentation: "presentations",
};

/** search.Fold: ı, İ and I become i, everything else lowercases. */
export function fold(s: string): string {
	let out = "";
	for (const ch of s) {
		if (ch === "I" || ch === "İ" || ch === "ı") out += "i";
		else if (ch === "̇") continue; // combining dot above, left by a decomposed İ
		else out += ch.toLowerCase();
	}
	return out;
}

export interface Query {
	match: string; // FTS5 MATCH expression; empty means nothing to search
	kind: string; // from a prefix, empty for all kinds
}

/**
 * search.Parse: "pr: go htm" is kind project and `"go" "htm"*`. Tokens are
 * letters and digits only, quoted, all required, the last one a prefix.
 */
export function parse(raw: string): Query {
	let s = raw.trim();
	let kind = "";
	const i = s.indexOf(":");
	if (i > 0) {
		const k = PREFIXES[s.slice(0, i).toLowerCase()];
		if (k) {
			kind = k;
			s = s.slice(i + 1);
		}
	}
	const toks = fold(s)
		.split(/[^\p{L}\p{N}]+/u)
		.filter((t) => t !== "")
		.map((t) => `"${t}"`);
	if (toks.length === 0) return { match: "", kind };
	toks[toks.length - 1] += "*";
	return { match: toks.join(" "), kind };
}

export interface Row {
	href: string;
	title: string;
	kind: string;
	lang: string;
}

interface Hit {
	entry_id: string;
}

interface Entry {
	id: string;
	kind: string;
	lang: string;
	slug: string;
	title: string;
	summary: string;
	url: string | null;
}

export function entryHref(e: Pick<Entry, "kind" | "lang" | "slug" | "url">): string {
	const seg = SECTIONS[e.kind];
	if (seg) return `/${e.lang}/${seg}/${e.slug}`;
	if (e.kind === "link") return e.url ?? "";
	if (e.kind === "fav") return `/${e.lang}/favs#${e.slug}`;
	if (e.kind === "page") return `/${e.lang}/${e.slug}`; // About, Now
	return "";
}

/** Runs the search: published public entries, page language first, 10 rows. */
export async function search(env: SearchEnv, raw: string, lang: string): Promise<Row[]> {
	const q = parse([...raw].slice(0, 100).join(""));
	if (!q.match) return [];
	let sql = `SELECT entry_id, bm25(search_fts, 0, 0, 0, 0, 0, 0, 10.0, 4.0, 1.0, 6.0) AS rank
FROM search_fts WHERE search_fts MATCH ?`;
	const args: unknown[] = [q.match];
	// Projects and homelab builds share entries.kind; homelab rows indexed
	// before Phase 4.5 still say project, so both kinds are read and the
	// type from the main database decides below.
	const projects = q.kind === "project" || q.kind === "homelab";
	if (projects) {
		sql += " AND kind IN ('project', 'homelab')";
	} else if (q.kind) {
		sql += " AND kind = ?";
		args.push(q.kind);
	}
	sql += " AND status = 'published' AND visibility = 'public' ORDER BY rank LIMIT 20";
	const hits = (await env.SEARCH.prepare(sql).bind(...args).all<Hit>()).results;
	if (hits.length === 0) return [];
	// Titles from the main database: the index may lag a publish by a job.
	const ids = hits.map((h) => h.entry_id);
	const entries = (
		await env.DB.prepare(
			`SELECT e.id, CASE WHEN p.project_type = 'homelab' THEN 'homelab' ELSE e.kind END AS kind, e.lang, e.slug, e.title, e.summary, l.url
FROM entries e LEFT JOIN links l ON l.entry_id = e.id LEFT JOIN projects p ON p.entry_id = e.id
WHERE e.status = 'published' AND e.visibility = 'public' AND e.id IN (${ids.map(() => "?").join(", ")})`,
		)
			.bind(...ids)
			.all<Entry>()
	).results;
	const byID = new Map(entries.map((e) => [e.id, e]));
	const mine: Row[] = [];
	const other: Row[] = [];
	for (const id of ids) {
		const e = byID.get(id);
		if (!e || (projects && e.kind !== q.kind)) continue;
		const href = entryHref(e);
		if (!href) continue;
		const row = { href, title: e.title || e.summary || e.slug, kind: e.kind, lang: e.lang };
		(e.lang === lang ? mine : other).push(row);
	}
	return [...mine, ...other].slice(0, 10);
}

/** Answers /x/search, or returns null for other paths. */
export async function serveSearch(req: Request, url: URL, env: SearchEnv): Promise<Response | null> {
	if (url.pathname !== "/x/search") return null;
	if (req.method !== "GET" && req.method !== "HEAD") {
		return new Response("Method Not Allowed", { status: 405, headers: { Allow: "GET, HEAD" } });
	}
	const lang = LANGS.includes(url.searchParams.get("lang") ?? "") ? url.searchParams.get("lang")! : "en";
	let rows: Row[];
	try {
		rows = await search(env, url.searchParams.get("q") ?? "", lang);
	} catch {
		return Response.json({ error: "search failed" }, { status: 500, headers: { "Cache-Control": "no-store" } });
	}
	return Response.json(rows, {
		headers: { "Cache-Control": "private, max-age=30", "X-Robots-Tag": "noindex" },
	});
}
