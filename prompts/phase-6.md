# Phase 6 prompts: Tech Radar

Prerequisite: Phase 5 exit check is all "done".
Run each in a fresh session (`/clear`), in plan mode (Shift+Tab). One commit (or PR) per step.

## 6.1 Radar schema + editions

```
Read CLAUDE.md and CONTENT_MODEL.md §2 (radar_item).
Phase 6, step 1:
1. Migration: radar_items, radar_history, radar_editions.
2. Rules: every ring change writes radar_history with the current edition; "new" means first appearance in an edition; "moved" means the ring differs from the previous edition.
3. Edition snapshot: freezing an edition copies the current ring of every item into radar_history for that edition.
4. Feed radar changes into the stream view.
5. Tests for new / moved / unchanged across three editions.
Propose a plan first, then wait for my approval.
```

## 6.2 Public radar

```
Read CLAUDE.md and design/README.md (Radar). Open design/Radar.dc.html.
Phase 6, step 2:
1. Server-rendered SVG 500x500 pixel-close: quadrants (languages top-right, techniques top-left, tools bottom-left, platforms bottom-right), rings at r 60/120/180/240 with tints, blips (circle unchanged, filled triangle new, dashed line moved).
2. Deterministic blip placement (seeded by item id) so positions don't jump between renders; no overlaps within a ring segment.
3. Side list grouped by ring; hover or focus syncs blip and row; active row expands its note. A small JS island only for the sync.
4. Edition switcher (?edition=2026-10), each edition is its own cached page.
5. Fully usable from the list and keyboard; SVG has a text alternative.
6. /radar/{slug} item page with history across editions.
Propose a plan first, then wait for my approval.
```

## 6.3 Admin radar editor

```
Read CLAUDE.md and DESIGN.md §7.
Phase 6, step 3 (not designed: derive from dashboard density):
1. Table editor: name, quadrant, ring dropdown, note TR/EN, inline save via htmx.
2. Optional drag on a radar preview to change ring; the table stays the source of truth.
3. "Freeze edition" button with a confirm dialog and a diff of changes since the last edition.
Propose a plan first, then wait for my approval.
```

## Phase 6 exit check

```
Check Phase 6 items in ROADMAP.md against the repo and prod. Verify new and moved markers between two editions, keyboard-only navigation of the list, and that blip positions are stable across re-renders. Compare the radar with design/Radar.dc.html and list visual differences. Do not change code.
```
