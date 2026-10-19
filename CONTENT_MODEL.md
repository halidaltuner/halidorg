# Content Model

## 1. Shared core: `entries`

Every content item with its own page has one row here. Type-specific fields live in a per-kind table joined on `entry_id`. Gallery-style data without detail pages (photos, certificates) lives in its own tables (section 3).

| Field | Type | Notes |
|---|---|---|
| id | TEXT (ULID) | PK |
| kind | TEXT | `post`, `note`, `garden`, `runbook`, `snippet`, `link`, `fav`, `project`, `buildlog`, `presentation`, `book`, `term`, `radar_item`, `page` |
| slug | TEXT | unique per (kind, lang) |
| lang | TEXT | `tr` or `en` |
| translation_group_id | TEXT | links TR/EN siblings |
| title | TEXT | |
| summary | TEXT | lists, OG, feeds, SEO description (≤160 enforced in editor) |
| status | TEXT | `draft`, `scheduled`, `published`, `archived` |
| visibility | TEXT | `public`, `unlisted`, `private` |
| published_at | INTEGER | unix ts |
| updated_at | INTEGER | |
| current_revision_id | TEXT | FK revisions; newest revision, what the editor shows |
| published_revision_id | TEXT | FK revisions; what public pages render, moves only on publish (ADR-006); required when `published` |
| cover_media_id | TEXT | FK media, nullable; falls back to Cover pattern / OG template |
| tone | TEXT | nullable override of the section tone (`cyan`, `indigo`, `amber`, `green`, `orange`, `rose`, `violet`, `navy`) |
| series_id | TEXT | nullable |
| series_order | INTEGER | nullable |

### Supporting tables
- `revisions(id, entry_id, body_md, body_html, toc_json, data_json, origin: autosave|manual|publish|import, created_at, note)` consecutive `autosave` rows coalesce (update in place) until a `manual` save
- `tags(id, slug, name_tr, name_en, tone)` + `entry_tags(entry_id, tag_id)`. Detail pages list up to three related entries: published, public, same language, most shared tags first; a change to a listed entry's tags, title, summary or slug re-renders its tag-mates' pages (the newest 30 per language).
- `categories(id, kind, slug, name_tr, name_en, tone, parent_id, sort)` links, snippets, favs, homelab builds, awesome lists (migrations/0006_short_form.sql). `kind` is one of link, snippet, fav, homelab, awesome; `(kind, slug)` is unique; `tone` is one of the DESIGN.md tones. An entry's `category_id` must point at a category of its own kind (checked in Go). Categories nest one level: a parent is a top-level category of the same kind. The admin (`/admin/categories`, Phase 2.5) renames, sets tones, reorders (`sort` 10, 20, 30) and reparents; slugs never change. Tags are renamed and merged at `/admin/tags`: a merge moves every entry onto the other tag and deletes the first.
- `series(id, slug, title_tr, title_en, summary, status, summary_tr, summary_en, sort)` (summary per language and sort: migrations/0016_series_summaries.sql; the old `summary` was copied into both and is unused). Edited at `/admin/series`; posts, garden pages and runbooks join one in the post editor (`series_id`, `series_order`). Status `active` or `done` shows on the site, `archived` hides the series page and leaves the editor's list. `/{lang}/series` lists the series with a published part in that language, `/{lang}/series/{slug}` its parts in order. A part's change re-renders the series pages and the other parts (their nav shows its title, `jobs.Ref.Series`/`Parts`); a series edit re-renders them too. Deleting a series takes its parts out of it.
- `links_graph(from_entry_id, to_entry_id, context)` backlinks (migrations/0004_links_graph.sql). The entry.changed job rebuilds an entry's outgoing rows from its published HTML: links to the page of any kind with one (`/{lang}/{posts|notes|snippets|projects|presentations|garden|runbooks|glossary}/{slug}`, relative or on the site origin), `context` is the link text. Unlisted and deleted entries have no outgoing rows.
- `media(id, r2_key, mime, width, height, alt_tr, alt_en, blurhash, created_at, name, bytes)` (name and bytes: migrations/0005_media_meta.sql). Images only (PNG, JPEG, GIF, WebP; no SVG), 10 MB max, R2 key `media/{id}/{file}`, served by the Worker at `/media/...`. An image is usable (cover, Markdown) only once both `alt_tr` and `alt_en` are set; one used as a cover cannot be deleted.
- `redirects(from_path, to_path, code)` importer + slug changes
- `not_found_log(path, hits, last_seen_at)` feeds the dashboard 404 panel
- `webmentions(id, target_entry_id, source_url, author, author_avatar, content, type, status, received_at)`
- `syndications(entry_id, network, url, posted_at)` POSSE
- `job_runs(id, type, payload, status: queued|running|done|failed|dead, attempt, max_attempts, error, created_at, started_at, finished_at)` (migrations/0003_jobs.sql)
- `admin_sessions(id, email, created_at, expires_at, last_reauth_at, ip, ua)` see ADR-005. Exception to ULID ids: `id` is the hex HMAC-SHA256 of the cookie token, never the token itself
- `search_fts` FTS5 virtual table, in the separate `halidorg-search-{env}` database, not the main one (ADR-010)

## 2. Per-kind schemas

### post
`posts(entry_id, reading_minutes, canonical_url, mermaid bool, comments_enabled bool)`

### note (TIL)
`notes(entry_id, source_url nullable, til bool)` (0006). Title optional; `source_url` must be absolute https.

### garden
`garden_pages(entry_id, maturity: seedling|budding|evergreen, last_tended_at)` (migrations/0015_garden_runbooks.sql). `last_tended_at` is set on every publish.

### runbook / cheatsheet
`runbooks(entry_id, platform, prerequisites, verified_on, risk: low|med|high)` (0015). `prerequisites` is plain text, one per line, shown as a checklist above the steps (not Markdown, so it needs no rendering outside the revision). `verified_on` is the day the steps last worked.

Garden pages and runbooks are long-form kinds written in the post editor (`internal/content/post`, `post.Kinds`): each also has a `posts` row (reading time, mermaid), the kind is picked when the entry is created and never changes, and lists, feeds and lookups keep to their kind. A series can mix the long-form kinds.

### snippet
`snippets(entry_id, category_id, gist_url nullable)`
`snippet_files(snippet_id, filename, language, code, sort)` one or more files, shown as tabs (0006). A snippet needs at least one file to publish; filenames are unique per snippet and have no slashes. Each revision stores the files in `data_json` (`{"files":[...]}`), so revision diffs and restores include them; the table holds the current files.

### link
`links(entry_id, url, domain, favicon_url, category_id, via nullable, archived_url nullable, read_status: unread|read)`. `favicon_url` is set by the entry.changed job: `/favicons/{domain}.{png|jpg|gif|webp|ico}` in R2, one object per domain, served by the Worker; empty when the site has no raster icon.
`title` = link title, `summary` = my comment. `domain` is derived from `url` (without `www.`). No item page: links live on `/{lang}/links`; search results open the link itself (0006).

### fav
`favs(entry_id, fav_type: tool|repo|video|product|article|book|place|other, author nullable, url nullable, rating 1-5 nullable, category_id)` (0006). No item page: favs live on `/{lang}/favs`.

### awesome list
Every link category is an awesome list at `/{lang}/awesome/{category}` (Phase 2.3): its published links, then one section per child category. No table of its own; a page that renders no link is a 404. A curated `page` with `data_json.list_category_ids` can come with pages in Phase 5.

### project (software and homelab/DIY share one model and one detail template)
`projects(entry_id, project_type: software|homelab, project_status, category_id nullable, tagline, kind_line, github_repo nullable, started_at, ended_at, featured bool, cost_total nullable, currency default 'TRY', time_spent)` (migrations/0007_projects.sql)
- `project_status` (named apart from `entries.status`, the publish state) follows the prototypes: `active`, `maintained`, `running`, `in_progress`, `paused`, `done`, `idea`, `archived`.
- `kind_line` is the small line over the title ("go · postgres · htmx"); empty means derived: the first three stack items for software, the category for homelab.
- `time_spent` is free text ("6 weekends", "ongoing"), as on the build cards. Money (`cost_total`, part prices) is in minor units (kuruş, cents); an empty `cost_total` means the sum of the parts priced in the project currency.
- `project_facts(project_id, key, value, sort)` fact strip cells (e.g. `nodes: 3`, `idle: 38 W`)
- `project_stack(project_id, name, role, sort)` "stack" aside, software only; `role` ("server", "storage + search") is shown on the right (migrations/0008)
- `project_parts(project_id, name, qty, price, currency, url nullable, sort)` "parts" aside, homelab only; price is the whole line
- `project_links(project_id, label, url, sort)` the hero buttons, the first is the primary one; a URL is https, a path on the site or an `#anchor` on the project page (`#log`)
- `project_gallery(project_id, media_id, sort)` images with both alt texts; a media item in a gallery cannot be deleted
- Homelab categories (`categories.kind = 'homelab'`, seeded by 0007 with the prototype tones): homelab orange, electronics violet, 3d print cyan, woodwork amber, network green. A homelab project needs one; a software project has none.
- Each revision keeps the facts, stack, parts, links and gallery in `data_json`, so a restore brings them back; public pages read the published revision's copy.
- Homelab stats tiles (builds, nodes, idle W, uptime): `builds` is computed; the rest live in `site_settings` under `homelab_stats` (`{"nodes","idle","uptime"}`, free text), edited at `/admin/settings`

### buildlog
`buildlogs(entry_id, project_entry_id, log_date, hours_spent nullable)` rendered as the project timeline and at `/{lang}/projects/{project}/log/{slug}`. A log is in its project's language; deleting a project deletes its logs.

### presentation (replaces `talk`)
migrations/0014_presentations.sql, `internal/content/presentation`.
`presentations(entry_id, pres_type: talk|workshop|lightning, event, city, held_on, talk_lang, slide_count, deck_r2_key, deck_version, recording_url nullable, recording_minutes nullable, outline_json)`
- The body is the abstract; `summary` is the card description. The cover is any media image (like photos, not only those with both alt texts); its alt text is the media item's, else the title. `held_on` is the day it was given (the list sorts by it); `talk_lang` is the language spoken, shown on the card, apart from the entry's page language.
- `outline_json`: `[{ "n": 3, "title": "Why render-on-publish" }]` drives the clickable outline. Like a project's rows, the outline and links are kept in the revision's data_json too and the public page shows the published revision's.
- Related links: `presentation_links(presentation_id, label, url, sort)`, an https URL or a path here; the page marks each with its section's glyph.
- The deck is one self-contained HTML file (at most 25 MB), uploaded on the editor below the form and stored at `decks/{entry_id}/index.html` in R2, replacing the last upload at once (no revision). `deck_version` is a hash prefix that versions the iframe URL. The upload counts the slides (the outermost `.slide` elements, else `<section>`) and fills an empty outline from their first h1 to h3; a written outline is kept. Deleting the presentation deletes the deck.
- The deck is served only from the decks origin (`DECKS_ORIGIN`, ADR-007): `{origin}/{entry_id}/?v={version}` to view, `{origin}/{entry_id}/{slug}.html` as a download.
- Writes queue `entry.changed`; a presentation purges its page, the list and feeds, home and About (the talks card).

### book
`books(entry_id, author, isbn, status: want|reading|done|dropped, progress_pct, started_at, finished_at, rating)` (migrations/0017_terms_books.sql). The cover is the entry's `cover_media_id` (any media image), the title the book's title, the summary a short take shown on the list, the body optional notes. A finished book is at 100%. Books have no page of their own: `/{lang}/reading` lists them by status (reading by progress, then want, done, dropped), each at `#{slug}`; the feeds link there. Now shows the books being read from this list; the books typed in the Now form are the fallback when none is, and archived Now versions keep theirs. A book change re-renders the list, its feeds and `/{lang}/now`.

### term (glossary)
`terms(entry_id, term, aliases_json, short_def)` (0017). The entry title is the term (`term` keeps a copy for sorting), `short_def` (at most 300 characters) is the index line and the page lead and the meta description when the summary is empty, the body explains it. `/{lang}/glossary` lists the terms A to Z under their letters (Turkish dotted İ, "#" for digits and signs), `/{lang}/glossary/{slug}` is the term. Terms are not linked automatically in writing (decided 2026-10-19): link them by hand.

### radar_item
`radar_items(group_id, slug, name, quadrant: languages|techniques|tools|platforms, ring: adopt|trial|assess|hold, note_tr, note_en, off)` (migrations/0018_radar.sql) is the working copy the admin edits, one row per item with both languages' notes. Nothing in it is public (decided 2026-10-19: frozen editions only).
`radar_editions(id, label, published_at)`, labels `YYYY-MM` that only grow (`2026-10` after `2026-04`).
`radar_history(group_id, edition_id, slug, name, quadrant, ring, note_tr, note_en, changed_at)` is what each edition showed: freezing copies every item not marked off into it, with its name and notes, so old editions keep their words and outlive a deleted item.
Freezing also publishes the item's entries (kind `radar_item`, one per language in the item's translation group `group_id`, same slug): title the name, summary the note in that language, empty body. An item is new in an edition when it was not on the one before (the first edition has nothing new), moved when its ring differs from the one before. The slug comes from the name when the item is created and stays when it is renamed; a name shaped like an edition label gets an `item-` prefix, since editions live at `/{lang}/radar/{YYYY-MM}`.

### page (singletons and free pages)
`pages(entry_id, template: about|now)` (migrations/0010_pages.sql). The slug is the template, so `UNIQUE (kind, lang, slug)` keeps one About and one Now per language. Structured sections live in `revisions.data_json` (Go struct `page.Data`, validated per template) and go live on publish like the body. `uses` and `generic` join when they get pages.

`page_versions(entry_id, revision_id, published_at)` records every publish; `/now/archive` lists the versions before the live one, one per day (the last of the day), each at `/now/archive/{YYYY-MM-DD}` (noindex).

**now** `data_json`
```json
{
  "place": "istanbul",
  "building": [{ "title": "", "note": "", "stage": "", "pct": 60, "tone": "green", "href": "/en/projects/x", "cover": "media id" }],
  "reading":  [{ "title": "", "author": "", "cover": "media id", "pct": 40, "tone": "orange" }],
  "learning": [{ "title": "", "note": "", "level": 4, "hours": 20, "tone": "indigo" }],
  "not_doing": ["", ""]
}
```
No weather (decided 2026-10-18). Books are typed in the Now form; the reading list (5.5) may replace them with book entries. "Updated N days ago" counts from the latest version; the `now.daily` cron job re-renders `/{lang}/now` every day at 00:07 UTC.

**about** `data_json`: `name` (set in mono in the heading), `portrait` (image), `cv` (a PDF in media), `ctas: [{label, url}]` (https, `mailto:` or a site path; the first is primary, the CV button follows it), `skills: [{group, tone, items[]}]`, `experience: [{from, to, role, org, note, tone}]`. The body is the bio. The link cards beside the timeline come from the live sections (projects, homelab, now), not from data.

## 3. Gallery data (no entry, no detail page)

### photos (500px links)
migrations/0012_photos.sql, `internal/content/photo`. No entries: one list for both languages, edited at `/admin/photos`.
- `photo_albums(id, slug, name_tr, name_en, sort)`: the slug comes from the English name when left empty. An album with photos cannot be deleted.
- `photos(id, album_id, media_id, title_tr, title_en, description_tr, description_en, url_500px, camera, lens, exposure, place, taken_at, row_span 1-3, col_span 1-2, sort, published, created_at, updated_at)`. `description_tr`, `description_en` (0013, plain text up to 2000 characters, line breaks kept) show under the title in the lightbox and in the feeds. At least one title; a title or description missing in one language falls back to the other. The image is any media image (media in use by a photo cannot be deleted); its alt text is the media item's, else the photo title. `url_500px` must start with `https://500px.com/`.
- No embed: the grid and the lightbox show the local image; the lightbox adds EXIF and a "view on 500px" link (decided 2026-10-18). The 500px profile URL is the `photos` site setting.
- Each album row at `/admin/photos` uploads several images at once (`photo.Repo.Upload`, at most 100 MB per request): each goes to media and becomes a photo in that album titled after the file name in both languages, a draft unless "publish now" is checked.
- EXIF (camera, lens, exposure, `taken_at`) is read from the JPEG on save to fill empty fields; typed values are kept, and "Fill from EXIF" previews it without saving. The reader is stdlib only (`photo.ReadEXIF`).
- The grid uses `row_span`/`col_span` (default 2x1); `?album=` filters in place (`filter.js`) and the server renders it as a noindex page.
- Saves and deletes queue `pages.changed` for `/{lang}/photos` and its feeds (`photo.Paths`). Feeds list published photos newest first with the image, linking to `/{lang}/photos#p-{id}`.

### certificates (Credly-style)
migrations/0011_certificates.sql, `internal/content/certificate`. No entries: one list for both languages, edited at `/admin/certificates`.
- `cert_issuers(id, slug, name, tone, sort)`: the tone is one of the entry tones and colors the card's top rule, the badge tint and the chip. An issuer with certificates cannot be deleted.
- `certificates(id, issuer_id, name, skills_json, badge_media_id, credly_url, issued_at, expires_at nullable, sort, published, created_at, updated_at)`. `issued_at` and `expires_at` are the first day of a month (unix UTC). The badge is an image from media with both alt texts; media in use as a badge cannot be deleted.
- Status is derived, not stored: `expired` once the expiry month has begun, `renew_soon` when it begins within 3 months, else `active` (also with no expiry). The `now.daily` job re-renders the certificates pages and About, so statuses follow the date.
- Saves and deletes queue `pages.changed` for `/{lang}/certificates`, its feeds and `/{lang}/about` (`certificate.Paths`). Feeds list published certificates by issue month, linking to the Credly URL when there is one.

## 4. Synced / derived data

### GitHub
The token owner's public data, synced every 6h (migrations/0009_github.sql, `internal/github`). The sync owns these tables.
- `gh_repos(id GitHub node id, full_name owner/name unique, name, description, url, stars, forks, language, topics_json, pushed_at, created_at, archived, fork, gone, pinned, pin_sort, synced_at)`. Repos are upserted, never deleted: one a complete sync no longer sees is marked `gone` and leaves the public pages. `pinned` comes from the GitHub profile on the first sync only (setting `github_pins_seeded`); after that it is set at `/admin/github`. Projects link to a repo by `projects.github_repo` (owner/name, any case), set on the project or at `/admin/github`.
- `gh_languages(repo_id, language, bytes)` replaced per repo on each sync; the language bar sums non-fork, non-gone repos.
- `gh_weekly(week_start, contributions)` 52-week strip (3 intensity buckets computed at render), replaced when the calendar was read.
- `gh_activity(id, type release|repo, repo, title, note, url, occurred_at)` notable activity: published releases (`release:{node id}`, the note is the first line of the description) and new non-fork repos (`repo:{node id}`). Ids are stable, so a re-sync adds nothing twice.
- Search: homelab builds are indexed with kind `homelab` (palette `h:`), software projects with `project` (`pr:`). The Worker reads both kinds for either prefix and keeps the rows whose type in the main database matches, so builds indexed before 4.5 are found without a reindex.
- Synced via GraphQL (`viewer`: pins, contribution calendar, public owned repos, 100 per page, up to 5 pages). The cron Worker enqueues `github.sync` at `17 */6 * * *`; `/admin/github` "run now" enqueues it too. The token is the Worker secret `GITHUB_TOKEN`, sent with each job delivery. Rate limits and network errors retry; a partial answer writes what came and fails the run so `job_runs` shows it. After a sync it queues `pages.changed` for `/{lang}/projects` and linked project pages.

### Stream
Two queries merged in Go (`public.streamRows`), newest first: published public entries (post, note, snippet, link, fav, project, and build logs under a published project) in the page language, and `gh_activity` (the same rows in both languages; repos marked gone are left out). `radar_history` joins in Phase 6. Stream kinds: homelab builds and their logs are `homelab`, software projects, their logs and GitHub rows are `project`. Filterable by kind (`?kind=`). `/{lang}/stream` shows the newest 30 rows; older rows live on month archive pages `/{lang}/stream/{YYYY-MM}` (UTC, at most 500 rows), so every page has a stable KV key. An entry change re-renders the archive pages of the months it was published in before and after (`jobs.Ref.Month`); a GitHub sync re-renders the current month and the months of activity from the last 90 days, linking a repo every month of its activity. The sitemap lists the month pages. A GitHub row opens the published project in that language linked to its repo, else GitHub.

### Site settings
`site_settings(key, value_json)` homelab stats, social links, footer tagline, 500px profile URL, default lang.

## 5. URL scheme

| Kind | Path |
|---|---|
| home | `/{lang}/` |
| post | `/{lang}/posts`, `/{lang}/posts/{slug}` |
| note | `/{lang}/notes`, `/{lang}/notes/{slug}` |
| garden | `/{lang}/garden?stage=evergreen`, `/{lang}/garden/{slug}` |
| runbook | `/{lang}/runbooks`, `/{lang}/runbooks/{slug}` |
| series | `/{lang}/series`, `/{lang}/series/{slug}` |
| snippet | `/{lang}/snippets`, `/{lang}/snippets/{slug}`, `/{lang}/snippets/{slug}/raw/{filename}` (text/plain) |
| link | `/{lang}/links?status=unread`, `/{lang}/awesome` (index), `/{lang}/awesome/{category}` |
| fav | `/{lang}/favs?type=tool` |
| project | `/{lang}/projects`, `/{lang}/projects/{slug}` |
| homelab | `/{lang}/homelab?category=electronics` (detail uses `/projects/{slug}`) |
| buildlog | `/{lang}/projects/{project}/log/{slug}` (in the projects feeds) |
| presentation | `/{lang}/presentations?type=talk`, `/{lang}/presentations/{slug}` |
| deck | `https://decks.halid.org/{entry_id}/` (isolated origin; `decks-staging.halid.org` on staging) |
| photos | `/{lang}/photos?album={slug}` |
| certificates | `/{lang}/certificates?issuer=aws`, `/{lang}/certificates/feed.xml` |
| book | `/{lang}/reading` (each book at `#{slug}`) |
| term | `/{lang}/glossary`, `/{lang}/glossary/{slug}` |
| radar | `/{lang}/radar?edition=2026-10` |
| github | `/{lang}/github` (also embedded in Projects) |
| stream | `/{lang}/stream?kind=note`, `/{lang}/stream/{YYYY-MM}` (month archive) |
| pages | `/{lang}/now`, `/{lang}/now/archive`, `/{lang}/about`, `/{lang}/uses` |
| feeds | `/{lang}/feed.xml`, `/{lang}/feed.json`, `/{lang}/{kind}/feed.xml`, `/{lang}/tags/{tag}/feed.xml` |
| admin | `/admin/login`, `/admin/auth/callback`, `/admin/...` |

## 6. Validation rules (in Go, not only UI)
- `slug` matches `^[a-z0-9-]{1,80}$`
- `published` requires `title` (except `note`) and `published_at`
- `link.url`, `fav.url`, `credly_url`, `embed_url` must be absolute https
- `summary` ≤ 160 chars when used as SEO description
- `radar_item` ring changes reach `radar_history` when an edition is frozen; the working copy is not public
- `project_type = homelab` requires `category_id` from homelab categories
- `presentation.deck_r2_key` must point to a single self-contained HTML file (sniffed as HTML, valid UTF-8, at most 25 MB)
- Deleting an entry with inbound backlinks requires typed slug confirmation and creates a `redirects` row or a 410 tombstone

## 7. Importer contract (Ghost first)
- Input: Ghost JSON export + images zip.
- Map Ghost `posts` → `post` (or `page`), tags → `tags`, lexical/mobiledoc → Markdown, images → R2 with rewritten URLs.
- Old URLs → `redirects` (301). Revisions created with `origin = import`.
- Dry-run report before commit.
