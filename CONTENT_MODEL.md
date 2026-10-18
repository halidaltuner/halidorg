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
- `tags(id, slug, name_tr, name_en, tone)` + `entry_tags(entry_id, tag_id)`
- `categories(id, kind, slug, name_tr, name_en, tone, parent_id, sort)` links, snippets, favs, homelab builds, awesome lists (migrations/0006_short_form.sql). `kind` is one of link, snippet, fav, homelab, awesome; `(kind, slug)` is unique; `tone` is one of the DESIGN.md tones. An entry's `category_id` must point at a category of its own kind (checked in Go).
- `series(id, slug, title_tr, title_en, summary, status)`
- `links_graph(from_entry_id, to_entry_id, context)` backlinks (migrations/0004_links_graph.sql). The entry.changed job rebuilds an entry's outgoing rows from its published HTML: links to `/{lang}/posts/{slug}` (relative or on the site origin), `context` is the link text. Unlisted and deleted entries have no outgoing rows.
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
`garden_pages(entry_id, maturity: seedling|budding|evergreen, last_tended_at)`

### runbook / cheatsheet
`runbooks(entry_id, platform, prerequisites_md, verified_on, risk: low|med|high)`

### snippet
`snippets(entry_id, category_id, gist_url nullable)`
`snippet_files(snippet_id, filename, language, code, sort)` one or more files, shown as tabs (0006). A snippet needs at least one file to publish; filenames are unique per snippet and have no slashes. Each revision stores the files in `data_json` (`{"files":[...]}`), so revision diffs and restores include them; the table holds the current files.

### link
`links(entry_id, url, domain, favicon_url, category_id, via nullable, archived_url nullable, read_status: unread|read)`
`title` = link title, `summary` = my comment. `domain` is derived from `url` (without `www.`). No item page: links live on `/{lang}/links`; search results open the link itself (0006).

### fav
`favs(entry_id, fav_type: tool|repo|video|product|article|book|place|other, author nullable, url nullable, rating 1-5 nullable, category_id)` (0006). No item page: favs live on `/{lang}/favs`.

### awesome list
A `page` with `data_json.list_category_ids`. No separate table.

### project (software and homelab/DIY share one model and one detail template)
`projects(entry_id, project_type: software|homelab, status: active|paused|archived|idea|done, category_id nullable, tagline, repo_url, demo_url, github_repo nullable, started_at, ended_at, featured bool, cost_total nullable, currency default 'TRY', time_spent_hours nullable)`
- `project_facts(project_id, key, value, sort)` fact strip cells (e.g. `nodes: 3`, `idle: 38 W`)
- `project_stack(project_id, name, sort)` "stack" aside for software
- `project_parts(project_id, name, qty, price, currency, url nullable, sort)` "parts" aside for homelab
- `project_gallery(project_id, media_id, sort)`
- Homelab categories (`categories.kind = 'homelab'`): homelab, electronics, 3d print, woodwork, network
- Homelab stats tiles (builds, nodes, idle W, uptime): `builds` is computed; the rest live in `site_settings` as manual values

### buildlog
`buildlogs(entry_id, project_entry_id, log_date, hours_spent nullable)` rendered as the project timeline

### presentation (replaces `talk`)
`presentations(entry_id, pres_type: talk|workshop|lightning, event, city, date, talk_lang, slide_count, deck_r2_key, recording_url nullable, outline_json)`
- `outline_json`: `[{ "n": 3, "title": "Why render-on-publish" }]` drives the clickable outline
- `deck_r2_key`: self-contained HTML deck, served from an isolated origin (ADR-007)
- Related links: `presentation_links(presentation_id, label, url, sort)`

### book
`books(entry_id, author, isbn, cover_media_id, status: want|reading|done|dropped, progress_pct, started_at, finished_at, rating)`

### term (glossary)
`terms(entry_id, term, aliases_json, short_def)`

### radar_item
`radar_items(entry_id, quadrant: languages|techniques|tools|platforms, ring: adopt|trial|assess|hold)`
`radar_history(radar_entry_id, edition_id, ring, changed_at, note)` drives new (triangle) and moved (dashed line)
`radar_editions(id, label, published_at)` labels like `2026-10`

### page (singletons and free pages)
`pages(entry_id, template: now|about|uses|awesome|generic)`. Structured sections live in `revisions.data_json`, validated by a Go schema per template.

**now** `data_json`
```json
{
  "place": "Ankara",
  "building": [{ "title": "", "note": "", "stage": "", "pct": 60, "tone": "green", "href": "/en/projects/x", "cover_media_id": null }],
  "reading":  [{ "book_entry_id": "", "pct": 40 }],
  "learning": [{ "title": "", "note": "", "level": 4, "hours": 20, "tone": "indigo" }],
  "not_doing": ["", ""]
}
```
Weather in the hero is fetched at render time and cached; it is not stored.
`/now/archive` lists past published revisions.

**about** `data_json`: `bio_md`, `portrait_media_id`, `cv_media_id`, `skills: [{group, tone, items[]}]`, `experience: [{org, role, from, to, note, tone}]`, `ctas: [{label, url}]`.

## 3. Gallery data (no entry, no detail page)

### photos (500px embeds)
- `photo_albums(id, slug, name_tr, name_en, sort)`
- `photos(id, album_id, title, embed_url, image_url, camera, lens, exposure, place, taken_at, row_span 1-3, col_span 1-2, sort, published bool)`
- The grid uses `row_span`/`col_span`; the lightbox shows EXIF and "view on 500px".

### certificates (Credly-style)
- `cert_issuers(id, slug, name, tone)` AWS, CNCF, HashiCorp, Google, Linux Foundation
- `certificates(id, issuer_id, name, skills_json, badge_media_id, credly_url, issued_at, expires_at nullable, sort, published bool)`
- Status is derived, not stored: `active`, `renew_soon` (expires within 3 months), `expired`.

## 4. Synced / derived data

### GitHub
- `gh_repos(id, name, description, stars, forks, language, topics_json, pushed_at, archived, pinned bool, project_entry_id nullable)`
- `gh_activity(id, type, repo, title, url, occurred_at)`
- `gh_weekly(week_start, contributions)` 52-week strip (3 intensity buckets computed at render)
- `gh_languages(repo_id, language, bytes)`
- Synced every 6h via GraphQL, token in Worker secrets.

### Stream
A D1 view unioning published `entries` + `gh_activity` + `radar_history`, cursor-paginated, filterable by kind (`?kind=`).

### Site settings
`site_settings(key, value_json)` homelab stats, social links, footer tagline, 500px profile URL, default lang.

## 5. URL scheme

| Kind | Path |
|---|---|
| home | `/{lang}/` |
| post | `/{lang}/posts`, `/{lang}/posts/{slug}` |
| note | `/{lang}/notes`, `/{lang}/notes/{slug}` |
| garden | `/{lang}/garden/{slug}` |
| runbook | `/{lang}/runbooks/{slug}` |
| snippet | `/{lang}/snippets`, `/{lang}/snippets/{slug}` |
| link | `/{lang}/links?status=unread` |
| fav | `/{lang}/favs?type=tool` |
| project | `/{lang}/projects`, `/{lang}/projects/{slug}` |
| homelab | `/{lang}/homelab?category=electronics` (detail uses `/projects/{slug}`) |
| buildlog | `/{lang}/projects/{project}/log/{slug}` |
| presentation | `/{lang}/presentations?type=talk`, `/{lang}/presentations/{slug}` |
| deck | `https://decks.halid.org/{slug}/` (isolated origin) |
| photos | `/{lang}/photos?album={slug}` |
| certificates | `/{lang}/certificates?issuer=aws` |
| book | `/{lang}/reading` |
| term | `/{lang}/glossary/{slug}` |
| radar | `/{lang}/radar?edition=2026-10` |
| github | `/{lang}/github` (also embedded in Projects) |
| stream | `/{lang}/stream?kind=note` |
| pages | `/{lang}/now`, `/{lang}/now/archive`, `/{lang}/about`, `/{lang}/uses` |
| feeds | `/{lang}/feed.xml`, `/{lang}/feed.json`, `/{lang}/{kind}/feed.xml`, `/{lang}/tags/{tag}/feed.xml` |
| admin | `/admin/login`, `/admin/auth/callback`, `/admin/...` |

## 6. Validation rules (in Go, not only UI)
- `slug` matches `^[a-z0-9-]{1,80}$`
- `published` requires `title` (except `note`) and `published_at`
- `link.url`, `fav.url`, `credly_url`, `embed_url` must be absolute https
- `summary` ≤ 160 chars when used as SEO description
- `radar_item` ring change always writes `radar_history`
- `project_type = homelab` requires `category_id` from homelab categories
- `presentation.deck_r2_key` must point to a single self-contained HTML file
- Deleting an entry with inbound backlinks requires typed slug confirmation and creates a `redirects` row or a 410 tombstone

## 7. Importer contract (Ghost first)
- Input: Ghost JSON export + images zip.
- Map Ghost `posts` → `post` (or `page`), tags → `tags`, lexical/mobiledoc → Markdown, images → R2 with rewritten URLs.
- Old URLs → `redirects` (301). Revisions created with `origin = import`.
- Dry-run report before commit.
