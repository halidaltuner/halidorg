// Lighthouse runs for the staging check (Phase 7.5, ROADMAP "Performance
// budget: LCP < 1.2s, JS < 30KB on content pages"). Desktop is judged: LCP
// under 1.2 s and under 30 KB of script transferred up front; mobile (slow
// 4G, a mid-range phone) is measured and reported, not judged. Each run's
// HTML report is written next to the screenshots.
import fs from "node:fs";
import lighthouse from "lighthouse";
import desktopConfig from "lighthouse/core/config/desktop-config.js";
import * as chromeLauncher from "chrome-launcher";

export const LCP_BUDGET_MS = 1200;
export const JS_BUDGET_BYTES = 30 << 10;

// run audits paths on origin with Chrome at chromePath, sending headers
// (the Access service token), and calls check(name, ok, detail) per result.
export async function runLighthouse({ origin, paths, headers, chromePath, out, check }) {
	const chrome = await chromeLauncher.launch({ chromePath, chromeFlags: ["--headless=new", "--no-sandbox", "--disable-gpu"] });
	try {
		for (const path of paths) {
			for (const [form, config] of [["desktop", desktopConfig], ["mobile", undefined]]) {
				const flags = { port: chrome.port, output: "html", onlyCategories: ["performance"], extraHeaders: headers, logLevel: "error" };
				let r;
				try {
					r = await lighthouse(origin + path, flags, config);
				} catch (err) {
					check(`Lighthouse ${form}: ${path}`, false, err.message);
					continue;
				}
				const a = r.lhr.audits;
				const lcp = a["largest-contentful-paint"]?.numericValue ?? NaN;
				const cls = a["cumulative-layout-shift"]?.numericValue ?? NaN;
				const tbt = a["total-blocking-time"]?.numericValue ?? NaN;
				const score = Math.round((r.lhr.categories.performance?.score ?? 0) * 100);
				// Script bytes over the wire, the islands only: same-origin
				// /static/ scripts (nothing else is allowed on the page).
				const js = (a["network-requests"]?.details?.items || [])
					.filter((i) => i.resourceType === "Script")
					.reduce((n, i) => n + (i.transferSize || 0), 0);
				const name = `lighthouse-${form}${path.replace(/[^a-z0-9]+/gi, "-").replace(/-$/, "")}.html`;
				fs.writeFileSync(`${out}/${name}`, r.report);
				const detail = `score ${score} · LCP ${Math.round(lcp)} ms · CLS ${cls.toFixed(3)} · TBT ${Math.round(tbt)} ms · JS ${(js / 1024).toFixed(1)} KB · ${name}`;
				if (form === "desktop") {
					check(`Lighthouse desktop: ${path} LCP < ${LCP_BUDGET_MS} ms`, lcp < LCP_BUDGET_MS, detail);
					check(`Lighthouse desktop: ${path} script < 30 KB`, js < JS_BUDGET_BYTES, detail);
				} else {
					check(`Lighthouse mobile: ${path} (reported)`, true, detail);
				}
			}
		}
	} finally {
		await chrome.kill();
	}
}
