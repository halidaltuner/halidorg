# CLAUDE.md

Rules for Claude Code working on the halid.org engine.

## Read first
README.md, ARCHITECTURE.md, CONTENT_MODEL.md, DESIGN.md, ROADMAP.md. Work only on the current ROADMAP phase unless asked.

## Design
- `/design/*.dc.html` are high-fidelity prototypes; `/design/README.md` describes each screen. Open the matching prototype before building a screen and match it pixel-close.
- Prototypes win over DESIGN.md. If you find a conflict, follow the prototype and update DESIGN.md in the same change.
- Never copy `support.js`, `image-slot.js` or inline prototype styles. Rebuild as Go templates + `tokens.css` / `app.css`.
- Light theme only. No theme toggle, no dark-mode media queries. Dark only as designed bands.
- Small text links use `--link`, not `--accent`.
- For screens marked **(derive)** in ROADMAP.md, compose from existing partials and tokens; don't invent new colors or tones.
- Prototype content is placeholder. Don't hard-code names, bio or sample data in templates.

## Stack
- Go (latest stable), `net/http` + `html/template`, htmx, minimal vanilla JS islands
- Markdown: goldmark + extensions, chroma for highlighting
- Cloudflare: Workers, Containers, D1, R2, KV, Queues, Cron, Access
- Runtime split (ADR-001): Go app runs in a Container; `/worker` is a thin TypeScript edge Worker. No Go/WASM.
- Migrations: plain SQL in `/migrations`, applied with `wrangler d1 migrations`

## Known constraints
Phase 0 spike findings. Full list in `prompts/_constraints.md`, details in `docs/spike-phase0.md`.
- All storage access from Go goes through `internal/store` (Worker bridge); never call Cloudflare APIs from the container.
- Assign `outboundByHost` inside a `static {}` block, not as a class field, and keep `export { ContainerProxy }`.
- Container env vars are set in the Durable Object constructor; a new image does not restart the DO, so new secrets or `DEPLOY_SHA` may lag.
- ServeMux patterns can conflict and panic (`GET /static/` vs `/{lang}/{$}`); every route change must pass the `routes()` test in `cmd/server/main_test.go`.
- Language-prefixed routes register literal prefixes per language (`for _, l := range i18n.Supported { "GET /"+l+"/posts" }`), never `/{lang}/literal`: that overlaps `/lang/{lang}` and ServeMux panics.
- Every write to an entry enqueues `jobs.TypeEntryChanged` with the pre-write `jobs.RefOf` snapshot; the job renders its OG card, re-renders affected pages into KV (`jobs.Paths`) and re-indexes search. New kinds add their paths in `jobs/purge.go` and their card in `jobs.EntryJobs.cards` (`render/og`).
- Every state-changing admin route is wrapped in `admin.Guard` (CSRF, `http.CrossOriginProtection`) inside `auth.RequireSession`. Publish checks `auth.Recent` itself after saving, so re-auth never drops typed text; delete uses `auth.RequireRecentAuth`.
- Kind admin pages build on `admin.UI` (shell, render, fragments) in `internal/content/<kind>/handlers_admin.go`. Short kinds (note, snippet, link, fav) describe themselves as a `shortadmin.Kind`; `internal/content/shortadmin` owns their list, editor and save/publish/delete flow. Tests that drive a kind's repo from package `jobs` live in package `jobs_test`: kinds import `jobs` to enqueue.
- KV pages carry the deploy SHA that rendered them (`PageMeta.deploy`). The Worker serves a page from an older deploy and re-renders it in the background (`X-Cache: STALE`), and `scripts/refresh-edge.sh` touches every page after each deploy. Template and CSS changes therefore reach unchanged content without a purge.
- Public R2 objects (`/og/*`, `/media/*`) are served by the Worker (`worker/src/r2public.ts`), never by the container; the Content-Type comes from the key's extension, which the app picks after sniffing the bytes. Media is images only, no SVG.
- Admin JS vendors (htmx, CodeMirror 6) are built by `scripts/build-editor.sh` from pinned versions in `editor/package.json` and committed under `app/web/static/vendor`.

## Conventions
- Each content kind: `internal/content/<kind>/` with `model.go`, `repo.go`, `validate.go`, `handlers_admin.go`, `handlers_public.go`, templates under `web/templates/<kind>/`
- No ORM. Hand-written SQL with small query helpers.
- IDs are ULIDs. Times are unix seconds in UTC.
- All user-visible strings go through the i18n catalog (`tr`, `en`).
- Templates use design tokens only (CSS custom properties). No hard-coded colors.
- No em dashes in UI copy or docs.

## Quality gates
- `go vet`, `staticcheck`, `go test ./...` pass
- New schema = new migration + CONTENT_MODEL.md update
- New public page = feed, sitemap, OG meta, and KV purge rules updated
- Every publish path must invalidate affected KV keys (item, lists, tags, feeds, stream, sitemap)

## Don't
- Don't add a JS framework or a CSS framework.
- Don't put secrets in code or `wrangler.jsonc`; use `wrangler secret`.
- Don't make public pages depend on a warm container.
- Don't trust the Google `hd` parameter alone; always check the ID token claims and email domain server-side.
- Don't serve presentation decks from the main origin.
