// Renders the brand PNGs and favicon.ico from the SVGs scripts/brand.py
// writes (Phase 7.4), with the Chromium Playwright finds. Run from the repo
// root: node scripts/brand-png.mjs (NODE_PATH pointing at playwright).
import { createRequire } from "node:module";
import { readFileSync, writeFileSync } from "node:fs";

// require, not import: it honours NODE_PATH, where Playwright is installed.
const { chromium } = createRequire(import.meta.url)("playwright");

const dir = "app/web/static/brand/";
const shots = [
	["monogram.svg", 16, "favicon-16.png"],
	["monogram.svg", 32, "favicon-32.png"],
	["monogram.svg", 48, "favicon-48.png"],
	["monogram.svg", 192, "icon-192.png"],
	["monogram.svg", 512, "icon-512.png"],
	["maskable.svg", 512, "icon-maskable-512.png"],
	["apple.svg", 180, "apple-touch-icon.png"],
];
const browser = await chromium.launch();
const page = await browser.newPage();
const png = {};
for (const [src, size, out] of shots) {
	const svg = readFileSync(dir + src, "utf8");
	await page.setViewportSize({ width: size, height: size });
	await page.setContent(`<html><body style="margin:0;background:transparent">${svg.replace("<svg ", `<svg width="${size}" height="${size}" `)}</body></html>`);
	const buf = await page.screenshot({ omitBackground: true, clip: { x: 0, y: 0, width: size, height: size } });
	writeFileSync(dir + out, buf);
	png[size] = png[size] || buf;
}
await browser.close();

// favicon.ico: 16, 32 and 48 as PNG entries (every browser since IE11 reads them).
const sizes = [16, 32, 48];
const header = Buffer.alloc(6 + 16 * sizes.length);
header.writeUInt16LE(0, 0);
header.writeUInt16LE(1, 2);
header.writeUInt16LE(sizes.length, 4);
let offset = header.length;
const bodies = sizes.map((s) => readFileSync(dir + `favicon-${s}.png`));
sizes.forEach((s, i) => {
	const e = 6 + 16 * i;
	header.writeUInt8(s, e);
	header.writeUInt8(s, e + 1);
	header.writeUInt8(0, e + 2);
	header.writeUInt8(0, e + 3);
	header.writeUInt16LE(1, e + 4);
	header.writeUInt16LE(32, e + 6);
	header.writeUInt32LE(bodies[i].length, e + 8);
	header.writeUInt32LE(offset, e + 12);
	offset += bodies[i].length;
});
writeFileSync(dir + "favicon.ico", Buffer.concat([header, ...bodies]));
console.log("brand PNGs and favicon.ico written");
