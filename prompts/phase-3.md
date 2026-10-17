# Phase 3 prompts: Ghost importer and launch

Prerequisite: Phase 2 exit check is all "done". Have a fresh Ghost JSON export and the content/images folder ready.
Run each in a fresh session (`/clear`), in plan mode (Shift+Tab). One commit (or PR) per step.

## 3.1 Ghost importer (dry run)

```
Read CLAUDE.md and CONTENT_MODEL.md §7.
Phase 3, step 1:
1. internal/importer/ghost: parse the Ghost JSON export (posts, pages, tags, posts_tags, users) and convert lexical/mobiledoc to Markdown.
2. Map Ghost cards: code (with language), image, gallery, callout, bookmark, embed, HTML. Anything unknown becomes a fenced HTML block and is listed in the report.
3. Image handling: upload referenced images to R2 as media rows, rewrite URLs.
4. Dry run only in this step: produce a report (counts, unknown cards, missing images, slug collisions, old URL to new URL map). No writes to D1.
5. Tests with a small fixture export.
Propose a plan first, then wait for my approval.
```

## 3.2 Import commit, redirects, admin screens

```
Read CLAUDE.md and ARCHITECTURE.md §2.
Phase 3, step 2:
1. Commit mode: write entries, revisions (origin = import), tags, media and redirects (301) in batches, idempotent on re-run (keyed by Ghost id).
2. Admin Import screen (not designed): upload export + images zip, show the dry-run report, confirm, progress via job_runs.
3. Admin Redirects screen (not designed): list, add, delete, test a path.
4. Re-render all imported entries and warm KV.
Propose a plan first, then wait for my approval.
```

## 3.3 Content swap + pre-launch checks

```
Read CLAUDE.md and DESIGN.md §9.
Phase 3, step 3:
1. Find every placeholder from the prototypes in templates and seeds (names, bio, tagline, sample posts, counts) and move them to site_settings or real content. List what I still need to provide.
2. Crawl staging: every internal link returns 200, every old Ghost URL from the import map returns 301 to a 200.
3. Lighthouse on Home and one Post (mobile): report LCP, CLS, JS size against the Phase 7 budget.
4. Check robots.txt and noindex on staging, index on prod.
Do not deploy to prod. Propose a plan first, then wait for my approval.
```

## 3.4 Production cutover runbook

```
Read CLAUDE.md and ARCHITECTURE.md §5.
Phase 3, step 4: write docs/runbook-cutover.md, do not execute it.
1. Steps: final Ghost export, prod import, prod Google OAuth client and redirect URIs, secrets, DNS switch for halid.org, cache warm-up, smoke tests, rollback plan (DNS back to Ghost).
2. Mark which steps I run manually and which commands Claude Code can run.
3. Nightly D1 export to R2 enabled before cutover.
Propose a plan first, then wait for my approval.
```

## Phase 3 exit check

```
Check Phase 3 items and exit criteria in ROADMAP.md against prod. Verify: old Ghost URLs redirect, 404 log is near zero after 24h, backups exist in R2, admin login works on prod with an @halid.org account and fails with another account. Report with evidence. Do not change code.
```
