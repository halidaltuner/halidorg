# Phase 2 prompts: short-form and curation

Prerequisite: Phase 1 exit check is all "done".
Run each in a fresh session (`/clear`), in plan mode (Shift+Tab). One commit (or PR) per step.

## 2.1 Schema for note, snippet, link, fav + categories

```
Read CLAUDE.md and CONTENT_MODEL.md §1 and §2 (note, snippet, link, fav, awesome list).
Phase 2, step 1:
1. Migration: notes, snippets, snippet_files, links, favs, categories (with tone).
2. internal/content/{note,snippet,link,fav}: model, repo, validate, tests. Reuse the entries + revisions machinery from Phase 1.
3. Purge rules for each new kind in internal/jobs/purge.go (item, list, category, home stream, feeds, sitemap) with tests.
4. Per-kind RSS/Atom/JSON feeds.
Propose a plan first, then wait for my approval.
```

## 2.2 Public: Snippets + Notes

```
Read CLAUDE.md and design/README.md (Snippets). Open design/Snippets.dc.html.
Phase 2, step 2:
1. Snippets page pixel-close: list on the left, viewer on the right with file tabs (amber underline), language badge, line-number toggle, raw link, copy.
2. /snippets/{slug} permalink and /snippets/{slug}/raw/{filename} as text/plain.
3. Notes list and note permalink (not designed: indigo tone, card without title emphasis, timestamp as permalink).
Propose a plan first, then wait for my approval.
```

## 2.3 Public: Links + Favs + awesome lists

```
Read CLAUDE.md and design/README.md (Links, Favs). Open design/Links.dc.html and design/Favs.dc.html.
Phase 2, step 3:
1. Links pixel-close: grouped by category, favicon, mono domain, comment, read/unread chip; filter all/unread/read reflected in ?status=.
2. Read/unread toggle is admin-only (htmx when signed in); visitors see the chip as static.
3. Favs pixel-close: type filter (?type=), striped cover, type dot, title/author, 5-square rating.
4. Awesome list page template from link categories.
5. Favicon fetching: on save, fetch and store in R2; fall back to a tone glyph.
Propose a plan first, then wait for my approval.
```

## 2.4 Quick Capture + bookmarklet

```
Read CLAUDE.md and design/README.md (Quick Capture). Open design/Quick Capture.dc.html.
Phase 2, step 4:
1. /admin/quick pixel-close at 390px: paste URL, fetch title/description/domain (server-side, with timeout and size limit), choose link/fav/note, comment, tr/en, draft or publish, success screen.
2. Hit targets at least 44px; works without JS as a plain form.
3. Bookmarklet that opens /admin/quick?url=...&title=... in a small window.
4. URL fetch must block private IP ranges and non-http(s) schemes (SSRF guard), with tests.
Propose a plan first, then wait for my approval.
```

## 2.5 Admin: categories, tags, stream, palette

```
Read CLAUDE.md and DESIGN.md §4 and §7.
Phase 2, step 5:
1. Admin categories and tags (not designed: dashboard density): create, rename TR/EN, tone picker limited to DESIGN.md tones, reorder, merge tags.
2. Home stream: add note, snippet, link, fav rows with kind glyph tiles and kind chips (?kind=).
3. Command palette: add prefixes n: s: l: f: backed by a small JSON index rebuilt on publish.
Propose a plan first, then wait for my approval.
```

## Phase 2 exit check

```
Check Phase 2 items and exit criteria in ROADMAP.md against the repo and staging. For each item say done, partial or missing, with evidence. Time the quick capture flow on a phone-sized viewport (target under 30 seconds). Compare Snippets, Links, Favs and Quick Capture with design/*.dc.html and list visual differences. Do not change code.
```
