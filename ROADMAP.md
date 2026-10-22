# Roadmap

Each phase ends with something deployable. Sizes are relative (S/M/L), not dates.
Order: 0, 1, 2, 4, 5, 6, 7, 8, 9, then 3 (launch). Until Phase 3 runs, the site lives on staging and exit checks that say "prod" run against staging.
Screens marked **(designed)** have a high-fidelity prototype in `/design`; build pixel-close. Screens marked **(derive)** are not designed: build from the design system in DESIGN.md, then review.

## Phase 0: Spike and foundations (M)
Goal: validate the accepted runtime (ADR-001: Go in Containers + edge Worker + KV render-on-publish) and lay foundations.

- [x] Wrangler project: edge Worker + Go container + D1 + R2 + KV, staging env
- [x] Verify container access to D1/R2/KV (direct vs Worker bridge), pick one (ADR-001)
- [x] Measure container cold start and KV hit latency
- [x] Google OIDC login restricted to @halid.org, server-side `hd` + email checks, `admin_sessions`, 12h session (ADR-005)
- [x] Admin Login screen **(designed)**: default, loading, non-halid error states
- [x] Cloudflare Access on staging as an extra layer
- [x] D1 migration tooling (`/migrations`, `wrangler d1 migrations`)
- [x] CI: lint, test, build image, deploy staging on `main`, prod on tag
- [x] Decide FTS5 vs Pagefind for search
- [x] Commit `/design` handoff to the repo

**Exit:** "hello" page rendered by Go, cached in KV, served by Worker; Google login works and rejects non-halid accounts.

## Phase 1: Design system, core engine, posts (L)
**Design system**
- [ ] `tokens.css` from DESIGN.md §3 (base, dark band, section tones), self-hosted IBM Plex Sans + JetBrains Mono
- [ ] Partials **(designed)**: Header (nav + more ▾, tr/en toggle), Footer, Cover, chips, callouts, code blocks (light, dark multi-tab, terminal)
- [ ] Command palette shell **(designed)** (⌘K, prefixes; searches posts only for now)
- [ ] Mobile header and nav sheet **(derive)**
- [ ] Chroma themes for light and dark code blocks

**Engine**
- [ ] `entries`, `revisions` (with `origin`, autosave coalescing), `tags`, `media`, `redirects`, `not_found_log`, `site_settings`
- [ ] Markdown pipeline: goldmark, chroma, heading anchors, TOC, footnotes as side notes, callouts, terminal blocks
- [ ] Render-on-publish + KV purge jobs (Queues)
- [ ] OG image renderer in Go from `OG Template` **(designed)**
- [ ] RSS/Atom/JSON Feed, sitemap, robots, canonical, `hreflang`

**Public**
- [ ] Home **(designed)**: dark hero, section tiles, latest posts, stream (posts only at first)
- [ ] Post **(designed)**: cover, sticky TOC, side notes, series nav, backlinks, untranslated banner
- [ ] Post list, tag page **(derive)**, 404/410 **(derive)**

**Admin**
- [ ] Dashboard **(designed)**: drafts, scheduled + recent, failed jobs, 404s (webmentions panel since Phase 8.1)
- [ ] Editor **(designed)**: CodeMirror 6 | htmx preview | side panel, autosave 800ms, ⌘S / ⌘⇧P, SEO counter, translation status, revisions list, typed-slug delete
- [ ] Re-auth gate on publish and delete
- [ ] Entry list, media library, revisions diff **(derive)**

**Exit:** a post can be written, published and read on staging, matching the prototypes.

## Phase 2: Short-form and curation (M)
- [ ] `note`, `snippet` (multi-file), `link` (favicon, read status), `fav` (author, rating squares)
- [ ] Snippets **(designed)**, Links **(designed)**, Favs **(designed)**
- [ ] Notes list and permalink **(derive)**
- [ ] Quick Capture **(designed)**: URL fetch, link/fav/note, tr/en, draft or publish, success screen; plus bookmarklet
- [ ] Categories and tags admin **(derive)**
- [ ] Admin list and editor for notes, snippets (files), links, favs **(derive)**, added after the exit check (2.6)
- [ ] Home stream: add note, snippet, link, fav rows and kind chips (`?kind=`)
- [ ] Palette prefixes `n: s: l: f:`
- [ ] Awesome list pages, per-kind feeds

**Exit:** links, favs and snippets added from phone in under 30 seconds.

## Phase 4: Projects, homelab, GitHub (M)
- [ ] `project` with `project_type` software|homelab, facts, stack, parts, gallery; `buildlog`
- [ ] Projects **(designed)** with GitHub section (52-week strip, language bar, pinned repos, releases)
- [ ] Project Detail **(designed)**: tone hero, fact strip, gallery, stack or parts aside, build log timeline
- [ ] Homelab & DIY **(designed)**: stats tiles, featured build, category chips, build cards
- [ ] GitHub sync job every 6h (`gh_repos`, `gh_activity`, `gh_weekly`, `gh_languages`)
- [ ] `/stream` and `/github` standalone pages **(derive)**; palette prefixes `pr: h:`

## Phase 5: Presentations, photos, certificates, personal pages (L)
- [x] Presentations **(designed)** and Presentation Detail **(designed)**: deck upload to R2, `decks.halid.org` origin, sandboxed iframe, slide controls, outline jump via `postMessage` (ADR-007) (staging: `decks-staging.halid.org`; prod's custom domain joins at the Phase 3 cutover)
- [x] Photos **(designed)**: albums, dense grid with spans, lightbox with EXIF, 500px embed in lightbox only, CSP (ADR-009) (local image and a "view on 500px" link instead of an embed, decided 2026-10-18)
- [x] Certificates **(designed)**: issuers, hexagon badges, derived status (active / renew soon / expired)
- [x] About **(designed)**: hero, portrait, skills, experience timeline, link cards
- [x] Now **(designed)**: building, reading, learning, not doing; `/now/archive` **(derive)** (no weather, decided 2026-10-18)
- [x] Admin forms for presentations, photos, certificates, now and about `data_json` **(derive)**
- [x] `garden`, `runbook`, `series`, `term`, `book` reading list **(derive)**, backlinks + related block (garden and runbook in 5.5a, in the post editor; series in 5.5b; glossary and reading list in 5.5c; backlinks for every kind and the related block, from shared tags, in 5.5d)

## Phase 6: Tech Radar (M)
- [x] `radar_item`, history, editions (6.1: frozen editions only; the admin edits a working copy)
- [x] Radar **(designed)**: SVG, quadrants, rings, new/moved blips, list sync, edition switcher, keyboard (6.2; editions at `/{lang}/radar/{YYYY-MM}`, items at `/{lang}/radar/{slug}`)
- [x] Admin radar editor **(derive)**, edition snapshot (6.1)

## Phase 7: Discovery and polish (M)
- [x] Full-site search behind the palette (FTS5 or Pagefind), no-JS search results page **(derive)** (7.1: FTS5, photos and certificates indexed, recency boost, rebuild job, `/{lang}/search`)
- [x] Mermaid: lazy client render first, server SVG later (7.2: vendored Mermaid 11.17.2, loaded only near a diagram; server SVG options in docs/mermaid.md)
- [x] Bilingual polish: paired URLs, untranslated fallbacks everywhere (7.3: kind lists show entries with no translation in the page's language, tagged and linking to their own page; hreflang x-default; translation column in every admin list; a test that every template key is in the catalog)
- [x] SVG wordmark, favicon and monogram (7.4: drawn from the font by scripts/brand.py; favicon set, apple-touch icon, web manifest)
- [x] Performance budget: LCP < 1.2s, JS < 30KB on content pages (7.5: TestJSBudget in CI, Lighthouse in the staging check, font preloads, eager hero covers; docs/performance.md)
- [x] Block editor (Tiptap, Notion-like) for posts, garden pages and runbooks: "/" commands, a formatting menu on selected text, drag handles; Markdown stays the stored text and CodeMirror stays as the Markdown view (7.6a); then H1, to-do lists, YouTube embeds, file and image upload, syntax colors in the editor (7.6a+)
- [x] Block editor for every other body editor: short kinds, presentations, terms, books, projects, build logs and the About page, all through the shared entry form (7.6b)

## Phase 8: IndieWeb and distribution (S/M)
- [x] Webmention receive + moderation (dashboard panel **(designed)**) and display on detail pages (8.1)
- [ ] Webmention send on publish
- [ ] POSSE to X and LinkedIn (manual confirm), syndication URLs
- [ ] Newsletter optional

## Phase 9: Ops hardening (S)
- [ ] Nightly D1 export + Markdown/JSON export to R2
- [ ] Jobs page with retry (`retry n/3`), based on the dashboard failed-jobs panel **(derive)**; settings **(derive)**
- [ ] Re-render-all job for renderer upgrades
- [ ] Cloudflare Web Analytics

## Phase 3: Ghost importer and launch (M)
Deferred: runs last, after Phase 9. The number stays so prompts and tags keep their names.
- [ ] Ghost JSON + images importer with dry-run report; import + redirects admin **(derive)**
- [ ] 301 redirects for old URLs, 404 report on dashboard
- [ ] Real content swap: replace all prototype placeholders (bio, tagline, names)
- [ ] Production cutover: DNS, OAuth client for prod, backups
- [ ] Remove Ghost

**Exit:** halid.org runs on the new engine, no broken inbound links.

## Backlog
- Comments (webmentions only)
- Public read-only JSON API
- Passkey as second factor
- AI-assisted TR↔EN draft translation in the editor
- `/uses` page design

## Removed after design review
- Dark theme and theme toggle (ADR-008)
- `talk` kind, replaced by `presentation`
- Cloudflare Access as the admin login (replaced by Google SSO, ADR-005)
