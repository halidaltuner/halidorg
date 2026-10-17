# halid.org engine

A personal publishing engine for halid.org. Not just a blog: posts, notes, snippets, links, favs, software and homelab projects, presentations, photos, certificates, a personal Tech Radar and more, all managed from an interactive admin panel and served from Cloudflare.

## Docs

| File | What it covers |
|---|---|
| [ROADMAP.md](ROADMAP.md) | Phased delivery plan, milestones, exit criteria |
| [ARCHITECTURE.md](ARCHITECTURE.md) | Runtime topology on Cloudflare, request paths, key decisions (ADRs) |
| [CONTENT_MODEL.md](CONTENT_MODEL.md) | Per-type schemas, shared fields, D1 tables, relations |
| [DESIGN.md](DESIGN.md) | Tokens, components, screen-to-template map, undesigned screens |
| [design/](design/) | Claude Design handoff: high-fidelity `.dc.html` prototypes + screen README (visual source of truth) |
| [CLAUDE.md](CLAUDE.md) | Working rules for Claude Code on this repo |

## Stack at a glance

- **Language:** Go (server-rendered `html/template` + htmx, minimal JS)
- **Runtime:** Cloudflare Workers (edge, read path) + Cloudflare Containers (Go app: admin, rendering, jobs)
- **Data:** D1 (relational + FTS), R2 (media), KV (rendered page cache), Queues / Cron Triggers (jobs)
- **Auth:** Google SSO (OIDC) restricted to @halid.org, verified server-side
- **UI:** light theme, IBM Plex Sans + JetBrains Mono, section tones, `halid.` aberration wordmark
- **Content source of truth:** D1. Markdown is the authoring format, HTML is rendered on publish.

## Principles

1. Every content type is first-class, with its own schema, page, and feed.
2. Public pages are fast without depending on a warm container.
3. Admin is the only write path. No git-based publishing.
4. Bilingual (TR/EN) by design, not bolted on.
5. Own the data: everything exportable as Markdown + JSON at any time.
