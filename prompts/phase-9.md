# Phase 9 prompts: ops hardening

Prerequisite: Phase 8 exit check is done (or Phase 8 consciously skipped).
Run each in a fresh session (`/clear`), in plan mode (Shift+Tab). One commit (or PR) per step.
Every prompt assumes `prompts/_constraints.md` (storage via internal/store, separate SEARCH DB, queue consumer Worker, routes() test).

## 9.1 Backups and export

```
Read CLAUDE.md and ARCHITECTURE.md §6 (backups).
Phase 9, step 1:
1. Nightly D1 export of the main DB to R2 with 30-day retention (Phase 3 now runs after this phase, so create it here; Phase 3 only verifies it before cutover). The SEARCH DB is not exported; document that it is rebuilt with the rebuild-index job.
2. Weekly full export: every entry as Markdown with front matter, gallery data as JSON, media manifest, zipped in R2.
3. Restore runbook in docs/runbook-restore.md: restore main DB, then rebuild the SEARCH DB. Script that restores into a fresh staging database; test it once on staging.
Propose a plan first, then wait for my approval.
```

## 9.2 Jobs, health and settings screens

```
Read CLAUDE.md and DESIGN.md §7.
Phase 9, step 2 (not designed: derive from the dashboard failed-jobs panel):
1. Jobs page: runs table with filters, retry (shows "retry n/3"), payload and error view.
2. Health page: last GitHub sync, last backup, failed jobs and DLQ depth in 24h, KV hit ratio, bridge latency p50/p95, D1 (main + SEARCH) and R2 usage, deployed image SHA vs DEPLOY_SHA seen by the container.
3. Settings page: everything in site_settings with typed inputs.
Propose a plan first, then wait for my approval.
```

## 9.3 Re-render all + analytics

```
Read CLAUDE.md and ARCHITECTURE.md ADR-002.
Phase 9, step 3:
1. "Re-render all" job in batches with progress in job_runs; safe to resume after failure.
2. Run it automatically when the renderer version changes (store renderer_version in site_settings).
3. Cloudflare Web Analytics snippet on public pages only, no cookies, nothing on admin.
Propose a plan first, then wait for my approval.
```

## 9.4 Security review

```
Read CLAUDE.md and ARCHITECTURE.md.
Phase 9, step 4: review only, no code changes.
Check CSP on every origin, cookie flags, OIDC claim checks (including auth_time fail-closed), that bridge.internal is unreachable from the internet, re-auth gate on publish and delete, SSRF guards, deck origin isolation, secrets handling, rate limits on login, webmention and quick capture endpoints. Write findings with severity to docs/security-review.md.
```

## Phase 9 exit check

```
Check Phase 9 items in ROADMAP.md against prod. Confirm a backup from the last 24h exists and a restore to staging was tested, and summarize open items from docs/security-review.md. Do not change code.
```
