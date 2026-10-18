# Environments

Three environments share one `wrangler.jsonc` (ARCHITECTURE.md §5). Each has its own D1, R2 and KV.

| Env | Worker | D1 | KV | R2 | Origin |
|---|---|---|---|---|---|
| local | `halidorg` (`wrangler dev`) | `halidorg-local` | `local-pages` | `halidorg-media-local` | `http://localhost:8787` |
| staging | `halidorg-staging` | `halidorg-staging` | `halidorg-staging-pages` | `halidorg-media-staging` | `https://staging.halid.org`, decks `https://decks-staging.halid.org` |
| prod | `halidorg-prod` | `halidorg-prod` | `halidorg-prod-pages` | `halidorg-media-prod` | `https://halidorg-prod.halid-09d.workers.dev` until the Phase 3 cutover |

Local resources need no creation: `wrangler dev` simulates them under `.wrangler/state`.

Presentation decks have their own origin (ADR-007, `DECKS_ORIGIN`): `http://127.0.0.1:8787` locally (a different origin from `localhost` on the same Worker), `decks-staging.halid.org` on staging, and `decks.halid.org` on prod, whose custom domain is added with the halid.org route at the Phase 3 cutover.

## 1. Create staging and prod resources

Run from `worker/` so the pinned wrangler is used. `--location` is a placement hint; `eeur` is closest to Istanbul.

```bash
cd worker

npx wrangler d1 create halidorg-staging --location eeur
npx wrangler d1 create halidorg-prod --location eeur

npx wrangler kv namespace create halidorg-staging-pages
npx wrangler kv namespace create halidorg-prod-pages

npx wrangler r2 bucket create halidorg-media-staging --location eeur
npx wrangler r2 bucket create halidorg-media-prod --location eeur
```

Each `d1 create` prints a `database_id`, each `kv namespace create` an `id`. Put them in `wrangler.jsonc`:

| Placeholder | Value from |
|---|---|
| `STAGING_D1_DATABASE_ID` | `d1 create halidorg-staging` |
| `PROD_D1_DATABASE_ID` | `d1 create halidorg-prod` |
| `STAGING_KV_NAMESPACE_ID` | `kv namespace create halidorg-staging-pages` |
| `PROD_KV_NAMESPACE_ID` | `kv namespace create halidorg-prod-pages` |

R2 is bound by bucket name, so it needs no id. Then apply the schema:

```bash
npx wrangler d1 migrations apply DB --remote --env staging -c ../wrangler.jsonc
npx wrangler d1 migrations apply DB --remote --env prod -c ../wrangler.jsonc
```

The search index lives in its own database (ADR-010), binding `SEARCH`,
migrations in `migrations-search/`. Created on 2026-10-18:

| Env | Database | `database_id` |
|---|---|---|
| staging | `halidorg-search-staging` | `d6c10762-47ac-4a2e-869e-4466f69ddf39` |
| prod | `halidorg-search-prod` | `e0381f89-ed14-40f3-ac58-36e06c29d9ae` |

```bash
npx wrangler d1 migrations apply SEARCH --remote --env staging -c ../wrangler.jsonc
npx wrangler d1 migrations apply SEARCH --remote --env prod -c ../wrangler.jsonc
```

It is never exported or backed up: a rebuild job recreates it from `DB`.

### Job queues (Phase 1.3)

Each env has a jobs queue and a dead-letter queue (`wrangler.jsonc`, `queues`).
Create them once before the first deploy that carries the `queues` block:

```bash
npx wrangler queues create halidorg-jobs-staging
npx wrangler queues create halidorg-jobs-dlq-staging
npx wrangler queues create halidorg-jobs-prod
npx wrangler queues create halidorg-jobs-dlq-prod
```

The API token used by CI needs **Account: Queues: Edit** to deploy a consumer.

## 2. Secrets

| Secret | Value |
|---|---|
| `GOOGLE_CLIENT_ID` | OAuth client id (section 3) |
| `GOOGLE_CLIENT_SECRET` | OAuth client secret |
| `SESSION_SECRET` | at least 32 random bytes: `openssl rand -base64 48` |
| `GITHUB_TOKEN` | GitHub sync (Phase 4.4): a fine-grained token on the GitHub account, repository access "Public repositories (read-only)", no other permissions. Optional: without it the sync job ends at once and `/admin/github` says so. |

`wrangler secret put` reads the value from a prompt, so it never lands in shell history:

```bash
npx wrangler secret put GOOGLE_CLIENT_ID --env staging -c ../wrangler.jsonc
npx wrangler secret put GOOGLE_CLIENT_SECRET --env staging -c ../wrangler.jsonc
npx wrangler secret put SESSION_SECRET --env staging -c ../wrangler.jsonc
npx wrangler secret put GITHUB_TOKEN --env staging -c ../wrangler.jsonc
```

The Worker hands most secrets to the container when its Durable Object starts, so a secret added or rotated later waits for a restart (docs/spike-phase0.md). `GITHUB_TOKEN` does not: the Worker sends it with each job delivery, so it takes effect at the next sync ("run now" on `/admin/github`).

Prod gets the same three with `--env prod` once its OAuth client exists (Phase 3). Use a different `SESSION_SECRET` per env: it keys the session ids, so sharing it would let a staging row hash match prod.

Locally, put the same names in `.dev.vars` at the repo root (gitignored):

```
GOOGLE_CLIENT_ID=...
GOOGLE_CLIENT_SECRET=...
SESSION_SECRET=...
```

Without them the login page renders, and "Continue with Google" fails closed with the generic error banner.

## 3. Google OAuth client

In Google Cloud Console, create an OAuth client of type **Web application** in a project owned by the halid.org Workspace:

- Authorized redirect URIs: `http://localhost:8787/admin/auth/callback`, `https://staging.halid.org/admin/auth/callback`. Prod adds `https://halid.org/admin/auth/callback` at cutover.
- Audience: **Internal**, so only halid.org accounts can complete consent. The server checks `hd` and the email domain regardless (ADR-005).
- **Enable `auth_time`** for the client. Re-auth for publish and delete asks Google for `auth_time` and refuses without it, so with this off every publish loops back to Google.

Re-auth is unverified against Google: whether `max_age=0` makes Google ask for the password again, rather than returning the existing session's `auth_time`, needs a live test on staging. If Google returns an old `auth_time`, re-auth fails closed with the generic banner.

## 4. CI

`.github/workflows/ci.yml` runs on pull requests; `deploy.yml` deploys staging on `main`, applying D1 migrations before `wrangler deploy`. Prod deploys only from a manual run (Actions → deploy → Run workflow → `prod`); halid.org stays on its "coming soon" page until the project is done. `v*` tags mark milestones (`v0.1.0` = Phase 1) and deploy nothing.

Repository secrets: `CLOUDFLARE_API_TOKEN`, `CLOUDFLARE_ACCOUNT_ID`.

Create the token from the **Edit Cloudflare Workers** template, then trim it to the halid.org account and zone:

| Scope | Permission | Why |
|---|---|---|
| Account | Workers Scripts: Edit | upload the Worker |
| Account | Containers: Edit | push the image and update the container application |
| Account | D1: Edit | `d1 migrations apply --remote` |
| Account | Workers KV Storage: Edit | the deploy validates the KV binding |
| Account | Workers R2 Storage: Edit | the deploy checks the bucket exists; Read may be enough |
| Account | Account Settings: Read | wrangler reads account details |
| Zone (halid.org) | Workers Routes: Edit | the `staging.halid.org` and `decks-staging.halid.org` custom domains |

The template's Workers Routes, KV and Account Settings rows come from Cloudflare. The Containers, D1 and R2 rows are inferred from what the deploy touches, not taken from a published minimum, so the first staging deploy is the test: a missing permission fails with `Authentication error [code: 10000]` and names the API call.

## 5. Cloudflare Access on staging

Staging sits behind Access as a second layer in front of the app's own Google login (ADR-005). Set up in the Zero Trust dashboard (team domain `halid-staging.cloudflareaccess.com`, Free plan) on 2026-10-17. The login page shows the organization name `halid.org` (Reusable components > Custom pages > Access login page).

- Application `halidorg staging`, self-hosted, destination `staging.halid.org`, session 24 hours.
- Policy `halid.org accounts only`: Allow, include "Emails ending in" `@halid.org`.
- Login method: One-time PIN (no identity provider configured). Access mails a code to the address you enter, and only `@halid.org` addresses get one.

Every path is covered, `/healthz` included. CI deploys are unaffected because they go through the Cloudflare API. Scripts that call staging, such as latency probes, need an Access service token sent as `CF-Access-Client-Id` and `CF-Access-Client-Secret`, plus a policy that allows it.

Prod has no Access application; adding one later needs no code change.

`decks-staging.halid.org` is outside the Access application on purpose: the site embeds decks in an iframe, and an Access login on another host would need a third-party cookie there. It serves only uploaded deck files under unguessable entry ids.
