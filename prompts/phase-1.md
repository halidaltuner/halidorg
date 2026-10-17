# Phase 1 prompts: design system, core engine, posts

Prerequisite: Phase 0 exit check is all "done" and `v0.0.1` is tagged.
Run each in a fresh session (`/clear`), in plan mode (Shift+Tab). One commit (or PR) per step. Order matters.
Every prompt below assumes `prompts/_constraints.md` (Phase 0 spike findings).

## 1.0 Persist constraints (one-time, short)

```
Read prompts/_constraints.md and CLAUDE.md.
Add a "Known constraints" section to CLAUDE.md that links to prompts/_constraints.md and docs/spike-phase0.md, and lists the three Worker/container pitfalls and the rule "all storage access from Go goes through internal/store" in one line each. Do not duplicate the whole file. Show me the diff before committing.
```

## 1.1 Design tokens + shared partials (extend, don't rewrite)

```
Read CLAUDE.md, prompts/_constraints.md, DESIGN.md §1 to §4 and design/README.md (Design Tokens, Shared Components). Open design/Header.dc.html, design/Footer.dc.html and design/Cover.dc.html.
Phase 1, step 1. tokens.css, app.css, self-hosted fonts and partials/lang_switch.html already exist from Phase 0: extend them, do not recreate them.
1. Complete tokens.css from DESIGN.md §3 (base, dark band, section tones, type scale, spacing, radius, shadows, focus). Keep existing token names used by Admin Login.
2. Go template partials, pixel-close to the prototypes: header (nav, more dropdown, existing lang switch reused as the tr/en segmented toggle, search button), footer, cover (real image or striped tone fallback), chips, callout, tag pill, status badge.
3. Mobile header and nav sheet under 900px (not designed: derive from the header, hit targets 44px, works without JS via <details>).
4. A dev-only /_styleguide page (404 in prod) that renders every partial and token.
5. Any new routes must pass the routes() test.
No content model yet. Propose a plan first, then wait for my approval.
```

## 1.2 Core schema, search DB, Markdown pipeline

```
Read CLAUDE.md, prompts/_constraints.md, CONTENT_MODEL.md §1, §2 (post only), §6 and ARCHITECTURE.md ADR-002, ADR-003, ADR-006, ADR-010.
Phase 1, step 2:
1. Main DB migration 0002_entries.sql: entries, revisions (origin + autosave coalescing), tags, entry_tags, series, media, redirects, not_found_log, posts. No search tables here.
2. Search DB: create migrations-search/0001_fts.sql with the FTS5 table (unicode61 remove_diacritics 2) for halidorg-search-{env}; add the SEARCH binding and both migration dirs to wrangler.jsonc per env. List the wrangler commands for me to create the search databases.
3. internal/content/post: model, repo, validate, all I/O via internal/store (use Batch for multi-statement writes), with tests.
4. internal/search: index document builder with Turkish ı/İ/I folding, shared by indexer and query parser, with tests. Writing to the index happens in step 1.3.
5. internal/render: goldmark + chroma with heading anchors, TOC JSON, footnotes as side notes, callouts, light code block, dark multi-file code block, terminal block. Match HTML structure to design/Post.dc.html.
6. Golden-file tests: Markdown fixtures in, expected HTML out.
Propose a plan first, then wait for my approval.
```

## 1.3 Render-on-publish, queues, KV purge, search index, feeds

```
Read CLAUDE.md, prompts/_constraints.md and ARCHITECTURE.md §2 and §6.
Phase 1, step 3:
1. Queues: define the jobs queue (and a DLQ) per env in wrangler.jsonc. Add a consumer Worker handler that forwards each message to the container's internal job endpoint; retries and DLQ on failure.
2. Publish flow in Go: save revision, render, enqueue render + purge + index jobs.
3. Purge rules in internal/jobs/purge.go with tests: for a post, rewrite KV via store.KV for the item page, post list, affected tag pages, home, feeds and sitemap, in both languages where a translation exists.
4. Search index job: upsert/delete the entry's row in the SEARCH DB on publish/unpublish (via the Worker bridge or a dedicated SEARCH route on the bridge; pick one and document it).
5. job_runs table with attempt/max_attempts; dashboard will read it.
6. RSS, Atom and JSON Feed, sitemap.xml, robots.txt (noindex on staging), canonical and hreflang.
7. not_found_log: Worker records 404s.
8. New routes pass the routes() test.
Propose a plan first, then wait for my approval.
```

## 1.4 Public: Home + Post + lists

```
Read CLAUDE.md, prompts/_constraints.md, DESIGN.md §5 and design/README.md (Home, Post). Open design/Home.dc.html and design/Post.dc.html.
Phase 1, step 4:
1. Home pixel-close: dark hero with featured post, section tiles (only pages that exist; hide the rest), latest posts, stream with posts only.
2. Post page pixel-close: cover, sticky TOC with scroll spy, series breadcrumb and prev/next, side notes, backlinks block (empty state), untranslated banner.
3. Post list and tag page (not designed: reuse latest-post cards and chips).
4. 404 and 410 pages (not designed: dark band, short copy, link home).
5. URLs per CONTENT_MODEL.md §5 with /tr and /en prefixes; language from the existing i18n.FromRequest.
6. Seed command that inserts 3 sample posts in both languages (local and staging only).
7. Verify on staging with the Access service token: second request for a page is a KV hit.
Propose a plan first, then wait for my approval.
```

## 1.5 OG image renderer

```
Read CLAUDE.md, prompts/_constraints.md and design/README.md (OG Template). Open design/OG Template.dc.html.
Phase 1, step 5:
1. Fonts: Go opentype cannot read woff2. Add TTF files of IBM Plex Sans (400, 600) and JetBrains Mono (400, 700) from their official OFL releases under app/internal/render/og/fonts, with their license files. Tell me the exact source URLs before downloading.
2. Go renderer for 1200x630 PNG matching the template: dark bg with grid texture, kind dot, mono kind + date, 76px title (max 960px, ellipsis after 3 lines), 44px wordmark with aberration, meta on the right.
3. Render as a job on publish, store in R2 via store.Blob, reference in og:image.
4. Snapshot tests for short, long and Turkish-character titles (ğüşıöç İ).
Propose a plan first, then wait for my approval.
```

## 1.6 Admin: Dashboard + Editor

```
Read CLAUDE.md, prompts/_constraints.md, DESIGN.md §7 and design/README.md (Admin Dashboard, Admin Editor). Open design/Admin Dashboard.dc.html and design/Admin Editor.dc.html.
Phase 1, step 6:
1. Admin shell behind auth.RequireSession: 208px sidebar, 52px top bar, staging badge. Show only sections that exist; others greyed out.
2. Dashboard: drafts, scheduled + recent publishes with translation status, failed jobs from job_runs with retry, 404s with "add redirect". Hide the webmentions panel until Phase 8.
3. Editor: CodeMirror 6 | htmx live preview | 280px side panel (kind, series, tags, cover drop zone, SEO counter n/160, translation status with "create tr draft", revisions list).
4. Autosave 800ms debounce with coalescing; ⌘S manual revision; ⌘⇧P publish behind auth.RequireRecentAuth (re-auth redirect must return to the editor with unsaved state preserved).
5. Typed-slug delete confirmation when the entry has backlinks, also behind auth.RequireRecentAuth.
6. Every form works without JS (plain POST fallback).
7. Admin search (drafts included) queries the SEARCH DB.
Propose a plan first, then wait for my approval.
```

## 1.7 Admin: entry list, media, revisions diff

```
Read CLAUDE.md, prompts/_constraints.md and DESIGN.md §6 and §7.
Phase 1, step 7 (screens not designed: derive from dashboard density and tokens):
1. Entry list per kind: filters (status, lang, tag), bulk publish/archive (re-auth), translation status column.
2. Media library: grid, drag-drop upload to R2 via store.Blob (show progress; R2 put is ~180ms p50), alt text TR/EN required before use.
3. Revisions: timeline and side-by-side diff, restore as a new revision.
Propose a plan first, then wait for my approval.
```

## Phase 1 exit check

```
Check Phase 1 items and exit criteria in ROADMAP.md against the repo and staging (use the Access service token). For each item say done, partial or missing, with evidence. Confirm: publish triggers KV rewrite and search index update, a deleted post leaves no KV or search rows, nightly D1 export still succeeds with the new tables. Compare /_styleguide, Home and Post on staging with design/*.dc.html and list visual differences. Do not change code.
```
