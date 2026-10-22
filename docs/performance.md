# Performance budget

Status: Phase 7.5 (2026-10-22). ROADMAP: LCP under 1.2 s, under 30 KB of
JavaScript on content pages.

## What is enforced

- **JS weight, in CI.** `TestJSBudget` (`app/internal/public/budget_test.go`)
  renders every public page type with content and adds up the files its
  `<script>` tags load. More than 30 KB uncompressed fails the build. Mermaid's
  chunks load on demand near a diagram and are not counted (docs/mermaid.md);
  the admin is not a content page.
- **LCP and script transfer, on staging.** `scripts/staging-check.mjs` runs
  Lighthouse 12.8.2 (`scripts/lighthouse.mjs`) on home, the newest post and
  the projects list after the other checks, when the pages are warm in KV.
  Desktop is judged: LCP under 1200 ms and under 30 KB of script
  transferred. Mobile (Lighthouse's slow 4G and mid-range phone) is
  reported only. Each run's HTML report lands next to the screenshots on
  `checks/staging`.

## Measured

`TestJSBudget`, uncompressed / gzip -9 (2026-10-22):

| page | scripts | bytes | gzip |
|---|---|---|---|
| most lists, a post | header, code, loader, palette (+ post) | 10.4 to 11.1 KB | 4.4 to 4.8 KB |
| with filters | + filter (+ links, photos, snippet, radar) | 11.8 to 14.1 KB | 5.0 to 6.1 KB |

Lighthouse against the local server (no CDN, no compression), desktop:
home and a long post LCP 640 ms, CLS 0, TBT 0, score 100. Mobile: LCP
2.7 s, score 94; the two stylesheets and the first font are the critical
path under the slow-4G model.

## Changes in 7.5

- The text fonts the first paint needs (Plex Sans 400 and 600, JetBrains
  Mono 400, latin) are preloaded on public pages.
- The hero cover of a post, a project, a presentation and home's featured
  post loads at once with `fetchpriority="high"`; card covers stay lazy.

## Not done

- Inlining the critical CSS: the mobile LCP would gain most from it, but
  the pages are served from KV with the stylesheets cached for a year
  (content-hashed), so repeat views do not pay for them. Revisit if the
  mobile numbers on staging say so.
