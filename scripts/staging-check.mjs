// Exit checks against staging: Phase 1 (prompts/phase-1.md, "Phase 1 exit
// check"), Phase 4 (projects, homelab, GitHub, stream) and Phase 5 (about,
// now, certificates, photos, presentations, the derived sections). Read-only: it
// never signs in to the admin and never writes.
// Run by .github/workflows/staging-check.yml with the Access service token.
//
//   STAGING_ORIGIN=https://staging.halid.org CF_ACCESS_CLIENT_ID=… CF_ACCESS_CLIENT_SECRET=… \
//     node scripts/staging-check.mjs out/
import { chromium } from "playwright";
import fs from "node:fs";

const origin = (process.env.STAGING_ORIGIN || "https://staging.halid.org").replace(/\/$/, "");
const out = process.argv[2] || "out";
fs.mkdirSync(out, { recursive: true });
const access = {
	"CF-Access-Client-Id": process.env.CF_ACCESS_CLIENT_ID || "",
	"CF-Access-Client-Secret": process.env.CF_ACCESS_CLIENT_SECRET || "",
};
if (!access["CF-Access-Client-Id"] || !access["CF-Access-Client-Secret"]) {
	console.error("CF_ACCESS_CLIENT_ID and CF_ACCESS_CLIENT_SECRET are required");
	process.exit(2);
}

const results = [];
function check(name, ok, detail = "") {
	results.push({ name, ok, detail });
	console.log(`${ok ? "PASS" : "FAIL"}  ${name}${detail ? "  · " + detail : ""}`);
	// Annotations are readable through the checks API, where logs are not.
	if (process.env.GITHUB_ACTIONS) console.log(`::${ok ? "notice" : "error"} title=${ok ? "PASS" : "FAIL"}::${name}${detail ? " · " + String(detail).replace(/\n/g, " ") : ""}`);
}

async function get(path, extra = {}) {
	const res = await fetch(origin + path, { headers: { ...access, ...extra }, redirect: "manual" });
	const body = res.headers.get("content-type")?.startsWith("image/") ? "" : await res.text();
	return { res, body };
}

// The first request may be a MISS (a cold key), STALE (stored by an older
// deploy) or already a HIT (warmed by a publish job); the second must be a HIT.
async function kvHit(path) {
	const first = await get(path);
	// A STALE page re-renders in the background (waitUntil): give it a
	// moment before asking again.
	if (first.res.headers.get("x-cache") === "STALE") await new Promise((r) => setTimeout(r, 4000));
	let second = await get(path);
	// A key written just now (first visit) can take a few seconds to be
	// readable at the edge: one more try before calling it a miss.
	if (first.res.headers.get("x-cache") === "MISS" && second.res.headers.get("x-cache") === "MISS") {
		await new Promise((r) => setTimeout(r, 10000));
		second = await get(path);
	}
	const a = first.res.headers.get("x-cache"), b = second.res.headers.get("x-cache");
	let why = "";
	if (b === "STALE") {
		// Still stale: what does the container answer for it now? A query
		// string bypasses KV and goes straight to the origin.
		const o = await get(path + (path.includes("?") ? "&" : "?") + "_origin=1");
		why = ` · origin ${o.res.status} build ${o.res.headers.get("x-build-sha") ?? "-"} cc ${o.res.headers.get("cache-control") ?? "-"} ct ${o.res.headers.get("content-type") ?? "-"}`;
	}
	check(`KV: ${path} second request is a HIT`, second.res.status === 200 && b === "HIT", `${first.res.status} ${a} → ${second.res.status} ${b}${why}`);
	// A page stored before the latest deploy still has the old templates:
	// every public page now loads the palette (Phase 1 PR #5).
	if (second.res.status === 200) check(`Fresh: ${path} has the current templates (palette.js)`, second.body.includes("/static/js/palette.js"),
		`deploy in footer: ${second.body.match(/deploy <span[^>]*>([0-9a-f]+)/)?.[1] ?? second.body.match(/deploy ([0-9a-f]{7})/)?.[1] ?? "?"}`);
	return second;
}

// 1. Access lets the token through (not the login page).
const health = await get("/healthz");
check("Access: service token reaches the app", health.res.status === 200 && health.body.trim() === "ok", `status ${health.res.status}`);

// 2. Public pages from KV.
const home = await kvHit("/en/");
await kvHit("/tr/");
const list = await kvHit("/en/posts");
const slugs = [...list.body.matchAll(/href="\/en\/posts\/([a-z0-9-]+)"/g)].map((m) => m[1]);
check("Posts: the list has published posts", slugs.length > 0, slugs.slice(0, 5).join(", "));
const slug = slugs[0];
let post = null;
if (slug) {
	post = await kvHit(`/en/posts/${slug}`);
	const ogm = post.body.match(/<meta property="og:image" content="([^"]+)"/);
	check("Post: og:image meta present", !!ogm, ogm?.[1] ?? "");
	if (ogm) {
		const u = new URL(ogm[1]);
		const img = await get(u.pathname + u.search);
		check("OG: card served from R2 by the Worker", img.res.status === 200 && img.res.headers.get("content-type") === "image/png",
			`${img.res.status} ${img.res.headers.get("content-type")} ${img.res.headers.get("cache-control")}`);
	}
	check("Post: canonical and hreflang", /rel="canonical"/.test(post.body) && /hreflang="tr"/.test(post.body));
	const fb = await get(`/tr/posts/${slug}`);
	check("Fallback: /tr/posts/{en-slug} is the banner page or a 301 to the translation",
		(fb.res.status === 200 && /noindex/.test(fb.body)) || fb.res.status === 301, `status ${fb.res.status}`);
}

// 3. Search (palette endpoint): published posts are indexed.
if (slug) {
	const title = (post.body.match(/<title>([^<·]+)/)?.[1] ?? slug).trim();
	const word = title.split(/\s+/).find((w) => w.replace(/[^\p{L}\p{N}]/gu, "").length >= 4)?.replace(/[^\p{L}\p{N}]/gu, "") ?? slug.split("-")[0];
	const s = await get(`/x/search?lang=en&q=${encodeURIComponent(word)}`);
	check(`Search: "${word}" finds /en/posts/${slug}`, s.res.status === 200 && s.body.includes(`/en/posts/${slug}"`), `${(s.body.match(/pal-row/g) || []).length} rows`);
}

// 4. Feeds, sitemap, robots, 404.
for (const [path, type] of [["/en/feed.xml", "application/rss+xml"], ["/en/feed.json", "application/feed+json"], ["/sitemap.xml", "application/xml"], ["/robots.txt", "text/plain"]]) {
	const r = await get(path);
	check(`${path} ${r.res.status} ${type}`, r.res.status === 200 && (r.res.headers.get("content-type") || "").startsWith(type), r.res.headers.get("content-type") || "");
}
const robots = await get("/robots.txt");
check("robots.txt disallows everything on staging", /Disallow: \/\s*$/m.test(robots.body));
const nf = await get("/en/posts/does-not-exist-" + Date.now());
check("404: styled page, not cached", nf.res.status === 404 && /noindex/.test(nf.body) && nf.res.headers.get("x-cache") !== "HIT", `${nf.res.status} ${nf.res.headers.get("x-cache")}`);
const adm = await get("/admin");
check("Admin: needs a session (redirects to login)", adm.res.status === 303 && (adm.res.headers.get("location") || "").startsWith("/admin/login"), `${adm.res.status} ${adm.res.headers.get("location")}`);

// 5. Phase 4: projects, homelab, build logs, GitHub, stream.
const projects = await kvHit("/en/projects");
await kvHit("/en/homelab");
await kvHit("/en/stream");
const ghPage = await kvHit("/en/github");
const pslugs = [...projects.body.matchAll(/class="pj-card" href="\/en\/projects\/([a-z0-9-]+)"/g)].map((m) => m[1]);
check("Projects: the list has software projects", pslugs.length > 0, pslugs.slice(0, 5).join(", "));
const hl = await get("/en/homelab");
const hslugs = [...new Set([...hl.body.matchAll(/href="\/en\/projects\/([a-z0-9-]+)"/g)].map((m) => m[1]))];
check("Homelab: the page lists builds", hslugs.length > 0, hslugs.slice(0, 5).join(", "));
const shots4 = [["projects", "/en/projects"], ["homelab", "/en/homelab"], ["stream", "/en/stream"], ["github", "/en/github"], ["projects-tr", "/tr/projects"]];
let logPath = "";
for (const [label, ps] of [["software", pslugs[0]], ["homelab", hslugs[0]]]) {
	if (!ps) continue;
	const d = await kvHit(`/en/projects/${ps}`);
	shots4.push([`project-${label}`, `/en/projects/${ps}`]);
	const ogm = d.body.match(/<meta property="og:image" content="([^"]+)"/);
	if (ogm) {
		const u = new URL(ogm[1]);
		const img = await get(u.pathname + u.search);
		check(`Project (${label}): its OG card is served`, img.res.status === 200 && img.res.headers.get("content-type") === "image/png" && u.pathname.includes("/projects/"),
			`${u.pathname} ${img.res.status}`);
	} else check(`Project (${label}): og:image meta present`, false);
	const lm = d.body.match(/href="(\/en\/projects\/[a-z0-9-]+\/log\/[a-z0-9-]+)"/);
	if (lm && !logPath) logPath = lm[1];
}
if (logPath) {
	const lg = await kvHit(logPath);
	check("Build log: page under its project with a back link", lg.res.status === 200 && /class="mono back"/.test(lg.body), logPath);
	shots4.push(["buildlog", logPath]);
} else check("Build log: a published build log is linked from a project page", false, "none found");
const pf = await get("/en/projects/feed.xml");
check("/en/projects/feed.xml 200 application/rss+xml", pf.res.status === 200 && (pf.res.headers.get("content-type") || "").startsWith("application/rss+xml"));
const sm = await get("/sitemap.xml");
for (const path of ["/en/projects", "/en/homelab", "/en/stream", "/en/github"]) check(`Sitemap lists ${path}`, sm.body.includes(`${path}</loc>`));
if (pslugs[0]) check("Sitemap lists a project page", sm.body.includes(`/en/projects/${pslugs[0]}</loc>`));
// GitHub: synced data on /github and the projects section.
check("GitHub: /en/github shows synced data", /<section class="gh">/.test(ghPage.body), /henüz|not been synced/.test(ghPage.body) ? "no sync yet: set GITHUB_TOKEN and run the sync" : "");
check("GitHub: the projects page has the GitHub section", /<section class="gh">/.test(projects.body));
check("GitHub: the activity list has rows", /class="gh-activity"/.test(ghPage.body));
const st = await get("/en/stream");
check("Stream: project or homelab rows", /glyph k-(project|homelab)/.test(st.body), `${(st.body.match(/class="stream-row"/g) || []).length} rows`);
check("Home: stream links to /en/stream", home.body.includes('href="/en/stream"'));
const hk = await get("/en/stream?kind=homelab");
check("Stream: ?kind=homelab shows only homelab rows", hk.res.status === 200 && !/glyph k-(post|note|snippet|link|fav|project) /.test(hk.body));
// Palette prefixes tell projects and homelab builds apart.
const firstWord = (html) => (html.match(/<h1>([^<]+)<\/h1>/)?.[1] ?? "").split(/\s+/).map((w) => w.replace(/[^\p{L}\p{N}]/gu, "")).find((w) => w.length >= 3) ?? "";
for (const [prefix, ps, want] of [["pr", pslugs[0], "project"], ["h", hslugs[0], "homelab"]]) {
	if (!ps) continue;
	const d = await get(`/en/projects/${ps}`);
	const word = firstWord(d.body) || ps.split("-")[0];
	const r = await get(`/x/search?lang=en&q=${encodeURIComponent(prefix + ": " + word)}`);
	let rows = [];
	try { rows = JSON.parse(r.body); } catch {}
	check(`Palette: "${prefix}: ${word}" finds ${want} rows only`, rows.length > 0 && rows.every((x) => x.kind === want), rows.map((x) => `${x.kind} ${x.href}`).join(", ") || r.body.slice(0, 80));
}
const ga = await get("/admin/github");
check("Admin: /admin/github needs a session", ga.res.status === 303, `${ga.res.status}`);

// 6. Phase 5: personal pages, certificates, photos, presentations, the
// derived sections. Designed screens need content; derived ones are
// checked when something is published.
const shots5 = [];
for (const [name, path, marker] of [["about", "/en/about", 'class="ab-'], ["now", "/en/now", 'class="nw-page'], ["now-archive", "/en/now/archive", "<main"],
	["certificates", "/en/certificates", 'class="ce-card'], ["photos", "/en/photos", 'class="ph-grid'], ["presentations", "/en/presentations", 'class="pr-card']]) {
	const pg = await kvHit(path);
	check(`${path}: has content`, pg.res.status === 200 && pg.body.includes(marker), pg.res.status === 200 ? "" : `status ${pg.res.status}`);
	shots5.push([name, path]);
}
await kvHit("/tr/about");
const pres = await get("/en/presentations");
const prSlug = pres.body.match(/class="pr-card" href="\/en\/presentations\/([a-z0-9-]+)"/)?.[1];
let deckURL = "";
if (prSlug) {
	const d = await kvHit(`/en/presentations/${prSlug}`);
	shots5.push(["presentation", `/en/presentations/${prSlug}`]);
	deckURL = d.body.match(/<iframe src="([^"]+)"/)?.[1]?.replace(/&amp;/g, "&") ?? "";
	if (deckURL) {
		const u = new URL(deckURL);
		check("Deck: iframe on the decks origin, not the main one", u.origin !== new URL(origin).origin && /sandbox="allow-scripts/.test(d.body), u.origin);
		// The decks host stays outside Access (a third-party cookie would not reach the iframe).
		const dr = await fetch(deckURL, { redirect: "manual" });
		const csp = dr.headers.get("content-security-policy") || "";
		check("Deck: served without Access, framed only by the site", dr.status === 200 && /frame-ancestors/.test(csp), `${dr.status} ${csp}`);
		const main = await get(u.pathname);
		check("Deck: not served from the main origin", main.res.status === 404, `${main.res.status}`);
	} else check("Deck: a presentation with an uploaded deck", false, `${prSlug} has no deck`);
}
const ab = await get("/en/about");
check("About: certificates card links /en/certificates", ab.body.includes('href="/en/certificates"'));
for (const [path, kind] of [["/en/garden", "garden"], ["/en/runbooks", "runbooks"], ["/en/series", "series"], ["/en/glossary", "glossary"], ["/en/reading", "reading"]]) {
	const pg = await kvHit(path);
	shots5.push([kind, path]);
	const m = pg.body.match(new RegExp(`href="(/en/${kind}/[a-z0-9-]+)"`));
	if (!m) {
		console.log(`info  ${path}: nothing published yet`);
		continue;
	}
	const d = await kvHit(m[1]);
	shots5.push([`${kind}-item`, m[1]]);
	if (kind !== "series") check(`${m[1]}: connections block or none to show`, d.res.status === 200, /id="(backlinks|related)"/.test(d.body) ? "has connections" : "");
}
if (slug) {
	const p = await get(`/en/posts/${slug}`);
	check("Post: backlinks block (and related, when tags are shared)", /id="backlinks"/.test(p.body), /id="related"/.test(p.body) ? "related shown" : "no tag-mates");
}
for (const path of ["/en/presentations/feed.xml", "/en/photos/feed.xml", "/en/certificates/feed.xml", "/en/glossary/feed.xml", "/en/reading/feed.xml"]) {
	const r = await get(path);
	check(`${path} 200 application/rss+xml`, r.res.status === 200 && (r.res.headers.get("content-type") || "").startsWith("application/rss+xml"), `${r.res.status}`);
}
for (const path of ["/en/about", "/en/now", "/en/certificates", "/en/photos", "/en/presentations", "/en/garden", "/en/glossary", "/en/reading"]) check(`Sitemap lists ${path}`, sm.body.includes(`${path}</loc>`));
const ph = await get("/en/photos");
const img = ph.body.match(/<img src="(\/media\/[^"]+)"/)?.[1];
if (img) {
	const r = await get(img);
	check("Photos: media served by the Worker", r.res.status === 200 && /^image\//.test(r.res.headers.get("content-type") || ""), `${img} ${r.res.status} ${r.res.headers.get("content-type")}`);
}
for (const a of ["/admin/presentations", "/admin/photos", "/admin/certificates", "/admin/series", "/admin/pages"]) {
	const r = await get(a);
	check(`Admin: ${a} needs a session`, r.res.status === 303, `${r.res.status}`);
}

// 7. Screenshots for the visual comparison with design/*.dc.html.
const browser = await chromium.launch();
const shots = [["home", "/en/"], ["styleguide", "/_styleguide"], ["posts", "/en/posts"]];
if (slug) shots.push(["post", `/en/posts/${slug}`], ["post-tr-fallback", `/tr/posts/${slug}`]);
shots.push(["not-found", "/en/nope"], ["admin-login", "/admin/login"], ...shots4, ...shots5);
for (const [w, h] of [[1280, 900], [390, 844]]) {
	const ctx = await browser.newContext({ viewport: { width: w, height: h }, extraHTTPHeaders: access, locale: "en-US" });
	const page = await ctx.newPage();
	const errors = [];
	page.on("pageerror", (e) => errors.push(e.message));
	for (const [name, path] of shots) {
		await page.goto(origin + path, { waitUntil: "networkidle" });
		const overflow = await page.evaluate(() => document.documentElement.scrollWidth - innerWidth);
		await page.screenshot({ path: `${out}/${name}-${w}.png`, fullPage: true });
		if (overflow > 0) check(`Layout: ${path} at ${w}px has no horizontal scroll`, false, `${overflow}px`);
	}
	// The palette opens and finds something.
	await page.goto(origin + "/en/", { waitUntil: "networkidle" });
	if (w === 1280 && slug) {
		await page.keyboard.press("Control+k");
		await page.keyboard.type(slug.split("-")[0]);
		await page.waitForTimeout(800);
		const rows = await page.locator(".pal-row").count();
		check("Palette: ⌘K opens and lists results", rows > 0, `${rows} rows`);
		await page.screenshot({ path: `${out}/palette-${w}.png` });
	}
	check(`JS errors at ${w}px`, errors.length === 0, errors.join(" | "));
	await ctx.close();
}
await browser.close();

fs.writeFileSync(`${out}/report.json`, JSON.stringify({ origin, at: new Date().toISOString(), results }, null, 2));
const failed = results.filter((r) => !r.ok);
const md = [`## Staging check: ${results.length - failed.length}/${results.length} passed`, "", "| | check | detail |", "|---|---|---|",
	...results.map((r) => `| ${r.ok ? "✅" : "❌"} | ${r.name} | ${String(r.detail).replace(/\|/g, "\\|")} |`)].join("\n");
fs.writeFileSync(`${out}/report.md`, md + "\n");
if (process.env.GITHUB_STEP_SUMMARY) fs.appendFileSync(process.env.GITHUB_STEP_SUMMARY, md + "\n");
process.exit(failed.length ? 1 : 0);
