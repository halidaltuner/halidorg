# Phase 8 prompts: IndieWeb and distribution

Prerequisite: Phase 7 exit check is all "done".
Run each in a fresh session (`/clear`), in plan mode (Shift+Tab). One commit (or PR) per step.

## 8.1 Webmentions

```
Read CLAUDE.md, CONTENT_MODEL.md §1 (webmentions) and design/README.md (Admin Dashboard webmentions panel, Post webmentions).
Phase 8, step 1:
1. Receive endpoint per the Webmention spec: validate source and target asynchronously in a job, fetch the source with the same SSRF guard as Quick Capture, parse h-entry for author, avatar and content.
2. All received mentions start as pending; enable the dashboard moderation panel (approve/reject removes the row).
3. Render approved mentions on posts (avatar plus source), purge on approval.
4. Send webmentions on publish for outbound links; record results in job_runs.
5. Add h-entry and h-card microformats to post and about templates.
Propose a plan first, then wait for my approval.
```

## 8.2 POSSE to X and LinkedIn

```
Read CLAUDE.md and CONTENT_MODEL.md §1 (syndications).
Phase 8, step 2:
1. Editor side panel: "syndicate" section with X and LinkedIn toggles and an editable preview text; nothing posts automatically.
2. On confirm, a job posts via each API, stores the URL in syndications, and shows u-syndication links on the post.
3. Tokens in secrets; handle expired tokens with a clear dashboard warning.
4. Ask me before implementing each network; list the API access I need to request first.
Propose a plan first, then wait for my approval.
```

## 8.3 Newsletter (optional)

```
Read CLAUDE.md.
Phase 8, step 3: only if I confirm I want a newsletter.
1. Compare 2 or 3 email providers that work well from Cloudflare (cost, API, double opt-in, EU/TR data handling) in docs/newsletter.md.
2. After I pick one: signup form, double opt-in, unsubscribe, and a "send as newsletter" action on publish.
Propose a plan first, then wait for my approval.
```

## Phase 8 exit check

```
Check Phase 8 items in ROADMAP.md against prod. Send a test webmention from an external page, moderate it, and confirm it renders. Confirm a syndicated post shows its syndication links. Do not change code.
```
