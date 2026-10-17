# Phase 0 prompts for Claude Code

Run each in a fresh session (`/clear` between them), in plan mode (Shift+Tab). Commit after each.

## 0.1 Scaffold + runtime spike

```
Read CLAUDE.md, README.md, ARCHITECTURE.md, CONTENT_MODEL.md and ROADMAP.md.
We are starting Phase 0. Scope for this session only:
1. Scaffold the repo layout from ARCHITECTURE.md §4 (worker/ in TypeScript, app/ in Go, migrations/, wrangler.jsonc, Dockerfile).
2. Edge Worker: route all requests; on GET check KV for a rendered page, else forward to the Go container and store the response in KV.
3. Go app: net/http server with a single "hello" page rendered by html/template, plus /healthz.
4. Spike: verify how the container reaches D1/R2/KV (direct binding vs Worker bridge). Write findings and cold-start numbers to docs/spike-phase0.md and update ADR-001 if needed.
Do not implement auth yet. Propose a plan first, then wait for my approval.
```

## 0.2 Data layer + staging environment

```
Read CLAUDE.md and docs/spike-phase0.md.
Phase 0, step 2:
1. Set up D1 migrations in /migrations with wrangler d1 migrations. Add 0001_core.sql with only site_settings and admin_sessions from CONTENT_MODEL.md (the rest comes in Phase 1).
2. Define local, staging and prod environments in wrangler.jsonc (separate D1, R2, KV per env).
3. Implement the D1/R2/KV access path chosen in the spike as a small Go package (internal/store) with tests.
4. Give me the exact wrangler commands to create the staging and prod resources; I will run them myself.
Propose a plan first, then wait for my approval.
```

## 0.3 Google SSO + Admin Login

```
Read CLAUDE.md, ARCHITECTURE.md ADR-005, design/README.md (Admin Login section) and open design/Admin Login.dc.html.
Phase 0, step 3:
1. Google OIDC login: authorization code + PKCE, hd=halid.org hint, server-side checks for ID token signature, aud, iss, hd == "halid.org", @halid.org email and email_verified.
2. Sessions in admin_sessions: opaque cookie (Secure, HttpOnly, SameSite=Lax), 12h lifetime, last_reauth_at, and a re-auth middleware for publish and delete (10 minutes).
3. Admin Login screen pixel-close to the prototype: default, loading and non-halid error states. Create tokens.css with only the tokens this screen needs.
4. A protected /admin stub that shows the signed-in email and a logout button.
5. Secrets: GOOGLE_CLIENT_ID, GOOGLE_CLIENT_SECRET, SESSION_SECRET via wrangler secret. List the commands; I will set them.
6. Tests for every claim check, including a gmail.com account and a missing hd claim.
Propose a plan first, then wait for my approval.
```

## 0.4 CI/CD

```
Read CLAUDE.md and ARCHITECTURE.md §5.
Phase 0, step 4: GitHub Actions.
1. On PR: go vet, staticcheck, go test ./..., TypeScript typecheck for worker/, docker build.
2. On push to main: deploy to staging. On tag v*: deploy to prod.
3. Use a Cloudflare API token from GitHub secrets (CLOUDFLARE_API_TOKEN, CLOUDFLARE_ACCOUNT_ID); list the minimum token permissions needed.
4. Commit the /design handoff if it is not committed yet, and check off finished Phase 0 items in ROADMAP.md.
Propose a plan first, then wait for my approval.
```

## Phase 0 exit check

```
Check Phase 0 exit criteria in ROADMAP.md against the repo and staging. For each item say done, partial or missing, with evidence (file, command output or URL). Do not change code.
```
