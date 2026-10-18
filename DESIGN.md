# Design

**Source of truth:** the high-fidelity prototypes in [`/design`](design/) (Claude Design handoff, 2026-10-17). Screen-level detail lives in [`design/README.md`](design/README.md). This file is the production summary: tokens, components, the screen-to-template map, and what is still undesigned. When this file and a prototype disagree, the prototype wins and this file gets updated.

## 1. Identity

- **Wordmark:** `halid` in JetBrains Mono 700, letter-spacing −0.03em, followed by a `.` in `#E11D48`. Text-only until the SVG exists.
- **Chromatic aberration:** light bg `text-shadow: -1.5px 0 rgba(225,29,72,.7), 1.5px 0 rgba(8,145,178,.7)`; dark bg `rgba(251,113,133,.8)` / `rgba(34,211,238,.8)`; hover grows the offset to 2.5px.
- **Wordmark sizes:** 24 header, 28 footer, 32 login, 44 OG.
- **Tri-stripe:** 3px bar above header and footer, `linear-gradient(90deg, #E11D48 0 33%, #0891B2 33% 66%, #4F46E5 66%)`.
- **Mood:** engineering notebook, colorful but precise. Each section owns a tone.
- **Voice:** short lowercase labels, mono for meta, no marketing tone.

## 2. Theme

**Light only.** No theme toggle, no `prefers-color-scheme` switch. Dark is used as **bands**, not as a theme: Home, About, Now and Presentation Detail heroes, Footer, Admin Login, OG image, dark code blocks.

## 3. Tokens (`web/static/css/tokens.css`)

### Base
| Token | Hex | Use |
|---|---|---|
| `--bg` | `#F7F8FA` | page |
| `--surface` | `#FFFFFF` | cards, header |
| `--surface-2` | `#EEF1F5` | inputs, inline code, chips |
| `--border` | `#DCE1E8` | dividers, card borders |
| `--border-soft` | `#EEF1F5` | row dividers inside cards |
| `--text` | `#0E1525` | body, headings |
| `--text-muted` | `#5A6578` | meta, captions |
| `--link` | `#0E7490` | small text links (AA on white) |
| `--accent` | `#0891B2` | focus ring, hover, highlights. Not for small text |
| `--accent-2` | `#E11D48` | aberration, destructive, radar hold |
| `--success` / `--warning` / `--info` | `#15803D` / `#B45309` / `#4F46E5` | statuses |

### Dark band
`--dk-bg #0B1220` · `--dk-surface #111A2E` · `--dk-surface-2 #17223A` · `--dk-border #24304A` · `--dk-text #E6EBF2` · `--dk-muted #8D98AB` · `--dk-cyan #22D3EE` · `--dk-rose #FB7185` · `--dk-green #4ADE80` · `--dk-amber #FBBF24` · `--dk-indigo #A5B4FC` · `--dk-cyan-hover #67E8F9` (link hover on dark, from Admin Login).
Grid texture: two 1px `#17223A` linear-gradients, 48px cell, opacity .6 (.7 on Admin Login, per the prototype).
Google sign-in mark: `--google-red #EA4335` · `--google-yellow #FBBC05` · `--google-green #34A853` · `--google-blue #4285F4`. Brand colors for that button only, not palette.

### Section tones (`--tone-{name}-tint|ink|stripe`)
| Tone | Tint | Ink | Stripe | Sections |
|---|---|---|---|---|
| cyan | `#D7F1F7` | `#0E7490` | `#C4E9F2` | posts, certificates |
| indigo | `#E4E3FC` | `#4338CA` | `#D5D3FA` | notes, radar, learning |
| amber | `#FCEBCF` | `#92400E` | `#F8DDB0` | snippets, reading |
| green | `#D9F0E0` | `#166534` | `#C5E7CF` | projects |
| orange | `#FDE3D3` | `#9A3412` | `#FAD0B6` | homelab & DIY |
| rose | `#FCE1E7` | `#BE123C` | `#F9CCD6` | presentations |
| violet | `#EBE2FD` | `#6D28D9` | `#DDCFFB` | photos |
| navy | `#DCE3EF` | `#0E1525` | `#CBD5E6` | neutral |

Nav underline: posts `#0891B2`, projects `#15803D`, homelab `#C2410C`, presentations `#BE123C`, photos `#6D28D9`, radar `#4F46E5`, now `#0891B2`, about `#0E1525`.

Kind glyphs (28×28 tile, tint bg, ink text): post `¶`, note `~`, snippet `{}`, link `↗`, fav `*`, project `◆`, homelab `⌁`.
Kind dots (more menu squares, palette rows; from `Header.dc.html`): post `#0891B2`, note `#4F46E5`, snippet `#B45309`, link `#5A6578`, fav `#E11D48`, project `#15803D`, homelab `#C2410C`, talk `#BE123C`, certificate `#0E7490`, page `#0E1525`.

### Typography
- **Sans:** IBM Plex Sans 400/500/600. **Mono:** JetBrains Mono 400/500/700. Self-hosted (woff2) in production.
- **Scale:** 11 / 12 / 13 / 14 / 15 / 16 / 18 / 20 / 22 / 24 / 28 / 36 / 40 / 48 / 56.
- **Hero H1:** 48/1.08, 600, −0.025em, `text-wrap: balance`. **H2:** 28/1.3, 600, −0.01em. **Card title:** 20 to 22 / 1.3, 600.
- **Prose:** 18/1.7, max 68ch, `text-wrap: pretty`.
- **Meta, eyebrows, chips, dates:** mono 11 to 13, muted. Eyebrows prefixed with the section glyph or `//`, in section ink.

### Space, radius, shadow, motion
- Spacing: 4 · 8 · 12 · 16 · 20 · 24 · 32 · 40 · 48 · 56 · 64 · 72 · 96. Gutters 24px (16px under 480px).
- Containers: 1200 default, 1100 Now, 1240 Post with TOC, 820 to 860 Links and stream, 68ch prose.
- Radius: 4 (inputs, buttons, code, badges), 8 (cards, panels), 999 (chips and tag pills only).
- Shadows: card `0 1px 3px rgba(14,21,37,.06)`; card hover `0 10px 28px rgba(14,21,37,.10)` + `translateY(-2px)`; hero media `0 16px 40px rgba(14,21,37,.14)`; popover `0 12px 32px rgba(14,21,37,.12)`.
- Motion: 120 to 180ms ease-out, `prefers-reduced-motion` respected.
- Focus: `2px solid #0891B2`, offset 2px.

## 4. Shared components (`web/templates/partials/`)

| Component | Prototype | Notes |
|---|---|---|
| Header | `Header.dc.html` | wordmark, nav `posts projects homelab presentations photos radar now about` + `more ▾` (notes, snippets, links, favs, certificates), search `⌘K`, tr/en segmented toggle |
| Footer | `Footer.dc.html` | dark, tri-stripe, tagline, rss, json feed, certificates, github, x, linkedin, admin, deploy hash |
| Cover | `Cover.dc.html` | `tone`, `image`, `label`, `radius`; real image or striped tone fallback with two aberration circles |
| Command palette | Header | ⌘K / Ctrl+K, subsequence fuzzy, prefixes `p: n: s: l: f: pr: h:`, ↑↓ ↵ Esc |
| Chips / filters | all | off: white, muted, border; on: section ink bg, white text; filter state reflected in URL |
| Code block (light) | Post | `--surface-2` body, white header with filename, wrap, copy |
| Code block (dark) | Post, Snippets | file tabs, language badge, copy (shows "copied" 1.4s), line numbers; chroma theme mapped to dark band tokens |
| Terminal block | Post | three dots, cyan path, rose `$`, green output |
| Callout | Post | white, 3px left rule: tip success, warning warning, note info, danger accent-2 |
| Side note | Post | 220px aside, wraps under paragraph on narrow screens |
| Status badge, tag pill, stat tile, progress bar, level meter, hexagon badge | various | see `design/README.md` |
| Untranslated banner | Post | amber rule, link to the other language |

## 5. Screens → templates → roadmap phase

| Screen | Prototype | Template | Phase |
|---|---|---|---|
| Home (hero, tiles, latest, stream) | `Home` | `public/home` | 1 (stream rows grow per phase) |
| Post | `Post` | `post/show` | 1 |
| OG image | `OG Template` | Go image renderer | 1 |
| Admin Login (Google SSO) | `Admin Login` | `admin/login` | 0 |
| Admin Dashboard | `Admin Dashboard` | `admin/dashboard` | 1 |
| Admin Editor | `Admin Editor` | `admin/editor` | 1 |
| Snippets | `Snippets` | `snippet/index` | 2 |
| Links | `Links` | `link/index` | 2 |
| Favs | `Favs` | `fav/index` | 2 |
| Quick Capture | `Quick Capture` | `admin/quick` | 2 |
| Projects + GitHub | `Projects` | `project/index` | 4 |
| Project Detail (software + homelab) | `Project Detail` | `project/show` | 4 |
| Homelab & DIY | `Homelab` | `project/homelab` | 4 |
| Presentations | `Presentations` | `presentation/index` | 5 |
| Presentation Detail | `Presentation Detail` | `presentation/show` | 5 |
| Photos | `Photos` | `photo/index` | 5 |
| Certificates | `Certificates` | `certificate/index` | 5 |
| About | `About` | `page/about` | 5 |
| Now | `Now` | `page/now` | 5 |
| Radar | `Radar` | `radar/index` | 6 |

## 6. Not designed yet (derive from the system, or design before the phase)

| Screen | Phase | Notes |
|---|---|---|
| Mobile nav sheet, header under 900px | 1 | derived in 1.1: `<details>` sheet under 900px, nav + more as one list, 44px rows, lang switch at the bottom, works without JS |
| Post list, tag page | 1 | reuse latest-post cards + chips |
| 404 / 410 | 1 | dark band, palette hint |
| Admin entry list, media library, revisions diff | 1 | dashboard density |
| Notes list and note permalink | 2 | indigo tone |
| Admin categories, tags | 2 | |
| Admin import (Ghost dry-run), redirects | 3 | |
| Stream full page `/stream`, GitHub standalone | 4 | Home stream row pattern |
| Now archive | 5 | |
| Garden, runbooks, series index, glossary, reading list | 5 | tones: garden green, runbook navy, reading amber |
| Admin radar editor, jobs, settings | 6 / 9 | |
| Search results page (no-JS fallback for ⌘K) | 7 | |
| Favicon and monogram | 7 | design/README.md says not designed |

## 7. Admin rules
- 14px base, denser tables. 208px sidebar, 52px top bar with dark quick-capture `+`, ⌘K search, `staging` badge in amber.
- Editor: Markdown (CodeMirror 6) | live preview (htmx) | 280px side panel. Autosave 800ms debounce, consecutive autosaves coalesce into one revision, explicit save always creates one.
- Delete with backlinks: type the slug to enable the button.
- htmx for all partial updates. Every form still works without JS.
- Quick Capture: 390px target, hit targets ≥ 44px.

## 8. Accessibility
- WCAG 2.1 AA. Small text links use `--link` (`#0E7490`), never `--accent`.
- Check tone ink on tone tint for chips (all inks above are dark enough; re-verify any new tone).
- Radar fully usable from the list and keyboard. Lightbox and palette trap focus and close on Esc.
- `lang` attribute per page and per mixed-language block.

## 9. Assets needed
Real cover images, portrait (4:5), Credly badge images, 500px embed codes, SVG wordmark, favicon. All prototype content (names, bio, posts, certs, photos) is placeholder.

## 10. Do not port
`design/support.js`, `design/image-slot.js` and inline styles in `.dc.html` are prototype tooling. Rebuild in Go templates + one CSS bundle.
