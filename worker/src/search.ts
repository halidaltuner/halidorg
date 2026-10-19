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
	g: "garden",
	rb: "runbook",
	gl: "term",
	b: "book",
	rd: "radar_item",
	ph: "photo",
	c: "certificate",
};

/** search.Score: bm25 (lower is better) into a score with a recency boost. */
export function score(rank: number, publishedAt: number | null, now: number): number {
	let s = -rank;
	if (publishedAt && now > publishedAt) s *= 1 + 0.5 * Math.exp(-(now - publishedAt) / 86400 / 365);
	else if (publishedAt) s *= 1.5;
	return s;
}

const LANGS = ["tr", "en"];

/** Item pages per kind; links open their URL, favs the favs page. */
const SECTIONS: Record<string, string> = {
	post: "posts",
	note: "notes",
	snippet: "snippets",
	project: "projects",
	homelab: "projects",
	presentation: "presentations",
	garden: "garden",
	runbook: "runbooks",
	term: "glossary",
	radar_item: "radar",
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
	kind: string;
	lang: string;
	rank: number;
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
	if (e.kind === "book") return `/${e.lang}/reading#${e.slug}`;
	if (e.kind === "page") return `/${e.lang}/${e.slug}`; // About, Now
	return "";
}

/** Runs the search: published public entries, photos and certificates,
 * page language first, then by score (bm25 with a recency boost), 10 rows. */
export async function search(env: SearchEnv, raw: string, lang: string): Promise<Row[]> {
	const q = parse([...raw].slice(0, 100).join(""));
	if (!q.match) return [];
	let sql = `SELECT entry_id, kind, lang, bm25(search_fts, 0, 0, 0, 0, 0, 0, 10.0, 4.0, 1.0, 6.0) AS rank
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
	sql += " AND status = 'published' AND visibility = 'public' ORDER BY rank LIMIT 30";
	const hits = (await env.SEARCH.prepare(sql).bind(...args).all<Hit>()).results;
	if (hits.length === 0) return [];
	// Photos and certificates are indexed in both languages under one id:
	// the row in the page language ranks them.
	const rank = new Map<string, number>();
	for (const h of hits) {
		const key = h.kind === "photo" || h.kind === "certificate" ? `${h.entry_id}/${lang}` : h.entry_id;
		if (!rank.has(key) || h.lang === lang) rank.set(key, h.rank);
	}
	const of = (kind: string) => [...new Set(hits.filter((h) => (kind === "" ? h.kind !== "photo" && h.kind !== "certificate" : h.kind === kind)).map((h) => h.entry_id))];
	const in_ = (n: number) => `(${Array(n).fill("?").join(", ")})`;
	const found: (Row & { score: number })[] = [];
	const now = Math.floor(Date.now() / 1000);
	// Titles from the main database: the index may lag a publish by a job.
	const ids = of("");
	if (ids.length) {
		const entries = (
			await env.DB.prepare(
				`SELECT e.id, CASE WHEN p.project_type = 'homelab' THEN 'homelab' ELSE e.kind END AS kind, e.lang, e.slug, e.title, e.summary, l.url, e.published_at
FROM entries e LEFT JOIN links l ON l.entry_id = e.id LEFT JOIN projects p ON p.entry_id = e.id
WHERE e.status = 'published' AND e.visibility = 'public' AND e.id IN ${in_(ids.length)}`,
			)
				.bind(...ids)
				.all<Entry & { published_at: number | null }>()
		).results;
		for (const e of entries) {
			if (projects && e.kind !== q.kind) continue;
			const href = entryHref(e);
			if (!href) continue;
			found.push({ href, title: e.title || e.summary || e.slug, kind: e.kind, lang: e.lang, score: score(rank.get(e.id) ?? 0, e.published_at, now) });
		}
	}
	const photos = of("photo");
	if (photos.length) {
		const rows = (
			await env.DB.prepare(
				`SELECT id, CASE WHEN ? = 'tr' AND title_tr <> '' OR title_en = '' THEN title_tr ELSE title_en END AS title, coalesce(taken_at, created_at) AS at
FROM photos WHERE published = 1 AND id IN ${in_(photos.length)}`,
			)
				.bind(lang, ...photos)
				.all<{ id: string; title: string; at: number }>()
		).results;
		for (const p of rows) found.push({ href: `/${lang}/photos#p-${p.id}`, title: p.title, kind: "photo", lang, score: score(rank.get(`${p.id}/${lang}`) ?? 0, p.at, now) });
	}
	const certs = of("certificate");
	if (certs.length) {
		const rows = (
			await env.DB.prepare(`SELECT id, name, issued_at FROM certificates WHERE published = 1 AND id IN ${in_(certs.length)}`)
				.bind(...certs)
				.all<{ id: string; name: string; issued_at: number }>()
		).results;
		for (const c of rows) found.push({ href: `/${lang}/certificates#c-${c.id}`, title: c.name, kind: "certificate", lang, score: score(rank.get(`${c.id}/${lang}`) ?? 0, c.issued_at, now) });
	}
	found.sort((a, b) => (a.lang === lang) !== (b.lang === lang) ? (a.lang === lang ? -1 : 1) : b.score - a.score);
	return found.slice(0, 10).map(({ href, title, kind, lang }) => ({ href, title, kind, lang }));
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
