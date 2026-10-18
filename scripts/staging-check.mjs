// Phase 1 exit check against staging (prompts/phase-1.md, "Phase 1 exit
// check"). Read-only: it never signs in to the admin and never writes.
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

// The first request may be a MISS (a cold key) or already a HIT (warmed by a
// publish job); the second must be a HIT.
async function kvHit(path) {
	const first = await get(path);
	const second = await get(path);
	const a = first.res.headers.get("x-cache"), b = second.res.headers.get("x-cache");
	check(`KV: ${path} second request is a HIT`, second.res.status === 200 && b === "HIT", `${first.res.status} ${a} → ${second.res.status} ${b}`);
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

// 5. Screenshots for the visual comparison with design/*.dc.html.
const browser = await chromium.launch();
const shots = [["home", "/en/"], ["styleguide", "/_styleguide"], ["posts", "/en/posts"]];
if (slug) shots.push(["post", `/en/posts/${slug}`], ["post-tr-fallback", `/tr/posts/${slug}`]);
shots.push(["not-found", "/en/nope"], ["admin-login", "/admin/login"]);
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
