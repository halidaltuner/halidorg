# Handoff: halid.org — personal engineering site + admin

## Overview
Bilingual (TR/EN) personal site for halid.org: posts, notes, snippets, links, favs, software projects, homelab/DIY builds, presentations, photos (500px embeds), certificates (Credly-style), tech radar, now page, about. Plus an admin: Google-SSO login (only @halid.org), dashboard, Markdown editor, mobile quick capture. Also includes a 1200×630 OG image template.

`DESIGN.md` (included) is the original spec: tokens, component list, admin rules, a11y. This README describes what the prototypes add or change on top of it. **Where they differ, the prototypes win** (logo is now "halid.", the theme is light-only, the palette is more colorful, and pages were added).

## About the Design Files
The `.dc.html` files are **design references built in HTML**. They are prototypes showing the intended look and behavior, not production code. Recreate them in the target stack. The spec implies Go + server-rendered templates + htmx (admin) + Postgres, with CodeMirror 6 for the editor. Follow that stack's patterns: server templates and partials, htmx swaps, a CSS file with tokens. Do not ship the HTML directly.

To view the prototypes, open any `.dc.html` in a browser (the runtime is in `support.js`). Styles are inline in the prototypes only because of the tooling. In production, extract the tokens and components into CSS.

Sample content (posts, certs, jobs, photos, the name "Halid", bio) is **placeholder**. Replace it with real data.

## Fidelity
**High-fidelity.** Final colors, type, spacing and interactions. Recreate pixel-close.

## Design Tokens (light theme only)

### Base colors
| token | hex | use |
|---|---|---|
| bg | `#F7F8FA` | page |
| surface | `#FFFFFF` | cards, header |
| surface-2 | `#EEF1F5` | inputs, inline code, chips |
| border | `#DCE1E8` | dividers, card borders |
| border-soft | `#EEF1F5` | row dividers inside cards |
| text | `#0E1525` | body/headings |
| text-muted | `#5A6578` | meta, captions |
| link | `#0E7490` | small text links (AA on white; `#0891B2` fails AA for small text) |
| accent | `#0891B2` | focus ring, hover, highlights |
| accent-2 | `#E11D48` | aberration split, destructive, "hold" ring |
| success `#15803D` · warning `#B45309` · info `#4F46E5` | | statuses |

### Dark surfaces (used as bands, not as a theme)
Home hero, About hero, Now hero, Presentation detail hero, Footer, Admin Login, OG image, dark code blocks:
`#0B1220` bg · `#111A2E` surface · `#17223A` surface-2 / grid lines · `#24304A` border · `#E6EBF2` text · `#8D98AB` muted · `#22D3EE` cyan · `#FB7185` rose · `#4ADE80` green · `#FBBF24` amber · `#A5B4FC` indigo.
Grid texture: two 1px `#17223A` linear-gradients, 48px cell, opacity .6.

### Section tones (tint bg / ink text)
Each section owns a tone. It is used for hero bands, chips, covers, tag pills and active nav underlines.
| tone | tint | ink | stripe | section |
|---|---|---|---|---|
| cyan | `#D7F1F7` | `#0E7490` | `#C4E9F2` | posts, certificates |
| indigo | `#E4E3FC` | `#4338CA` | `#D5D3FA` | notes, radar, learning |
| amber | `#FCEBCF` | `#92400E` | `#F8DDB0` | snippets, reading |
| green | `#D9F0E0` | `#166534` | `#C5E7CF` | projects |
| orange | `#FDE3D3` | `#9A3412` | `#FAD0B6` | homelab & DIY |
| rose | `#FCE1E7` | `#BE123C` | `#F9CCD6` | presentations |
| violet | `#EBE2FD` | `#6D28D9` | `#DDCFFB` | photos |
| navy | `#DCE3EF` | `#0E1525` | `#CBD5E6` | neutral |

Nav active underline colors: posts `#0891B2`, projects `#15803D`, homelab `#C2410C`, presentations `#BE123C`, photos `#6D28D9`, radar `#4F46E5`, now `#0891B2`, about `#0E1525`.

Kind colors (dots and stream glyph tiles; the glyph sits in a 28×28 tile with tint bg and ink text): post `¶`, note `~`, snippet `{}`, link `↗`, fav `*`, project `◆`, homelab `⌁`.

### Typography
- Sans: **IBM Plex Sans** 400/500/600. Mono: **JetBrains Mono** 400/500/700.
- Scale: 11 / 12 / 13 / 14 / 15 / 16 / 18 / 20 / 22 / 24 / 28 / 36 / 40 / 48 / 56.
- Page hero H1: 48px/1.08, 600, letter-spacing −0.025em, `text-wrap: balance`.
- Section H2: 28px/1.3, 600, −0.01em.
- Card title: 20–22px/1.3–1.35, 600.
- Long-form body: 18px/1.7, max 68ch, `text-wrap: pretty`.
- Meta, eyebrows, chips, dates: mono 11–13px, muted. Eyebrows are prefixed with a section glyph or `//` and use the section ink color.

### Spacing / radius / shadow / motion
- Spacing: 4 · 8 · 12 · 16 · 20 · 24 · 32 · 40 · 48 · 56 · 64 · 72 · 96.
- Containers: 1200px (most pages), 1100px (Now), 1240px (Post with TOC), 820–860px (Links, stream), 68ch (prose).
- Gutters: 24px.
- Radius: 4px (inputs, buttons, code, badges), 8px (cards, panels), 999px (chips and tag pills only).
- Shadows:
  - card rest: `0 1px 3px rgba(14,21,37,.06)`
  - card hover: `0 10px 28px rgba(14,21,37,.10)` plus `translateY(-2px)`
  - hero media: `0 16px 40px rgba(14,21,37,.14)`
  - popovers: `0 12px 32px rgba(14,21,37,.12)`
- Motion: 120–180ms ease-out. Respect `prefers-reduced-motion`.
- Focus: `2px solid #0891B2`, offset 2px.

### Brand
- **Wordmark:** text "halid" in JetBrains Mono 700, letter-spacing −0.03em, followed by a "." in `#E11D48`.
- **Chromatic aberration:**
  - light background: `text-shadow: -1.5px 0 rgba(225,29,72,.7), 1.5px 0 rgba(8,145,178,.7)`
  - dark background: `rgba(251,113,133,.8)` and `rgba(34,211,238,.8)`
  - hover: the offset grows to 2.5px
- Sizes: 24px header, 28px footer, 32px login, 44px OG. Replace with the SVG wordmark when it is available.
- **Tri-stripe:** a 3px bar above the header and footer, `linear-gradient(90deg, #E11D48 0 33%, #0891B2 33% 66%, #4F46E5 66%)`.

## Shared Components

**Header** (`Header.dc.html`). White, bottom border, tri-stripe on top.
- Row contents: wordmark, nav, then right-aligned search and tr/en.
- Nav: posts · projects · homelab · presentations · photos · radar · now · about, then a **more ▾** dropdown with notes, snippets, links, favs, certificates (each with an 8px tone square).
- Nav items: 14px, padding 6×7, nowrap. Active item is text color with an inset 2px bottom shadow in the section color.
- Search button: shows "search ⌘K" and opens the command palette.
- **tr/en segmented toggle:** the active segment is `#0E1525` with white text. Persist the choice in `localStorage['kromatik-lang']` and broadcast a `kromatik-lang` window event so pages re-render their copy. In production, follow DESIGN.md: switch to the paired translation URL, or show a "not translated yet" page.
- **No theme toggle.** It was removed.

**Command palette.**
- Opens with ⌘K / Ctrl+K and closes with Esc. Overlay `rgba(14,21,37,.32)`; panel max 620px, placed at 12vh from the top.
- Input is mono with a rose `>` prompt.
- Fuzzy match is subsequence-based. Prefix filters: `p:` posts, `n:` notes, `s:` snippets, `l:` links, `f:` favs, `pr:` projects, `h:` homelab.
- Keyboard: ↑↓ moves the selection (row bg `#EEF1F5`), ↵ navigates.
- Each row: kind dot, title, kind label.
- Footer row: lists the prefixes as hints.

**Footer** (`Footer.dc.html`). Background `#0B1220`.
- Contents: tri-stripe, the dark wordmark, the tagline "engineering notebook · istanbul".
- Links: rss, json feed, certificates, github, x, linkedin, admin.
- A rule, then the deploy hash in cyan.

**Cover** (`Cover.dc.html`). A colorful cover-image placeholder used everywhere.
- Props: `tone`, `slot` (image id), `label`, `radius`. Fills its parent; the parent must be `position:relative` with an aspect ratio.
- Layers:
  1. tint background
  2. 135° stripes in the stripe color (10px on, 10px off) at opacity .7
  3. two large offset outlined circles at the top-right, one in the tone ink and one in `#E11D48`, opacity ~.13 (the aberration motif)
  4. the actual image, full-bleed, object-fit cover
- In production, render the real cover image when present and fall back to this pattern.

**Chips / filters.** Mono 12px, padding 4–5 × 10–12, pill radius, 1px border.
- Off: white background, muted text, `#DCE1E8` border.
- On: section ink as background and border, white text. On Home stream the active chip uses the kind tint background with ink text and border.

**Code blocks.** Two styles:
- **Light:** `#EEF1F5` body with a white header bar. The header shows the filename on the left and wrap/copy on the right.
- **Dark:** `#0B1220` body, a `#111A2E` tab bar with file tabs (active tab has a 2px cyan underline), a language badge, a copy button that shows "copied" for 1.4s, and line numbers in muted text.
  - Syntax colors: tags `#FB7185`, attributes `#A5B4FC`, strings `#4ADE80`, comments `#8D98AB`.
- Also a terminal variant with three dots (rose/amber/green): a cyan path, a rose `$` prompt, and output in green.

**Callouts.** White background, 3px left rule in the token color, mono 12px label (`tip` success, `warning` warning, `note` info, `danger` accent-2).

## Screens

### Public
1. **Home** (`Home.dc.html`)
   - Dark hero: grid texture, two columns (auto-fit, min 340px). Left: eyebrow, 48px headline with two colored words (cyan `web`, rose `homelab`), 18px sub, a cyan primary CTA ("about me") and an outline CTA ("projects →"), both nowrap. Right: featured post card on `#111A2E` with a 16:9 Cover, mono meta and title.
   - Tile row: 6 section tiles (projects, homelab, presentations, photos, certificates, radar). Each tile has a tint bg, ink text, a 22px glyph, label and count, and lifts 2px on hover.
   - "Latest posts": 3 cards (min 320px) with a 16:9 cover, tinted tag pills, title, 2-line clamped summary and mono date/reading time.
   - "Stream": kind filter chips; a white card grouped by day (110px date column); rows have a glyph tile, title and meta. `?kind=` in the URL presets the filter.
2. **Post** (`Post.dc.html`)
   - Full-width cover: `clamp(240px, 38vw, 440px)`.
   - Layout: a sticky 200px TOC (active item has a 2px cyan left rule; the scroll position drives it) and an article column.
   - Article top: series breadcrumb, 48px H1, mono meta.
   - Body content:
     - side notes: a 220px aside next to the paragraph that wraps under it on narrow screens
     - a light code block, a dark multi-tab code block and a terminal block
     - tip and warning callouts
   - End of article: series prev/next cards, backlinks, webmentions (avatar plus source).
   - In TR mode, a "Bu yazı henüz Türkçeye çevrilmedi" banner (amber rule) with a link to switch to English.
3. **Snippets** (`Snippets.dc.html`). Left: snippet list. Right: viewer with file tabs (active has an amber underline), language badge, line-number toggle (#), raw link and copy.
4. **Links** (`Links.dc.html`). Grouped by category. Each row: favicon, title, domain (mono), comment, and a read/unread chip that toggles. Filter: all / unread / read.
5. **Favs** (`Favs.dc.html`). Type filter. Card with a striped cover, type dot and title/author. Rating is 5 small squares (filled `#0E1525`); no stars.
6. **Projects** (`Projects.dc.html`)
   - Header: green eyebrow and a 48px H1.
   - Project cards: 16:9 cover, a status badge overlaid at the top-left, a mono name, green stack pills, and stars, activity and "details →". Links to `Project Detail?id=`.
   - GitHub section:
     - 52-week bar strip, 40px tall, in three cyan intensities (`#C9E9F1`, `#67C6DB`, `#0891B2`)
     - language bar (8px)
     - pinned repos grid
     - releases list
7. **Homelab & DIY** (`Homelab.dc.html`)
   - Orange tint hero: H1, sub and 4 stat tiles (builds, nodes, idle W, uptime).
   - Featured build: 4:3 cover with a white caption plate.
   - Category chips: homelab, electronics, 3d print, woodwork, network (each with its own tone).
   - Build cards: 4:3 cover, category in ink, status, title, description, and cost and time in mono.
8. **Project Detail** (`Project Detail.dc.html?id=site|radargen|ghost2md|rack|eink`). One template for both software and homelab.
   - Tint hero in the project tone: back link, status badge, kind, 48px H1, 20px tagline, primary/secondary buttons, and a 4:3 cover.
   - Fact strip: overlaps the hero by −28px; 1px-gap grid of white cells, each a mono key over a 20px value.
   - Article: H2 + paragraph sections, then a gallery (2fr/1fr grid, 2 rows of 180px).
   - Sticky aside:
     - list card titled **stack** for software, or **parts** with prices for homelab
     - build log timeline: 10px tone dots with a 3px tint ring
9. **Presentations** (`Presentations.dc.html`). Rose hero. Type filter: talk / workshop / lightning. Card: 16:9 title-slide cover with a dark "N slides · html" badge, event name in rose, language, title, description, date and city. Links to the detail page.
10. **Presentation Detail** (`Presentation Detail.dc.html`)
    - Dark hero: back link, event meta, H1.
    - **16:9 HTML deck embed** (production: an `<iframe>` to the self-contained deck HTML).
    - Embed control bar: ← → buttons, `n / total`, a rose progress bar, and "fullscreen ↗".
    - Below:
      - abstract
      - clickable outline (the number in rose; clicking jumps the deck to that slide)
      - aside with "open deck", "download .html", "watch recording", and related links
11. **Photos** (`Photos.dc.html`)
    - Violet hero with a "500px profile ↗" button.
    - Album chips with counts.
    - Grid: `auto-fill minmax(260px,1fr)`, 120px rows, dense flow; tiles span 1–3 rows and 1–2 columns.
    - **Each tile is a 500px embed.** A caption button opens a lightbox: dark panel with a large image, album, title, EXIF (camera, lens, exposure, place) and "view on 500px". Esc or a click outside closes it.
12. **Certificates** (`Certificates.dc.html`). Cyan hero with stat tiles (active, issuers, total) and issuer chips (AWS, CNCF, HashiCorp, Google, Linux Foundation; each has its own tone).
    - Card: 4px top border in the issuer ink, and a **112px hexagon badge** (clip-path `polygon(50% 0,93% 25%,93% 75%,50% 100%,7% 75%,7% 25%)`) holding the Credly badge image.
    - Card text: issuer, name, skill pills, then date, status and "verify ↗" (the Credly URL).
    - Status: active `#15803D`, renew soon (expires within 3 months) `#B45309`, expired muted with the card at 55% opacity.
13. **About** (`About.dc.html`)
    - Dark hero: 56px "Hi, I'm **Halid**" with the name in mono and the aberration effect, bio, CTAs (say hello, cv.pdf, github, linkedin).
    - 4:5 portrait framed by two offset outlines (rose +10px, cyan −10px).
    - 4 skill tiles, each a tone tint with white pills.
    - Experience timeline: colored dots on a vertical rule.
    - Link cards: certificates (with mini hexagons), talks, homelab, now.
14. **Now** (`Now.dc.html`)
    - Dark hero: green pulse dot, 48px mono "updated N days ago", date, place, weather, archive link.
    - **Building:** cards with a 140px cover thumbnail, title, note, stage and a progress bar in the tone color.
    - **Reading:** 2:3 book covers with a 4/6px offset solid shadow in the tone ink, and a progress bar.
    - **Learning:** tint cards with a level label and a 10-cell level meter.
    - "Not doing" strip: dashed rose border.
15. **Radar** (`Radar.dc.html`)
    - SVG 500×500. Quadrants: languages (top-right), techniques (top-left), tools (bottom-left), platforms (bottom-right).
    - Rings at r = 60 / 120 / 180 / 240, tinted green / amber / indigo / rose (hold uses accent-2).
    - Blips: circle = no change, filled triangle = new, dashed line = moved (from the previous ring position).
    - Hover or focus syncs the blip and the side list; the list is grouped by ring and the active row expands its note.
    - Edition switcher: 2026-10 / 2026-04.
    - The radar must be fully usable from the list and the keyboard.
16. **OG Template** (`OG Template.dc.html`). 1200×630, `#0B1220` with grid texture.
    - Kind dot, kind and date in mono 26px, title at 76px/1.08 (600, max 960px).
    - Bottom: the 44px wordmark with aberration, and meta on the right.
    - Props: title, kind, date, meta.

### Admin (14px base, denser)
17. **Admin Login** (`Admin Login.dc.html`). **Google SSO only, restricted to @halid.org.**
    - Left: dark brand panel with grid texture, two aberration circles, the wordmark and a status summary.
    - Right (`#111A2E`): "Sign in", a staging badge, the copy "Admin access is limited to Google Workspace accounts on @halid.org", and a single white **Continue with Google** button (52px tall).
    - Below the button: security notes — hd=halid.org enforced server-side; 12h session with re-auth for publish and delete; 2-step verification via Google.
    - States:
      - loading: the button reads "signing in…"
      - non-halid account: banner (rose rule) reading "`x@gmail.com` is not allowed. Sign in with an @halid.org account."
    - **Production:** use Google OAuth/OIDC with `hd=halid.org`, **and** verify the `hd` claim and the email domain server-side; `hd` is only a hint. No password form.
18. **Admin Dashboard** (`Admin Dashboard.dc.html`)
    - 208px white sidebar: Dashboard, Content (indented kinds with counts), Categories, Tags, Media, Radar, GitHub, Jobs, Redirects, Import, Settings.
    - 52px top bar: a dark quick-capture `+`, search with ⌘K, and a `staging` badge in amber.
    - Panels:
      - drafts
      - scheduled + recent publishes (with translation status)
      - webmentions to moderate (approve/reject removes the row)
      - failed jobs (retry → "queued · retry 2/3")
      - 404s with "add redirect"
19. **Admin Editor** (`Admin Editor.dc.html`). Full height. Three columns: Markdown | live preview | 280px side panel.
    - Top bar: back link, title, status badge (draft = info, published = success), save state, staging badge, save (⌘S) and publish (⌘⇧P).
    - Autosave: 800ms debounce. Consecutive autosaves coalesce into one revision; explicit saves always create a new one.
    - Side panel:
      - kind
      - series
      - tags (Enter adds, click removes)
      - cover drop zone (falls back to the OG template)
      - SEO description with an n/160 counter
      - translation status with "create tr draft"
      - revisions list
    - Delete: typed confirmation of the slug, required because the entry has backlinks. The delete button is disabled until the text matches.
20. **Quick Capture** (`Quick Capture.dc.html`). Mobile, 390px wide, all hit targets ≥44px.
    - Paste a URL → fetch → title (editable), description, domain.
    - Choose link / fav / note; add a comment; choose tr/en.
    - Draft or publish → success screen.

## Interactions & State
- Language: global `lang` (tr|en), persisted, broadcast to all mounted components.
- Filters (stream kind, link status, fav type, homelab category, talk type, photo album, cert issuer): local state; reflect them in the URL in production.
- Radar: `edition`, `hoveredId`.
- Photos: `album`, `openIndex` (lightbox).
- Presentation: `slide` (1..N). Outline click → set the slide and scroll to the top.
- Editor: `md`, `status`, `savedLabel`, `revisions[]`, `tags[]`, `seo`, `confirming`, `confirmText`.
- Login: `chooser`, `loading`, `error`, `errorEmail`.
- All admin partial updates go through htmx (see DESIGN.md §5). Every form must still work without JS.

## Assets
- No real images are included. Every Cover, portrait, badge and photo is a placeholder slot. Provide real cover images, the portrait, Credly badge images and 500px embed codes.
- Fonts: Google Fonts (IBM Plex Sans, JetBrains Mono). Self-host them in production.
- The wordmark is text-only for now; swap in the SVG when it is available.
- Favicon and monogram: not designed yet (see DESIGN.md §7).

## Files
- `Header.dc.html`, `Footer.dc.html`, `Cover.dc.html` — shared components.
- Public: `Home`, `Post`, `Snippets`, `Links`, `Favs`, `Projects`, `Project Detail`, `Homelab`, `Presentations`, `Presentation Detail`, `Photos`, `Certificates`, `About`, `Now`, `Radar`, `OG Template` (`.dc.html`).
- Admin: `Admin Login`, `Admin Dashboard`, `Admin Editor`, `Quick Capture` (`.dc.html`).
- `DESIGN.md` — original spec. `support.js`, `image-slot.js` — prototype runtime only; do not port.
