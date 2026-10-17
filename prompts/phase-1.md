# Phase 1 prompts for Claude Code

Prerequisite: Phase 0 exit check is all "done".
Run each in a fresh session (`/clear`), in plan mode (Shift+Tab). One commit (or PR) per step. Order matters.

## 1.1 Design tokens + shared partials

```
Read CLAUDE.md, DESIGN.md §1 to §4 and design/README.md (Design Tokens, Shared Components). Open design/Header.dc.html, design/Footer.dc.html and design/Cover.dc.html.
Phase 1, step 1:
1. Complete web/static/css/tokens.css from DESIGN.md §3 (base, dark band, section tones, type scale, spacing, radius, shadows, focus).
2. Self-host IBM Plex Sans and JetBrains Mono (woff2, font-display: swap) under web/static/fonts.
3. Go template partials, pixel-close to the prototypes: header (nav, more dropdown, tr/en toggle, search button), footer, cover (real image or striped tone fallback), chips, callout, tag pill, status badge.
4. Mobile header and nav sheet under 900px (not designed: derive from the header, keep hit targets 44px).
5. A dev-only /_styleguide page that renders every partial and token, so I can compare it side by side with the prototypes.
No content model yet. Propose a plan first, then wait for my approval.
```

## 1.2 Core schema + Markdown pipeline

```
Read CLAUDE.md, CONTENT_MODEL.md §1, §2 (post only), §6 and ARCHITECTURE.md ADR-002, ADR-003, ADR-006.
Phase 1, step 2:
1. Migration 0002_entries.sql: entries, revisions (with origin and autosave coalescing rules), tags, entry_tags, series, media, redirects, not_found_log, posts.
2. internal/content/post: model, repo (hand-written SQL), validate, with tests.
3. internal/render: goldmark + chroma pipeline with heading anchors, TOC JSON, footnotes rendered as side notes, callouts (tip, warning, note, danger), light code block, dark multi-file code block, terminal block. Match the HTML structure to the Post prototype (open design/Post.dc.html).
4. Golden-file tests: Markdown fixtures in, expected HTML out.
Propose a plan first, then wait for my approval.
```

## 1.3 Render-on-publish, KV purge, feeds

```
Read CLAUDE.md and ARCHITECTURE.md §2 (request paths) and §6.
Phase 1, step 3:
1. Publish flow: save revision, render, enqueue render + purge jobs on Queues.
2. Purge rules: for a post, rewrite KV for the item page, post list, affected tag pages, home, feeds and sitemap, in both languages where a translation exists. Put the rules in one place (internal/jobs/purge.go) with tests.
3. job_runs table and retry with attempt/max_attempts.
4. RSS, Atom and JSON Feed, sitemap.xml, robots.txt, canonical and hreflang tags.
5. not_found_log: record 404s from the Worker.
Propose a plan first, then wait for my approval.
```

## 1.4 Public: Home + Post + lists

```
Read CLAUDE.md, DESIGN.md §5 and design/README.md (Home, Post). Open design/Home.dc.html and design/Post.dc.html.
Phase 1, step 4:
1. Home pixel-close: dark hero with featured post, section tiles (link only to pages that exist; hide the rest for now), latest posts, stream with posts only.
2. Post page pixel-close: cover, sticky TOC with scroll spy, series breadcrumb and prev/next, side notes, backlinks block (empty state for now), untranslated banner.
3. Post list and tag page (not designed: reuse latest-post cards and chips).
4. 404 and 410 pages (not designed: dark band, short copy, link home).
5. URLs per CONTENT_MODEL.md §5 with /tr and /en prefixes; language default from Accept-Language.
6. Seed command that inserts 3 sample posts in both languages for local testing.
Propose a plan first, then wait for my approval.
```

## 1.5 OG image renderer

```
Read CLAUDE.md and design/README.md (OG Template). Open design/OG Template.dc.html.
Phase 1, step 5:
1. Go renderer for 1200x630 PNG OG images matching the template: dark bg with grid texture, kind dot, mono kind + date, 76px title (wrap to max 960px, ellipsis after 3 lines), 44px wordmark with aberration, meta on the right.
2. Use the self-hosted fonts. Render on publish, store in R2, reference in og:image.
3. Snapshot tests for short, long and Turkish-character titles.
Propose a plan first, then wait for my approval.
```

## 1.6 Admin: Dashboard + Editor

```
Read CLAUDE.md, DESIGN.md §7 and design/README.md (Admin Dashboard, Admin Editor). Open design/Admin Dashboard.dc.html and design/Admin Editor.dc.html.
Phase 1, step 6:
1. Admin shell: 208px sidebar, 52px top bar, staging badge. Show only sections that exist; others greyed out.
2. Dashboard: drafts, scheduled + recent publishes with translation status, failed jobs with retry, 404s with "add redirect". Hide the webmentions panel until Phase 8.
3. Editor: CodeMirror 6 | htmx live preview | 280px side panel (kind, series, tags, cover drop zone, SEO counter n/160, translation status with "create tr draft", revisions list).
4. Autosave 800ms debounce with coalescing; ⌘S saves a manual revision; ⌘⇧P publishes behind the re-auth gate.
5. Typed-slug delete confirmation when the entry has backlinks.
6. Every form must work without JS (plain POST fallback).
Propose a plan first, then wait for my approval.
```

## 1.7 Admin: entry list, media, revisions diff

```
Read CLAUDE.md and DESIGN.md §6 and §7.
Phase 1, step 7 (screens not designed: derive from the dashboard density and tokens):
1. Entry list per kind: filters (status, lang, tag), bulk publish/archive, translation status column.
2. Media library: grid, drag-drop upload to R2, alt text TR/EN required before an image can be used.
3. Revisions: timeline and side-by-side diff, restore as a new revision.
Propose a plan first, then wait for my approval.
```

## Phase 1 exit check

```
Check Phase 1 items and exit criteria in ROADMAP.md against the repo and staging. For each item say done, partial or missing, with evidence. Then compare /_styleguide, Home and Post on staging against design/*.dc.html and list visual differences. Do not change code.
```
