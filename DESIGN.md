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
| Code block (dark) | Post | file tabs, language badge, copy (shows "copied" 1.4s), line numbers; chroma theme mapped to dark band tokens |
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
| Post list, tag page | 1 | derived in 1.4: list head (eyebrow, 48px H1) over the latest-post card grid |
| 404 / 410 | 1 | derived in 1.4: dark band with grid, mono status code in cyan, short copy, home CTA; noindex, never cached |
| Admin entry list, media library, revisions diff | 1 | dashboard density |
| Notes list and note permalink | 2 | derived in 2.2: indigo tone, see §16 |
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

## 11. Implementation notes (Phase 1.4)
- Prose blocks share one measure, `--w-measure` (46rem, 68ch of the 18px body face). The prototype sets `max-width: 68ch` on each block, which makes code, terminal and callouts narrower than paragraphs because their type is smaller.
- The post TOC is hidden under 900px; above the title it pushes the article down a screen. Headings keep their anchors.
- Reading time: `N min` on cards, stream rows and the featured card; `N min read` on the post page.

## 12. Implementation notes (Phase 1.5)
- The OG card is drawn in Go (`app/internal/render/og`) from the template's values, with IBM Plex Sans 600 and JetBrains Mono 400/700 TTFs from their OFL releases (licenses next to the fonts). Colors are the band tokens, hard-coded in Go because a PNG renderer cannot read `tokens.css`; change both together.
- The title is balanced like `text-wrap: balance` and cut after 3 lines with an ellipsis. The prototype has no limit and overflows into the wordmark with long titles.
- Pages without their own card (home, lists, tags, errors) use a site card: no kind row, the home headline as title, `halid.org` as meta.
- Any visual change to the card bumps `og.Version`, which changes every card URL so social caches refetch.

## 13. Implementation notes (Phase 1.6)
- Admin shell: the sidebar and the search box use `box-sizing: content-box` like the prototype, so the 208px sidebar is 208px inside its padding. Sections that do not exist yet stay in the sidebar, greyed and unlinked. The signed-in email, the tr/en switch and sign out sit at the bottom of the sidebar (not in the prototype, which has no way to sign out).
- `--warning-tint` (#FEF3E2) is the `staging` badge fill from the prototype.
- Dashboard: the webmentions panel is hidden until Phase 8, so failed jobs move into the third column. "add redirect" opens an inline form (from, to, or 410) under the 404 heading.
- Editor additions to the prototype: a title field above the Markdown (the prototype derived the title from a `# heading`, which would print the title twice on the public page) and a slug field in the side panel. The preview shows the title as its h1. The cover drop zone shows the post's OG card when published; uploads come with media (1.7).
- Every admin form posts without JS. With JS: CodeMirror 6 replaces the textarea and writes back to it, htmx autosaves 800ms after the last input, ⌘S and ⌘⇧P click save and publish, tags become chips.
- Below 1100px the editor hides the preview; below 760px the columns stack.

## 14. Implementation notes (Phase 1.7)
- Entry list (derive): filters (status, language, tag) are GET parameters above the table; bulk publish and archive sit in a bar inside the panel and say that they need a recent sign-in. Status pills reuse the editor status colors (draft info, published success, scheduled and archived warning).
- Media library (derive): a dashed drop zone (cyan tint while dragging), a grid of cards with the image on the striped placeholder, TR and EN alt fields, a "ready to use" or "alt text missing" pill, copy Markdown (disabled until both alts exist) and a two-click delete. Uploads show one progress bar per file.
- Revisions (derive): a 320px timeline with old/new radios and a side-by-side line diff. Removed lines use `--tone-rose-tint`, added lines `--tone-green-tint`; unchanged runs fold to "n unchanged lines". Restore appears only for revisions that are not current.
- The editor's cover picker lists only images with both alt texts; the drop zone image falls back to the OG card.

## 15. Implementation notes (command palette)
- The palette matches `Header.dc.html`: overlay at 32% of `--text`, 620px panel 12vh from the top, mono input with a rose `>`, rows with kind dot, title and kind, and a hints footer. Results come from `GET /x/search` (FTS5 prefix search, ADR-010) instead of the prototype's client-side subsequence match; results in the page language come first, others are marked with their language.
- The header button is hidden until `palette.js` runs: the palette needs JS. The no-JS search page arrives with full-site search (ROADMAP Phase 4).

## 16. Implementation notes (Phase 2.2)
- Snippets follow `Snippets.dc.html`: a light viewer (white file bar, `--surface-2` code area), not the dark code block the component table listed for it. Tokens are colored with the light chroma classes of the post code blocks; the prototype only mutes comment lines.
- The active file tab has a 2px `--kind-snippet` underline. Without JS every file shows, one under the other, and the tabs are links to them; `snippet.js` shows one file, moves the language badge and raw link with the tab, and unhides `#` (line numbers) and copy.
- `/{lang}/snippets` shows the newest snippet; `/{lang}/snippets/{slug}` the chosen one with the same list. Below about 616px the list stacks above the viewer.
- Notes (derive): the post list head with an indigo eyebrow, then a 860px column of white cards. A card has the `~` glyph tile, the publish time as the permalink (`--link`), a TIL pill, the title in body size at weight 500, the body, tags and the source domain.

## 17. Implementation notes (Phase 2.3)
- Links and Favs follow their prototypes. The filter chips are links (`?status=`, `?type=`) so they work without JS; the cached page holds every item and `filter.js` filters in place. Favs offer only the types that have favs.
- A link row shows its favicon (16px, `--radius-1`) when the job found one, else the prototype's initial tile. The group name links to the category's awesome page.
- The read chip is static for visitors. Signed in, `links.js` turns it into a button that toggles the status without a revision.
- Fav cards keep the striped cover with the type in mono, since favs have no image yet; the type dot is `--kind-fav`. Cards with a URL are links, and `#slug` targets a card (feed items point there) with the focus outline.
- Awesome list (derive): the list head with the category tone (navy without one), a back link to links, a mono section index when there are child categories, then one H2 and bullet list per section: title in `--link`, domain in mono, comment on its own line.
- Awesome navigation (after the staging check): the links intro ends with "all awesome lists →" (`/{lang}/awesome`), and each categorized group head has an "awesome →" link on the right in mono 12px `--link`; the group name alone did not read as a link. An awesome page links back to links and to the index.
- Awesome index (derive): the list head, then one row per category with links, children too, in category order: bold name, link count in mono, and for a child a small "part of {parent}" line. A parent's count includes its children's links, as its page lists them.

## 18. Implementation notes (Phase 2.4)
- Quick Capture follows `Quick Capture.dc.html`: a 390px card on `--surface-2`, filling the screen under 480px. Every control is at least 44px tall.
- It works without JS. "fetch" is a GET of the same page with the URL (the bookmarklet opens that GET with `?url=&title=`); link / fav / note and tr / en are radio buttons styled as the prototype's buttons; the comment label reads "note" for notes through `:has()`.
- Before a fetch only the URL shows and draft / publish are disabled, as in the prototype. A failed fetch keeps the fields open so the title can be typed; fetching with an empty URL opens a note without a link.
- Publishing saves the draft first. Without a recent sign-in it goes through re-auth and comes back to the saved screen with "publish now".
- The admin top bar's `+` opens Quick Capture (as in `Admin Dashboard.dc.html`), no longer the new post form.
- The bookmarklet is a dashed `--link` pill ("drag me") with a one-line hint, under the Quick Capture card on wide screens and in a strip under the dashboard heading next to "+ quick capture" (hidden on touch screens). Clicked in place it does not navigate; the hint asks to drag it instead.
