# Design

## 1. Identity

- **Brand mark:** existing "Kromatik" chromatic aberration wordmark (light + dark SVG variants).
- **Mood:** terminal-precise, engineering notebook, quietly technical. Dark navy and cyan as the core, with a red/cyan split echoing the wordmark used sparingly.
- **Voice in UI:** short labels, lowercase nav, no marketing tone.

## 2. Design tokens

### Color

| Token | Light | Dark | Use |
|---|---|---|---|
| `--bg` | `#F7F8FA` | `#0B1220` | page background |
| `--surface` | `#FFFFFF` | `#111A2E` | cards, panels |
| `--surface-2` | `#EEF1F5` | `#17223A` | code blocks, inputs |
| `--border` | `#DCE1E8` | `#24304A` | dividers |
| `--text` | `#0E1525` | `#E6EBF2` | body |
| `--text-muted` | `#5A6578` | `#8D98AB` | meta, captions |
| `--accent` | `#0891B2` | `#22D3EE` | links, focus, primary actions |
| `--accent-2` | `#E11D48` | `#FB7185` | aberration split, "hold" ring, destructive |
| `--success` | `#15803D` | `#4ADE80` | adopt, published |
| `--warning` | `#B45309` | `#FBBF24` | trial, scheduled |
| `--info` | `#4F46E5` | `#A5B4FC` | assess, draft |

Kind colors (small dot or left rule only, never full backgrounds): post `accent`, note `info`, snippet `warning`, link `text-muted`, fav `accent-2`, project `success`.

Theme switch: system default, manual toggle stored in `localStorage` and `data-theme` on `<html>`.

### Typography

| Role | Font | Size / line-height |
|---|---|---|
| UI + headings | Inter (or IBM Plex Sans) | |
| Body (long-form) | same sans, 18px / 1.7, max 68ch | |
| Code + meta | JetBrains Mono (or IBM Plex Mono) | 15px / 1.6 |
| Scale | 14 / 16 / 18 / 22 / 28 / 36 / 48 | modular ~1.25 |

Meta lines (dates, tags, reading time) are set in mono, muted, uppercase-free.

### Spacing, radius, motion
- Spacing: 4px base, scale `4 8 12 16 24 32 48 64 96`
- Radius: `4px` inputs/code, `8px` cards. No pills except tags.
- Shadows: none in dark mode, a single soft shadow in light mode.
- Motion: 120 to 180ms ease-out, `prefers-reduced-motion` respected. Signature effect: wordmark aberration shifts 1 to 2px on hover only.

## 3. Layout

- Content column 68ch, wide column 1100px for lists, radar and GitHub.
- Header: wordmark left, nav `posts notes snippets links projects radar now`, search (`⌘K`), language toggle `tr/en`, theme toggle.
- Footer: feeds, GitHub, X, LinkedIn, last deploy hash in mono.
- Mobile: 16px gutters, nav collapses into a sheet, no horizontal scroll.

## 4. Public components

| Component | Notes |
|---|---|
| **Entry card** | kind dot, title, summary (2 lines), mono meta (date, tags, reading time) |
| **Stream row** | compact one-liner per item, icon per kind, grouped by day |
| **Post page** | title, mono meta, sticky TOC on wide screens, footnotes as side notes on wide, series nav, backlinks block, webmentions |
| **Note** | card without title emphasis, timestamp as permalink |
| **Snippet** | file tabs, language badge, copy button, line numbers toggle, "raw" link |
| **Link list** | grouped by category, favicon, domain in mono, my comment below, read status chip |
| **Fav grid** | filter chips by type, optional rating as 5 small squares (not stars) |
| **Project card** | status badge, stack chips, repo stars, last activity, link to build logs |
| **GitHub overview** | pinned repos grid, language bar, 52-week activity strip, releases list |
| **Tech Radar** | SVG radar, 4 quadrants, 4 rings, blips numbered; side list grouped by ring; new = filled triangle, moved = arrow; hover/focus syncs list and blip; edition switcher |
| **Now page** | big "updated N days ago" mono line, sections, archive link |
| **Callouts** | `note`, `tip`, `warning`, `danger` with left rule in token color |
| **Code block** | chroma theme matched to tokens, filename header, copy, wrap toggle |
| **Mermaid** | rendered in `--surface`, theme variables mapped to tokens |
| **Command palette** | `⌘K`, fuzzy search across all kinds, kind filter prefix (`s:` snippets, `l:` links) |
| **Language switch** | goes to paired translation, or shows "not translated yet" with link to the other language |

## 5. Admin UI

Same tokens, denser: 14px base, compact tables.

### Layout
- Left sidebar: Dashboard, Content (per kind), Categories, Tags, Media, Radar, GitHub, Jobs, Redirects, Import, Settings.
- Top bar: quick capture `+`, global search, environment badge (`staging` in warning color).

### Key screens
| Screen | Notes |
|---|---|
| **Dashboard** | drafts, scheduled, recent publishes, failed jobs, 404s, webmentions to moderate |
| **Entry list** | per kind, filters (status, lang, tag, category), bulk actions, translation status column |
| **Editor** | split view: Markdown (CodeMirror 6) left, htmx live preview right; side panel for kind fields, tags, cover, SEO, translation link; autosave draft revisions; `⌘S` save, `⌘⇧P` publish |
| **Quick capture** | single mobile form: paste URL → fetch title/description → choose link/fav/note → save |
| **Revisions** | timeline + side-by-side diff, restore |
| **Media** | grid, drag-drop upload, alt text TR/EN required before use |
| **Radar** | table editor with ring dropdown, drag on radar to move blips, edition snapshot button |
| **Import** | upload Ghost export, dry-run report, confirm |
| **Jobs** | runs table, retry, logs |

### Interaction rules
- htmx for all partial updates; full reload never needed during editing.
- Destructive actions use `--accent-2` and require typed confirmation for entries with backlinks.
- Every form works without JS for basic save.

## 6. Accessibility
- WCAG 2.1 AA contrast on both themes (verify cyan on navy for small text, use `--text` for body).
- Visible focus ring: 2px `--accent` outline + 2px offset.
- Radar fully usable from the list view and keyboard.
- `lang` attribute per page and per mixed-language block.

## 7. Assets
- Wordmark SVG light/dark, favicon (monogram with aberration), OG template 1200×630: dark navy, title in sans, kind + date in mono, subtle red/cyan offset on the wordmark.
