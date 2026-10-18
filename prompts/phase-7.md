# Phase 7 prompts: discovery and polish

Prerequisite: Phase 6 exit check is all "done".
Run each in a fresh session (`/clear`), in plan mode (Shift+Tab). One commit (or PR) per step.
Every prompt assumes `prompts/_constraints.md` (storage via internal/store, separate SEARCH DB, queue consumer Worker, routes() test).

## 7.1 Full-site search

```
Read CLAUDE.md, prompts/_constraints.md, ARCHITECTURE.md ADR-010 and docs/spike-phase0.md.
Phase 7, step 1. The SEARCH DB, folding and per-entry indexing already exist from Phases 1 to 5; harden them:
1. Coverage audit: every public kind is indexed in both languages; photos and certificates by title/issuer only. Add an admin "rebuild search index" job that recreates the SEARCH DB from the main DB.
2. Ranking: bm25 weights (title > summary > body), recency boost, prefix search on the last token. Public queries exclude drafts; admin queries include them.
3. Command palette: query the backend, keep prefixes, show kind dot and label, keyboard navigation unchanged.
4. /search?q= results page for no-JS (not designed: stream row pattern).
5. Tests for ranking basics and Turkish normalization (ı/İ/I folded in Go; ş, ğ, ç, ö, ü handled by remove_diacritics 2).
Propose a plan first, then wait for my approval.
```

## 7.2 Mermaid

```
Read CLAUDE.md and DESIGN.md §4.
Phase 7, step 2:
1. Mermaid blocks render client-side, lazy-loaded only on pages that contain them, themed with DESIGN.md tokens.
2. Fallback: show the source in a code block when JS is off.
3. Investigate server-side SVG (Cloudflare Browser Rendering from the Worker, or mermaid-cli in the container; note the image size and cold start impact) and write the trade-offs to docs/mermaid.md. Do not implement it yet.
Propose a plan first, then wait for my approval.
```

## 7.3 Bilingual polish

```
Read CLAUDE.md and ARCHITECTURE.md §6 (i18n).
Phase 7, step 3:
1. Language toggle on every page goes to the paired translation; if none exists, show the untranslated banner pattern or a "not translated yet" page.
2. hreflang and x-default on every page, sitemap with alternates.
3. Audit all UI strings: everything goes through the i18n catalog; list missing TR or EN keys.
4. Admin: translation status column and "create tr/en draft" on every kind.
Propose a plan first, then wait for my approval.
```

## 7.4 Brand assets

```
Read CLAUDE.md and DESIGN.md §1 and §9.
Phase 7, step 4 (only after I provide the SVG wordmark and favicon; ask me for them first):
1. Replace the text wordmark with the SVG in header, footer, login and OG, keeping the aberration effect.
2. Favicon set (svg, ico, apple-touch-icon), web manifest, theme-color.
Propose a plan first, then wait for my approval.
```

## 7.5 Performance budget

```
Read CLAUDE.md and ROADMAP.md Phase 7.
Phase 7, step 5:
1. Measure LCP, CLS and JS weight on Home, Post, Projects, Photos and Radar (mobile profile).
2. Fix anything over budget (LCP < 1.2s, JS < 30KB on content pages): image sizes and formats, font subsetting and preload, critical CSS, cache headers.
3. Add a CI check that fails if a content page ships more than 30KB of JS.
Propose a plan first, then wait for my approval.
```

## Phase 7 exit check

```
Check Phase 7 items in ROADMAP.md against prod. Report search results for 3 Turkish and 3 English queries, Lighthouse numbers for the 5 pages above, and any page missing hreflang. Do not change code.
```
