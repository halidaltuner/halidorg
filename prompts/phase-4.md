# Phase 4 prompts: projects, homelab, GitHub

Prerequisite: Phase 2 exit check is all "done". Phase 3 (launch) is deferred to the end (ROADMAP.md), so checks that say prod run against staging.
Run each in a fresh session (`/clear`), in plan mode (Shift+Tab). One commit (or PR) per step.
Every prompt assumes `prompts/_constraints.md` (storage via internal/store, separate SEARCH DB, queue consumer Worker, routes() test).

## 4.1 Project schema (software + homelab)

```
Read CLAUDE.md and CONTENT_MODEL.md §2 (project, buildlog).
Phase 4, step 1:
1. Migration: projects (project_type software|homelab), project_facts, project_stack, project_parts, project_gallery, buildlogs; homelab categories seed (homelab, electronics, 3d print, woodwork, network).
2. internal/content/{project,buildlog}: model, repo, validate (homelab requires a homelab category), tests.
3. Admin forms for projects (not designed): facts, stack or parts (with price and currency), gallery ordering, build log entries.
4. Purge rules: project page, projects list, homelab list, home tiles and stream.
Propose a plan first, then wait for my approval.
```

## 4.2 Public: Projects + Project Detail

```
Read CLAUDE.md and design/README.md (Projects, Project Detail). Open design/Projects.dc.html and design/Project Detail.dc.html.
Phase 4, step 2:
1. Projects page pixel-close: project cards with status badge, mono name, stack pills, stars, activity, details link. GitHub section uses placeholder data until 4.4.
2. Project Detail pixel-close, one template for software and homelab: tone hero, fact strip overlapping the hero, article sections, gallery, sticky aside (stack or parts with prices), build log timeline.
3. Build log permalink /projects/{project}/log/{slug}.
Propose a plan first, then wait for my approval.
```

## 4.3 Public: Homelab & DIY

```
Read CLAUDE.md and design/README.md (Homelab & DIY). Open design/Homelab.dc.html.
Phase 4, step 3:
1. Homelab page pixel-close: orange hero, 4 stat tiles (builds computed; nodes, idle W, uptime from site_settings), featured build, category chips (?category=), build cards with cost and time.
2. Admin: edit homelab stats in settings.
3. Home: enable the homelab and projects tiles with real counts.
Propose a plan first, then wait for my approval.
```

## 4.4 GitHub sync

```
Read CLAUDE.md and CONTENT_MODEL.md §4 (GitHub).
Phase 4, step 4:
1. Cron (every 6h) enqueues a GitHub sync job, handled by the consumer Worker and forwarded to the container: repos, languages, releases, notable activity and 52 weeks of contribution counts via GraphQL. Token from Worker secrets.
2. Tables gh_repos, gh_activity, gh_weekly, gh_languages; upsert, never delete repos that are linked to projects.
3. Wire the Projects GitHub section: 52-week strip with 3 intensity buckets, language bar, pinned repos, releases.
4. Admin GitHub screen (not designed): last sync, run now, pin/unpin, link repo to project.
5. Handle rate limits and partial failures; log to job_runs. The GitHub token is a container env var: remember the DO restart caveat when rotating it.
Propose a plan first, then wait for my approval.
```

## 4.5 Stream + GitHub standalone pages

```
Read CLAUDE.md and CONTENT_MODEL.md §4 (Stream).
Phase 4, step 5:
1. D1 view for the stream (entries + gh_activity + radar_history placeholder), cursor pagination.
2. /stream page (not designed: Home stream pattern, 820px container, day groups, kind chips, htmx "load more").
3. /github standalone page (not designed: reuse the Projects GitHub section at full width).
4. Command palette prefixes pr: and h:.
Propose a plan first, then wait for my approval.
```

## Phase 4 exit check

```
Check Phase 4 items in ROADMAP.md against the repo and prod. Verify a GitHub sync ran in the last 6h, a software project and a homelab build render correctly, and stream pagination works without JS. Compare Projects, Project Detail and Homelab with design/*.dc.html and list visual differences. Do not change code.
```
