# Phase 5 prompts: presentations, photos, certificates, personal pages, knowledge

Prerequisite: Phase 4 exit check is all "done".
Run each in a fresh session (`/clear`), in plan mode (Shift+Tab). One commit (or PR) per step.
Every prompt assumes `prompts/_constraints.md` (storage via internal/store, separate SEARCH DB, queue consumer Worker, routes() test).

## 5.1 Presentations + isolated deck origin

```
Read CLAUDE.md, ARCHITECTURE.md ADR-007, CONTENT_MODEL.md §2 (presentation) and design/README.md (Presentations, Presentation Detail). Open both prototypes.
Phase 5, step 1:
1. Migration: presentations, presentation_links.
2. Deck upload in admin: single self-contained HTML file to R2; served from decks.halid.org/{slug}/ by the Worker with its own CSP.
3. Presentations page pixel-close: rose hero, type filter (?type=talk|workshop|lightning), cards with slide count badge.
4. Presentation Detail pixel-close: dark hero, 16:9 sandboxed iframe, control bar (prev/next, n/total, progress, fullscreen), abstract, clickable outline, aside links.
5. postMessage protocol between page and deck (goTo, slideChanged) with origin checks; decks without the listener still render.
6. Admin form for outline_json and links (not designed).
Propose a plan first, then wait for my approval.
```

## 5.2 Photos (500px)

```
Read CLAUDE.md, ARCHITECTURE.md ADR-009, CONTENT_MODEL.md §3 (photos) and design/README.md (Photos). Open design/Photos.dc.html.
Phase 5, step 2:
1. Migration: photo_albums, photos.
2. Photos page pixel-close: violet hero with 500px profile button, album chips with counts (?album=), dense grid with row/col spans.
3. Grid uses local thumbnails from R2; the 500px embed loads only inside the lightbox.
4. Lightbox: large image, album, title, EXIF, "view on 500px", focus trap, Esc and outside click close, arrow keys move.
5. CSP allowlist for the 500px domains.
6. Admin (not designed): album CRUD, photo form with span pickers and drag reorder.
Propose a plan first, then wait for my approval.
```

## 5.3 Certificates

```
Read CLAUDE.md, CONTENT_MODEL.md §3 (certificates) and design/README.md (Certificates). Open design/Certificates.dc.html.
Phase 5, step 3:
1. Migration: cert_issuers (with tone), certificates.
2. Certificates page pixel-close: cyan hero with stat tiles, issuer chips (?issuer=), cards with 4px issuer top border, 112px hexagon badge, skills, date, derived status (active / renew soon within 3 months / expired at 55% opacity), verify link.
3. Status is computed at render time; add a daily cron that enqueues a re-render job so status changes appear without a publish.
4. Admin (not designed): issuer and certificate forms, badge upload.
Propose a plan first, then wait for my approval.
```

## 5.4 About + Now

```
Read CLAUDE.md, CONTENT_MODEL.md §2 (page: now, about) and design/README.md (About, Now). Open both prototypes.
Phase 5, step 4:
1. Go schemas that validate data_json for the now and about templates.
2. About pixel-close: dark hero with aberration name, portrait with offset outlines, skill tiles, experience timeline, link cards (certificates with mini hexagons, talks, homelab, now).
3. Now pixel-close: dark hero with pulse dot and "updated N days ago", building cards with progress, reading covers with offset shadow, learning tiles with 10-cell meter, "not doing" strip.
4. Weather in the Now hero: fetched at render time, cached in KV for 3h via store.KV, omitted on failure.
5. /now/archive (not designed): list of past published revisions with dates, each viewable read-only.
6. Admin structured forms for now and about (not designed), not raw JSON.
7. "updated N days ago" must stay correct: daily re-render job.
Propose a plan first, then wait for my approval.
```

## 5.5 Knowledge kinds: garden, runbook, series, glossary, reading

```
Read CLAUDE.md and CONTENT_MODEL.md §2 (garden, runbook, book, term) and the series table.
Phase 5, step 5 (none of these are designed: derive from Post and list patterns, tones per DESIGN.md §6):
1. Migrations and content packages for garden_pages, runbooks, books, terms.
2. Garden page with maturity badge and "last tended" date; runbook page with platform, prerequisites, verified_on and risk badge.
3. Series index page listing parts in order.
4. Add garden, runbook, term and book to the SEARCH index.
5. Glossary index and term page; auto-link the first mention of a term or alias in posts at render time, with tests for word boundaries and Turkish characters.
6. Reading list page (/reading) grouped by status; Now page reading section pulls from books.
Propose a plan first, then wait for my approval.
```

## 5.6 Backlinks + related

```
Read CLAUDE.md and CONTENT_MODEL.md §1 (links_graph).
Phase 5, step 6:
1. Rebuild links_graph on render from internal links in body_md.
2. Backlinks block on all long-form kinds; purge rules re-render targets when a new backlink appears.
3. "Related" block: shared tags plus series, max 3, no external calls.
4. Delete protection: typed-slug confirmation when backlinks exist, then redirect or 410 tombstone.
Propose a plan first, then wait for my approval.
```

## Phase 5 exit check

```
Check Phase 5 items in ROADMAP.md against the repo and prod. Verify: a deck renders from decks.halid.org and outline clicks change slides, photos grid loads without 500px requests until the lightbox opens, a certificate expiring within 3 months shows "renew soon", Now shows the correct "updated N days ago". Compare Presentations, Presentation Detail, Photos, Certificates, About and Now with design/*.dc.html and list visual differences. Do not change code.
```
