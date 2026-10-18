# Known constraints (from Phase 0 spike)

Every prompt that touches Worker, container, storage or routes should be read together with this file. Details live in `docs/spike-phase0.md` and ARCHITECTURE.md (ADR-001, ADR-005, ADR-010).

## Storage access
- The container has no Cloudflare bindings. All D1/KV/R2 access from Go goes through `app/internal/store` (Worker bridge at `http://bridge.internal/{d1,kv,r2}/...`, answered by the Worker via `outboundByHost`).
- Use `store.DB` (`Query`, `Batch` for a single D1 transaction), `store.KV` (`Get`, `Put` with metadata/TTL, `Delete`), `store.Blob` (`Get`, `Put`, `Delete`), and `store.Scan[T]` for rows. Never call Cloudflare REST APIs from the container.
- Typical latency (p50): D1 select 45ms, KV get 9ms, KV put 38ms, R2 get 52ms, R2 put 176ms. Batch writes; avoid N+1 queries.

## Search (ADR-010)
- FTS5 lives in a **separate** D1 database `halidorg-search-{env}` (binding `SEARCH`), with its own migrations directory (`migrations-search/`). Never put virtual tables in the main DB (D1 export would break).
- Tokenizer `unicode61 remove_diacritics 2`. Fold Turkish ı/İ/I to `i` in Go at both index and query time. Prefix search, no stemming.
- The Worker queries `SEARCH` directly. Index rows are updated per entry on publish/unpublish.

## Worker / container pitfalls
1. Assign `outboundByHost` handlers inside a `static {}` block, not as `static outboundByHost = {...}` (ES2022 class fields skip the setter). `export { ContainerProxy }` is required.
2. Container env vars are set in the Durable Object constructor. A new image does not restart the DO, so new secrets or `DEPLOY_SHA` may not appear immediately. Plan rollouts accordingly.
3. Go ServeMux: patterns like `GET /static/` can conflict with `/{lang}/{$}` and panic. Every change that adds routes must pass the `routes()` test in `cmd/server/main_test.go`.
4. `/{lang}/literal` patterns overlap `/lang/{lang}` (both match `/lang/literal`) and panic. Register language routes with literal prefixes per supported language.

## Jobs
- The container cannot consume Queues. A **consumer Worker** receives messages and forwards each job to the container over the internal path. Queue bindings are defined per env in `wrangler.jsonc`.
- Cron triggers enqueue; they never call the container directly.

## Auth (ADR-005)
- Re-auth uses `max_age=0` + `claims={"id_token":{"auth_time":{"essential":true}}}` (Google does not support `prompt=login`). Missing `auth_time` fails closed.
- Middlewares: `auth.RequireSession`, `auth.RequireRecentAuth`. `admin_sessions.id` is the HMAC of the cookie token.

## Environments
- Staging is behind Cloudflare Access (@halid.org, One-time PIN). Scripts and checks against staging need a service token.
- Prod runs at `halidorg-prod.halid-09d.workers.dev`. No `halid.org` route and prod Google login disabled until Phase 3 cutover.

## i18n
- Language: `lang` cookie > `Accept-Language`, set via `/lang/{tr|en}?next=`. `partials/lang_switch.html` and `i18n.FromRequest` exist. No localStorage.

## Fonts
- Web: self-hosted woff2 (latin + latin-ext) already in `web/static/fonts`.
- Go `opentype` cannot read woff2. Server-side rendering (OG images) needs TTF/OTF copies of IBM Plex Sans and JetBrains Mono.
