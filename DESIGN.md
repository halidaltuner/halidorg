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

- Page widths on Snippets, Links and Favs (`--w-snippets`, `--w-links`, `--w-favs`) are content widths, as in the prototypes; the page adds the gutter on top (found in the Phase 2 exit check, the content was 48px narrower). The snippet meta reads "snippet · date · #tag #tag".
- Links and Favs follow their prototypes. The filter chips are links (`?status=`, `?type=`) so they work without JS; the cached page holds every item and `filter.js` filters in place. Favs offer only the types that have favs.
- A link row shows its favicon (16px, `--radius-1`) when the job found one, else the prototype's initial tile. The group name links to the category's awesome page.
- The read chip is static for visitors. Signed in, `links.js` turns it into a button that toggles the status without a revision.
- Fav cards keep the striped cover with the type in mono, since favs have no image yet; the type dot is `--kind-fav`. Cards with a URL are links, and `#slug` targets a card (feed items point there) with the focus outline.
- Awesome list (derive): the list head with the category tone (navy without one), a back link to links, a mono section index when there are child categories, then one H2 and bullet list per section: title in `--link`, domain in mono, comment on its own line.
- Awesome navigation (after the staging check): the links intro ends with "all awesome lists →" (`/{lang}/awesome`), and each categorized group head has an "awesome →" link on the right in mono 12px `--link`; the group name alone did not read as a link. An awesome page links back to links and to the index.
- Category tones show as an 8px dot in the tone's `--t-ink` before the name on `/links` group heads and `/awesome` index rows (a child without a tone takes its parent's, else navy; "other" has none). The awesome page header already carries the tone.
- Awesome index (derive): the list head, then one row per category with links, children too, in category order: bold name, link count in mono, and for a child a small "part of {parent}" line. A parent's count includes its children's links, as its page lists them.

## 18. Implementation notes (Phase 2.4)
- Quick Capture follows `Quick Capture.dc.html`: a 390px card on `--surface-2`, filling the screen under 480px. Every control is at least 44px tall.
- It works without JS. "fetch" is a GET of the same page with the URL (the bookmarklet opens that GET with `?url=&title=`); link / fav / note and tr / en are radio buttons styled as the prototype's buttons; the comment label reads "note" for notes through `:has()`.
- Before a fetch only the URL shows and draft / publish are disabled, as in the prototype. A failed fetch keeps the fields open so the title can be typed; fetching with an empty URL opens a note without a link.
- Publishing saves the draft first. Without a recent sign-in it goes through re-auth and comes back to the saved screen with "publish now".
- The admin top bar's `+` opens Quick Capture (as in `Admin Dashboard.dc.html`), no longer the new post form.
- The bookmarklet is a dashed `--link` pill ("drag me") with a one-line hint, under the Quick Capture card on wide screens and in a strip under the dashboard heading next to "+ quick capture" (hidden on touch screens). Clicked in place it does not navigate; the hint asks to drag it instead.
- An address already saved as a link, a fav or a note's source shows a warning under the URL field (`--warning` left border on `--warning-tint`) with a link to the existing entry, its kind and status. Fetch shows it; a save that has not shown it re-renders with it (409) and saves nothing. The form then carries `dup_ok` with that address, so the next press saves; editing the address checks again. Comparison ignores the fragment, trailing slashes and case.

## 19. Implementation notes (Phase 2.5)
- Home stream (`Home.dc.html`): posts, notes, snippets, links and favs in one day-grouped list. Each row has its kind's glyph tile (post `¶` cyan, note `~` indigo, snippet `{}` amber, link `↗` on `--surface-2` with `--text-muted`, fav `*` rose) and its meta: reading time, time of day, the first file's language, the domain, or the fav type. Untitled notes show their first line.
- Kind chips follow the prototype: the active kind chip takes its tint and ink, "all" is inverted. They are `?kind=` links filtered on the server without JS, and `filter.js` filters the cached page in place; chips appear only for kinds that have entries. Projects and homelab join in Phase 4.
- Palette: the hints list `p: n: s: l: f:` from the i18n catalog; rows are built from the Worker's JSON with the kind labels the header button carries.
- Categories and tags admin (derive, dashboard density): one table per page with inline TR / EN fields, a row of 16px tone swatches (the eight DESIGN.md tones plus "none", radios that work without JS), a save button per row and the entry count. Categories have kind tabs (links, snippets, favs), ↑ ↓ reorder buttons, a parent select limited to top-level categories of the kind (child rows are indented with ↳) and a "new category" form; tags have a "merge" form that names its effect. Slugs are shown, not edited: they are in URLs.

## 20. Implementation notes (Phase 2.6)
- Admin for notes, snippets, links and favs (derive, dashboard density): `/admin/{notes,snippets,links,favs}` list rows (title, lang, status pill, updated) with status and language filters, and a one-column editor in a 760px panel. Untitled notes are listed by their first line.
- The editor is a plain form: title, the language on new entries (radios), the kind's fields, summary (links and favs label it "my comment"), the Markdown body for notes and snippets, tags, slug (empty on a new entry means "from the title"; emptied later keeps the old one). "save" keeps the status; "publish" / "update live" saves, then publishes after a recent sign-in, otherwise goes through re-auth and offers "publish now". Delete sits in a closed `details` and needs a recent sign-in; a published note or snippet leaves a 410 at its address.
- Snippet files are boxes of name, language (guessed from the name when empty), order and code, plus one empty box: filling it adds a file, emptying a file's name and code removes it, the order field reorders. No JS.
- On screens up to 760px inputs use 16px text and every target is at least 44px, as in Quick Capture. Quick Capture's saved screen and its duplicate warning link to the editor.

## 21. Implementation notes (Phase 4.1)
- Project and build log admin (derive) use the Phase 2.6 editor. The project form adds the type (software / homelab radios; `:has()` shows the stack rows for software and the parts rows and category for homelab), status, tagline, the line above the title, featured, start and end months, the tone picker, a cover and, as rows like snippet files (fill an empty row to add, empty a row to remove, an order field), the hero buttons, the fact strip, stack or parts and the gallery. Amounts accept "27000", "27.000" or "149,90".
- On screens up to 760px a part row stacks as name / qty, price, currency, order / URL.
- A saved project lists its build log entries with "+ new entry", which opens the build log form with the project and language filled in; build logs sit under projects in the sidebar.

## 22. Implementation notes (Phase 4.2)
- `/projects` lists software projects only, as the prototype ("Hardware lives in homelab & diy"); homelab builds get their page in 4.3. Card pills are the line above the title split on "·", else the first three stack names. Stars and activity come with the GitHub sync (4.4); until then the card foot shows only "details →", and the GitHub section is left out rather than filled with sample data.
- Project Detail uses the project tone, else its category's, else green (software) or orange (homelab). The fact strip is the typed rows; a homelab build adds a "cost" cell from cost_total or the sum of its parts. Prices show compact ("₺27.0k"). The aside is "stack" (name and role) or "parts" (a quantity above 1 prefixes the name, "3× mini PC"). Gallery: three or more images follow the 2fr/1fr layout, two sit side by side, one fills the row. Up to 760px the hero and columns stack and the fact strip is two wide (an odd last cell spans both).
- Build log entry pages (derive) are a 760px article column: back to the project's timeline, a tone dot with the date and hours, the title, the text, and links to the older and newer entries.

## 23. Implementation notes (Phase 4.3)
- Homelab & DIY follows its prototype in the orange tone. Stat tiles: builds is the count of published homelab builds; nodes, idle W and uptime are typed at `/admin/settings` (free text, at most 16 characters, "–" when empty). The featured plate shows the build marked featured, else the newest. Category chips are `?category=` links in each category's tone, shown when builds sit in two or more categories; `filter.js` filters the cached page in place. Build cards show category (tone ink), status, title, tagline, and cost and time in mono.
- Home: the tile row under the hero shows only sections with a page (projects: live software projects, homelab: builds), on an `auto-fill` grid so two tiles keep the width they will have with six. The hero gains the outline "projects →" CTA.
- `/admin/settings` (derive) holds the homelab stats for now; a save queues `pages.changed` for `/{lang}/homelab`.

## 24. Implementation notes (Phase 4.4)
- Projects GitHub section follows the prototype: header with the 52-week total, the strip (week bars at `--gh-1..3` by thirds of the busiest week, min 8% high, first/middle/last month under it), the language bar (top four in `--accent`, `--info`, `--warning`, `--success`, the rest as "other" in `--border`), pinned repos (up to six, two columns) and the four latest releases. It is left out until the first sync. On a phone the header wraps and the repo grid is one column.
- Project cards linked to a repo show "★ stars" and "pushed 2h ago" in the foot, as the prototype.
- `/admin/github` (derive): last sync (status, time, error; a missing token says how to add it), "run now", and a repo table with pin/unpin and the project link.

## 25. Implementation notes (Phase 4.5)
- Stream rows gain projects (◆ green, meta the project status), homelab builds (⌁ orange, meta the status), build logs (meta "build log", in their project's tone) and GitHub activity as project rows (meta "release" or "new repo"), as the Home prototype. Chips add project and homelab.
- `/{lang}/stream` (derive): list header, the home stream's chips and day-grouped card at 860px. The chips filter on the server. The first page shows the newest 30 rows; "older →" opens month archive pages (`/{lang}/stream/2026-09`, the month muted after the title), each a whole month, linked "← newer" / "older →" to the neighbouring months with rows. Filtered pages are noindex. Home's stream ends with a "whole stream →" link.
- `/{lang}/github` (derive): the projects header pattern (◆ green eyebrow), the Projects GitHub section without its heading (the page title says it), then every activity row by month in the stream card. On Projects the section heading links to it.
- Palette: `pr:` finds software projects, `h:` homelab builds (⌁ dot in `--kind-homelab`).
- Build logs have their own OG card ("build log" in the project dot's green), keyed by the log's slug (`og/{lang}/logs/{slug}.png`), so a project slug change does not orphan it. The projects feeds carry build logs, titled "project: log".

## 26. Implementation notes (Phase 5.1)
- About follows its prototype: dark hero with the grid texture, "Hi, I'm **Name**." (the name in mono with the aberration shadow), bio at 20px in `--dk-text-soft`, the CTA buttons (cv.pdf second, as a download), and the 4:5 portrait with the rose and cyan offset frames, left out without a portrait. Skill tiles in their tone tint with white pills; the experience timeline with tone dots on a 2px rule; link cards in tone tints for the sections that have pages (projects, homelab & diy, now). Certificates and talks cards join with their pages.
- Now follows its prototype without the weather: green pulse dot, "updated N days ago" at 48px mono (36px on a phone), date and place, "N earlier versions →". Building cards (140px cover, progress bar in the tone ink), reading covers 2:3 with the 4/6px offset shadow (two per row on a phone), learning cards with the 10-cell meter, the dashed rose "not doing" strip (several items separated by "·"). Cards keep their width when there is only one (`auto-fill`).
- `/now/archive` (derive): list header with a back link, versions as dated rows in a card at 860px. A version page is the Now page with "version of {date}" and a "current page →" link.
- `/admin/pages` (derive): the shared editor; a new page picks About or Now first, then shows that template's sections as rows (dashboard density, note fields on their own line).

## 27. Implementation notes (Phase 5.2)
- Certificates follow the prototype: the cyan hero with stat tiles (active in `--success`, issuers in the cyan ink, total in text), issuer chips in each issuer's tone (`?issuer=`, filtered in place by `filter.js`, shown with two or more issuers), and cards with the 4px top rule in the issuer ink, the 112px hexagon badge (the issuer tint with a ✓ until a badge image is set), issuer, name, skill pills, then date, status and "verify ↗". Renew soon uses `--warning`; expired cards sit at 55% opacity.
- About gains the certificates card first: up to five mini hexagons (52px) of active certificates in their issuer tones, and "A, B, C and N more".
- `/admin/certificates` (derive): the list with issuer, months and status badges (expired muted), the issuers as inline rows below it, and a form per certificate.

## 28. Implementation notes (Phase 5.3)
- Photos follow the prototype: the violet hero with the "500px profile ↗" button (shown when the setting is set), album chips "name · count" (`?album=`), and the dense grid of 120px rows with tile spans and a dark mono caption "title ↗" on each tile. The prototype's "embedded from 500px" copy says "a selection from my 500px profile": there is no embed.
- The lightbox is a `:target` dialog per photo (works without JS; `photos.js` adds Esc, focus and scroll lock): the large image beside a dark panel with the album in `--dk-violet`, the title, the description (14px in `--dk-text-soft`, line breaks kept), the EXIF list and a solid violet "view on 500px ↗". On narrow screens the panel stacks under the image. The backdrop is `--dk-scrim`.
- `/admin/photos` (derive): the list with album, EXIF and spans, albums as inline rows below it, each with a multi-file upload and "publish now", and a form per photo with a "Fill from EXIF" button.

## 29. Implementation notes (Phase 5.4)
- Presentations follow the prototype: the rose hero, type chips (`?type=`, filtered in place by `filter.js`, shown with two or more types), and 16:9 cards with the dark "N slides · html" badge, the event in rose and the language spoken, title, description, then date and city. A card without a cover image draws a title slide: the event and title on the tone tint with a short ink rule.
- Presentation Detail follows its prototype with one change: the control bar (← →, `n / total`, the rose progress bar, "fullscreen ↗") sits under the 16:9 deck instead of over its bottom edge, so it never covers a slide. Fullscreen takes the deck and its bar. Outline rows are buttons that jump the deck and scroll it into view; without the deck bridge the buttons stay disabled and the rows open the deck. Without a deck the frame shows the cover or title slide and a note.
- The aside: "open deck ↗" solid rose, "download .html" and "watch recording · N min" outlined, then "related" with section glyphs (¶ posts, {} snippets, ◆ projects, ↗ elsewhere) in `--link`.
- Home gets the presentations tile ("N talks") and About the talks card ("N talks at M events") once one is published. The palette filters presentations with `t:`.
- `/admin/presentations` (derive): the entry editor with type, event, city, date, language spoken, cover, recording, outline and related rows, and a Deck panel under the form with the upload, the current version and the bridge script to paste into a deck.

## 30. Implementation notes (Phase 5.5a)
- Garden (derive, green): the posts list layout with a stage chip row (`?stage=`, filtered in place by `filter.js`) and post cards with a stage badge (seedling `--warning`, budding `--info`, evergreen `--success`) and "tended DATE". The article is the post template with the stage badge and the tended date in the meta line.
- Runbooks (derive, navy): a dense list of cards with a 3px navy rule, title, summary and platform, risk badge (low `--success`, medium `--warning`, high `--accent-2`) and "verified DATE". The article is the post template with a facts strip (platform, risk, verified) and the prerequisites as a list on the navy tint above the steps.
- Both join the more menu (garden, runbooks) and the admin sidebar under posts; the post editor shows the kind chip on, and the kind's fields in the side panel (maturity; platform, risk, last verified, prerequisites).

## 31. Implementation notes (Phase 5.5b)
- Series (derive, the posts list in cyan): the index has cards with a 3px cyan top rule, the part count, an ongoing (`--success`) or complete (`--info`) status, title and description; the posts list header links "series →". A series page lists its parts in order: the number in cyan mono, the kind glyph and title, the description and the date. The post crumb links the series name to its page.
- `/admin/series` (derive): the list with slug, part count and status, and a form per series (title and description per language, slug, status, order) with its parts linked to their editors.

## 32. Implementation notes (Phase 5.5c)
- Glossary (derive, indigo): a letter bar of mono chips, then the terms under each letter heading, the term in bold with its other names in mono, the short definition below. A term page has the short definition as a lead with a 3px indigo rule, then the explanation.
- Reading list (derive, amber): groups for reading, want to read, read and dropped, each a grid of cards with a 2:3 cover (the amber cover pattern without an image), title, author, the short take, stars or dates, and for books being read an amber progress bar with the percentage.
- Both join the more menu and the admin sidebar (glossary, reading list) and use the shared entry editor.

## 33. Implementation notes (Phase 5.5d)
- Connections (derive, from the post's backlinks block): one partial under every detail page (post, garden page, runbook, note, snippet, project, presentation, term). Backlinks first, then "related": up to three entries in the page's language sharing the most tags, newest first among equals. Each related row has the kind glyph from the stream (same tints), the title in `--link` and the summary muted below it.
- Posts, garden pages and runbooks always show the backlinks block with its empty message; other kinds show the block only when it has something.
- An entry with no title and no summary (a note) goes by its kind and day: "note · 2026-10-19".

## 34. Implementation notes (Phase 6.1, admin radar editor)
- Derive, dashboard density: one table of the working copy (name with the slug under it, quadrant and ring selects, both notes, an "off" box for leaving an item out of the next edition). Each row saves on its own with htmx and comes back in place with a "saved" badge or its errors; without JS the form posts and the page reloads at the row. Delete sits in a small disclosure under save.
- Above the table: the latest edition and what the next one changes ("next edition: 1 new, 2 moved, 0 dropped"), linking to the freeze page. Rows on the radar come first, by ring, quadrant and name; off rows are struck through.
- Freeze page: the diff since the latest edition (new, moved with from and to ring, dropped), the count of unchanged items, the label (this month, or the month after the latest edition) and a two-step confirm. Freezing and deleting need a recent sign-in.

## 35. Implementation notes (Phase 6.2, public radar)
- Built from design/Radar.dc.html: 1100px page, 36px title, the edition switcher as a segmented control, the legend, then the SVG card and the list side by side (one column on phones). Ring inks and band tints are tokens (`--ring-adopt` to `--ring-hold`, `--ring-*-tint`), the inks matching success, warning, info and accent-2.
- The SVG is drawn on the server: rings at r 60/120/180/240, axes, ring and quadrant labels, numbered blips (circle unchanged, triangle new, dashed trail from the ring it left). It is one image with a text alternative; the list is the way in for keyboards and screen readers.
- List rows are links to the item pages (the prototype's rows are buttons). Without JS every note shows; with `radar.js` the active row (hover or focus) shows its note and its blip darkens and grows, hovering a blip lights its row, ↑ and ↓ move between rows. Tags read "new", "↑ from hold", "↓ from adopt". Items that left show under "left in this edition".
- Item page (derive, indigo): back link, quadrant eyebrow, the name, its ring now as a chip, the note as a lead with a 3px rule, then its ring per edition with the move tag and the note where it changed, then the connections block.
- The stream shows one row per edition with the radar glyph ◎ (indigo), "3 new · 2 moved" as its meta.

## 36. Implementation notes (Phase 7.1, search page)
- `/{lang}/search` (derive: stream rows): eyebrow "> // search", a 48px heading, a mono input with a dark submit button, the prefix list as the subline, then results as stream rows (kind glyph tile, title, kind label and the language when it is not the page's). Photos use ◐ (violet), certificates ✓ (cyan).
- The header search button is a link to this page; with JS it opens the palette, and Enter in the palette with nothing selected goes here with the query.

## 37. Implementation notes (Phase 7.2, Mermaid)
- A diagram renders in a figure like the code blocks: surface background, 1px border, 8px radius, 16px padding, centered, scrolling sideways when wider than the column. Mermaid's base theme takes Plex Sans at 14px and the tokens: nodes in the cyan tint with cyan ink borders, secondary in indigo, notes in amber, lines in `--text-muted`.
- Without JS, and when a diagram does not parse, the source stays in its block (a rose border marks a failed one). Details and the server-side options: docs/mermaid.md.

## 38. Implementation notes (Phase 7.6a, block editor)
- The post, garden and runbook editor opens in a visual view (Tiptap): the text column at the reading size (`--fs-18`) with the settings column on the right; the live preview hides, since the view is the preview. A mono button in the editor bar switches to the Markdown view (CodeMirror, with the preview) and back; the choice is kept per browser.
- "/" opens the command menu: a popover list (surface, 1px border, popover shadow) of mono glyph tiles and labels, ↑ ↓ and Enter to pick. Selected text gets a dark formatting bar (bold, italic, strike, code, link). A ⋮⋮ handle in `--text-muted` appears beside the hovered block and drags it.
- Callouts show their kind as a small mono label above the body, with a 3px left rule in the callout tone (info, success, warning, rose); footnote references are small mono labels on the cyan tint and their notes sit at the end in muted text behind a dotted rule; code blocks have a mono field above them for the fence info (`go file=main.go`, `terminal`, `mermaid`) and dark blocks use the `--dk-*` tokens; a mermaid block previews its diagram under the source.
- A document the view cannot write back unchanged (raw HTML, say) opens in the Markdown view with a note above the editor.
- Code blocks are colored as you type (highlight.js classes sharing the chroma colors: light block per Post.dc.html, dark blocks per design/README.md). A fence without a language is guessed, in the editor and in the renderer. The code bar suggests languages.
- More "/" blocks: Heading 1, a to-do list (GFM `- [ ]`, the checkbox in `--link`), a YouTube video and a file. Images and PDFs are uploaded to the media library from "/image" and "/file", or pasted or dropped into the text; "/image" then asks for the alt text.
- A YouTube URL on a line of its own renders as a 16:9 player from youtube-nocookie.com, loaded lazily, 8px radius on `--dk-bg`. A link to a PDF under /media/ on a line of its own renders as a download card: surface, 1px border, 8px radius, a rose "PDF" tag, the name and "download ↓" in `--link`.
- Short-kind forms (note, snippet, link, fav, project, build log, presentation, term, book, pages; `admin/entry_edit.html`) use the same block editor for their body: a bordered box (1px `--border`, `--link` on focus, 4px radius) at 16px text, with the Markdown toggle beside the field label. The textarea stays the form field and the Markdown view.
