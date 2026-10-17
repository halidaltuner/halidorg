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
