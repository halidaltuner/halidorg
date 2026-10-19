# Mermaid diagrams

Status: client rendering shipped in Phase 7.2 (2026-10-19). Server-side SVG
is investigated here and not built.

## What ships now

- The renderer keeps a ```` ```mermaid ```` block as its source in
  `<pre class="mermaid">` and marks the post (`posts.mermaid`). Without JS the
  reader sees the source, which is readable on its own.
- Posts, garden pages and runbooks with a diagram load `static/js/mermaid.js`
  (2 KB). It imports Mermaid only when a diagram is within 600px of the
  viewport, then replaces the block with the SVG, themed from the design
  tokens (Plex Sans, the cyan, indigo and amber tones, `--border`). A block
  that fails to parse keeps its source with a rose border.
- Mermaid 11.17.2 is vendored, pinned in `editor/package.json` and built by
  `scripts/build-editor.sh` into `app/web/static/mermaid`: the ESM entry
  (32 KB) and its chunks in one flat directory, 3.5 MB on disk. A page loads
  only the chunks its diagram types need: a flowchart is 27 files, 860 KB
  uncompressed and 230 KB gzipped (measured 2026-10-19). Chunk names carry a content
  hash and are served immutable for a year; the entry is linked with its
  `?v=` hash.
- No third-party origin: nothing to add to a CSP, nothing that breaks when a
  CDN does.

Costs: the first diagram on a page costs about 230 KB of compressed JS and
the main-thread work to parse it and lay the diagram out (not measured yet:
Phase 7.5), and there is a layout shift when the source block becomes a
figure of a different height.
Pages without diagrams pay nothing. The Phase 7.5 budget (30 KB of JS on
content pages) counts what a page loads up front; the Mermaid chunks load
on demand and are listed as the one exception.

## Server-side SVG: options

The page could carry the SVG itself: no JS, no layout shift, diagrams in
feeds and in KV. The renderer is Go and Mermaid needs a browser DOM (it
measures text), so the SVG has to come from a browser somewhere.

### A. mermaid-cli in the container

`@mermaid-js/mermaid-cli` (`mmdc`) drives headless Chromium through
Puppeteer.

- Image size: Chromium and its libraries add several hundred megabytes to an
  image that today holds only the Go binary, templates, static files and
  fonts, and Node comes with it.
- Cold start: the container image is pulled and started on demand; a tenfold
  bigger image makes cold starts slower, and public pages must not wait on a
  warm container (CLAUDE.md). Rendering runs at publish time in a job, so a
  reader never waits, but every deploy and every cold job pays for it.
- Memory: a headless Chromium needs far more memory than the Go server; the
  container instance type would likely have to grow.
- Verdict: works, but makes the whole app heavier for a feature a handful of
  posts use.

### B. Cloudflare Browser Rendering from the Worker or a job

A headless browser on Cloudflare's side (Workers binding `BROWSER`, or the
REST API). The render job would send the diagram source and get the SVG
back, then store it with the revision (`revisions.data_json` or R2 under
`og/`-like keys).

- Image size: unchanged.
- Cold start: a browser session takes seconds to start; acceptable at
  publish time, not per request.
- Cost and limits: billed per browser hour on the paid plan, with concurrency
  limits; publish-time rendering of a few diagrams a month is far inside them.
- Moving parts: a new binding, a page with Mermaid loaded inside the browser
  (from our vendored copy), and a fallback when the service is down (keep the
  source and the client island, which is what ships now).
- Verdict: the better fit if diagrams become common. The client island stays
  as the fallback, so it can be added without changing posts.

### C. A pure JS renderer without a browser

Projects that lay out Mermaid without a DOM (for example a Rust port) cover
only some diagram types and drift from Mermaid's syntax. Not worth tracking
now.

## Recommendation

Keep client rendering. Revisit when a post needs a diagram in its feed or the
layout shift shows up in the Phase 7.5 numbers; then build option B at
publish time, storing the SVG with the revision, with the island as fallback.
