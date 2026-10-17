# Phase 0 spike: container access to D1, R2 and KV

Decision for ADR-001's open question. Status: **path chosen, proven locally and on staging; cold start about 3 s from one sample.**

## Decision

The container reaches the stores through **method B, outbound interception**. It calls `http://bridge.internal/{d1,kv,r2}/...`, and the Worker's `App.outboundByHost` handler answers with the Worker's own bindings (`worker/src/bridge.ts`). The Go client is `app/internal/store`.

| Method | How | Why not |
|---|---|---|
| A, direct binding | bindings inside the container | Containers have no binding surface. |
| **B, interception** | `bridge.internal`, handled in the Worker | Chosen: no public route, no secret, stays inside Cloudflare. |
| C, public bridge | `/__bridge/*`, HMAC-signed | A route that runs arbitrary SQL would be reachable from the internet. |
| D, REST API | Cloudflare API with a token | Every call leaves Cloudflare, needs an account token in the container, and R2 needs SigV4. |

## Bridge protocol

| Request | Body | Response |
|---|---|---|
| `POST /d1/query` | `{"sql", "params"}` | `{"results", "changes", "last_row_id"}` |
| `POST /d1/batch` | `{"statements": [{"sql", "params"}]}` | array of the above, one D1 transaction |
| `GET/PUT/DELETE /kv/{key}` | value; `X-KV-Metadata` (JSON), `X-KV-TTL` (s) | value with `X-KV-Metadata` |
| `GET/PUT/DELETE /r2/{key}` | object; `Content-Type` | object with `Content-Type`, `ETag` |

Keys are path-escaped as one segment, so `page:en:/en/` survives. A missing key is 404, which the Go client maps to `store.ErrNotFound`. D1 errors are 500 with SQLite's message.

## Measured locally (`wrangler dev`, 2026-10-17)

The spike's probe endpoint, `GET /x/spike/bridge?method=B&n=20`, through `wrangler dev` with the local container on an Apple Silicon laptop:

| Op | ok / fail | p50 ms | p95 ms |
|---|---|---|---|
| d1_select | 20 / 0 | 0.87 | 2.15 |
| kv_get | 20 / 0 | 0.60 | 0.86 |
| kv_put | 20 / 0 | 1.19 | 1.60 |
| r2_get | 20 / 0 | 0.71 | 0.90 |
| r2_put | 20 / 0 | 1.24 | 2.05 |

These numbers are the bridge's own overhead against simulated stores. They say nothing about edge latency.

Also run through `wrangler dev`: `/en/` returns `X-Cache: MISS` then `HIT`; `/admin` with an unknown session cookie goes container to bridge to D1 and returns 303 to the login page.

## Measured on staging (2026-10-17)

`staging.halid.org`, `lite` instance, requests from Istanbul. `GET /x/spike/bridge?method=B&n=20`:

| Op | ok / fail | p50 ms | p95 ms |
|---|---|---|---|
| d1_select | 20 / 0 | 45.3 | 97.0 |
| kv_get | 20 / 0 | 8.8 | 11.7 |
| kv_put | 20 / 0 | 37.8 | 104.6 |
| r2_get | 20 / 0 | 52.2 | 69.2 |
| r2_put | 20 / 0 | 176.3 | 224.5 |

KV page cache, `GET /en/` six times: the first was `X-Cache: MISS` (`kv;dur=75, origin;dur=29`), the next five `HIT` with `kv;dur` 3 to 24 ms. Total time seen from Istanbul was 220 to 290 ms, most of it the network round trip.

Cold start, about 3 s (one sample): idle waits never caught one, because the container does not go to sleep after `sleepAfter` (5m). Twice, after 6.5 minutes with no traffic, `wrangler containers list` still showed 1 live instance and a KV miss on `/en/` came back with `origin;dur=36` and `41`. What did catch one was polling `/admin/login` once a second through the deploy of `b4ca9f5`: at 19:05:51Z, 14 s before the container application was updated, one request took 3.27 s against 0.22 to 0.29 s warm. The rest of the six-minute window stayed under 1.3 s. Public pages are unaffected either way, since KV hits never reach the container.

**Trap: an image rollout keeps the old environment.** After that deploy the new code was running (`/x/spike/bridge` was gone) but the login page still showed the previous `DEPLOY_SHA`. `App` sets `envVars` in its Durable Object constructor, and the rollout replaced the container without restarting the Durable Object. A rotated secret (`wrangler secret put`) can therefore stay unused until the Durable Object restarts; check before relying on a rotation.

## Two traps found on the way

1. **`ContainerProxy` must be exported** from the Worker entrypoint, or the container fails to start once `outboundByHost` is set.
2. **`static outboundByHost = {...}` silently does nothing** with ES2022+ class fields: it defines an own property instead of calling `Container`'s static setter, so the handler is never registered and the call falls through to the internet as `internal error`. `worker/src/index.ts` assigns it in a `static {}` block.

Both were in the spike code, so method B had never actually run before this.

## Not measured yet

- cold start beyond one sample
- monthly cost at expected traffic

The probe code (`internal/spike`, `/x/spike/bridge`, the public `/__bridge` route and the method C and D clients) was removed after these measurements; it is in git history before that commit.
