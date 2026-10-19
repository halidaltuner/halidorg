## Staging check: 123/129 passed

| | check | detail |
|---|---|---|
| ✅ | Access: service token reaches the app | status 200 |
| ✅ | KV: /en/ second request is a HIT | 200 HIT → 200 HIT |
| ✅ | Fresh: /en/ has the current templates (palette.js) | deploy in footer: f69792d |
| ✅ | KV: /tr/ second request is a HIT | 200 HIT → 200 HIT |
| ✅ | Fresh: /tr/ has the current templates (palette.js) | deploy in footer: f69792d |
| ✅ | KV: /en/posts second request is a HIT | 200 STALE → 200 HIT |
| ✅ | Fresh: /en/posts has the current templates (palette.js) | deploy in footer: f69792d |
| ✅ | Posts: the list has published posts | test, sample-short-note, sample-long-read, sample-renderer-tour |
| ❌ | KV: /en/posts/test second request is a HIT | 200 STALE → 200 STALE · origin 200 build f69792dc5329c373396aebfbc39dbc9010e0a1cd cc public, max-age=0, must-revalidate ct text/html; charset=utf-8 |
| ✅ | Fresh: /en/posts/test has the current templates (palette.js) | deploy in footer: c843e2e |
| ✅ | Post: og:image meta present | https://staging.halid.org/og/en/posts/test.png?v=3e9d48e4d7e7 |
| ✅ | OG: card served from R2 by the Worker | 200 image/png public, max-age=31536000, immutable |
| ✅ | Post: canonical and hreflang |  |
| ✅ | Fallback: /tr/posts/{en-slug} is the banner page or a 301 to the translation | status 200 |
| ✅ | Search: "test" finds /en/posts/test | 0 rows |
| ✅ | /en/feed.xml 200 application/rss+xml | application/rss+xml; charset=utf-8 |
| ✅ | /en/feed.json 200 application/feed+json | application/feed+json; charset=utf-8 |
| ✅ | /sitemap.xml 200 application/xml | application/xml; charset=utf-8 |
| ✅ | /robots.txt 200 text/plain | text/plain; charset=utf-8 |
| ✅ | robots.txt disallows everything on staging |  |
| ✅ | 404: styled page, not cached | 404 BYPASS |
| ✅ | Admin: needs a session (redirects to login) | 303 /admin/login |
| ✅ | KV: /en/projects second request is a HIT | 200 STALE → 200 HIT |
| ✅ | Fresh: /en/projects has the current templates (palette.js) | deploy in footer: f69792d |
| ✅ | KV: /en/homelab second request is a HIT | 200 STALE → 200 HIT |
| ✅ | Fresh: /en/homelab has the current templates (palette.js) | deploy in footer: f69792d |
| ✅ | KV: /en/stream second request is a HIT | 200 HIT → 200 HIT |
| ✅ | Fresh: /en/stream has the current templates (palette.js) | deploy in footer: f69792d |
| ✅ | KV: /en/github second request is a HIT | 200 STALE → 200 HIT |
| ✅ | Fresh: /en/github has the current templates (palette.js) | deploy in footer: f69792d |
| ✅ | Projects: the list has software projects | sample-project-site |
| ✅ | Homelab: the page lists builds | sample-project-rack |
| ✅ | KV: /en/projects/sample-project-site second request is a HIT | 200 STALE → 200 HIT |
| ✅ | Fresh: /en/projects/sample-project-site has the current templates (palette.js) | deploy in footer: f69792d |
| ✅ | Project (software): its OG card is served | /og/en/projects/sample-project-site.png 200 |
| ✅ | KV: /en/projects/sample-project-rack second request is a HIT | 200 STALE → 200 HIT |
| ✅ | Fresh: /en/projects/sample-project-rack has the current templates (palette.js) | deploy in footer: f69792d |
| ✅ | Project (homelab): its OG card is served | /og/en/projects/sample-project-rack.png 200 |
| ✅ | KV: /en/projects/sample-project-rack/log/sample-log-brackets second request is a HIT | 200 STALE → 200 HIT |
| ✅ | Fresh: /en/projects/sample-project-rack/log/sample-log-brackets has the current templates (palette.js) | deploy in footer: f69792d |
| ✅ | Build log: page under its project with a back link | /en/projects/sample-project-rack/log/sample-log-brackets |
| ✅ | /en/projects/feed.xml 200 application/rss+xml |  |
| ✅ | Sitemap lists /en/projects |  |
| ✅ | Sitemap lists /en/homelab |  |
| ✅ | Sitemap lists /en/stream |  |
| ✅ | Sitemap lists /en/github |  |
| ✅ | Sitemap lists a project page |  |
| ✅ | GitHub: /en/github shows synced data |  |
| ✅ | GitHub: the projects page has the GitHub section |  |
| ✅ | GitHub: the activity list has rows |  |
| ✅ | Stream: project or homelab rows | 27 rows |
| ✅ | Home: stream links to /en/stream |  |
| ✅ | Stream: ?kind=homelab shows only homelab rows |  |
| ✅ | Palette: "pr: sample" finds project rows only | project /en/projects/sample-project-site |
| ✅ | Palette: "h: sample" finds homelab rows only | homelab /en/projects/sample-project-rack |
| ✅ | Admin: /admin/github needs a session | 303 |
| ✅ | KV: /en/about second request is a HIT | 200 STALE → 200 HIT |
| ✅ | Fresh: /en/about has the current templates (palette.js) | deploy in footer: f69792d |
| ✅ | /en/about: has content |  |
| ✅ | KV: /en/now second request is a HIT | 200 STALE → 200 HIT |
| ✅ | Fresh: /en/now has the current templates (palette.js) | deploy in footer: f69792d |
| ✅ | /en/now: has content |  |
| ✅ | KV: /en/now/archive second request is a HIT | 200 STALE → 200 HIT |
| ✅ | Fresh: /en/now/archive has the current templates (palette.js) | deploy in footer: f69792d |
| ✅ | /en/now/archive: has content |  |
| ✅ | KV: /en/certificates second request is a HIT | 200 STALE → 200 HIT |
| ✅ | Fresh: /en/certificates has the current templates (palette.js) | deploy in footer: f69792d |
| ❌ | /en/certificates: has content |  |
| ✅ | KV: /en/photos second request is a HIT | 200 STALE → 200 HIT |
| ✅ | Fresh: /en/photos has the current templates (palette.js) | deploy in footer: f69792d |
| ✅ | /en/photos: has content |  |
| ✅ | KV: /en/presentations second request is a HIT | 200 STALE → 200 HIT |
| ✅ | Fresh: /en/presentations has the current templates (palette.js) | deploy in footer: f69792d |
| ✅ | /en/presentations: has content |  |
| ❌ | KV: /tr/about second request is a HIT | 404 BYPASS → 404 BYPASS |
| ✅ | KV: /en/presentations/dpdk second request is a HIT | 200 STALE → 200 HIT |
| ✅ | Fresh: /en/presentations/dpdk has the current templates (palette.js) | deploy in footer: f69792d |
| ✅ | Deck: iframe on the decks origin, not the main one | https://decks-staging.halid.org |
| ✅ | Deck: served without Access, framed only by the site | 200 frame-ancestors 'self' https://staging.halid.org |
| ✅ | Deck: not served from the main origin | 404 |
| ✅ | About: certificates card links /en/certificates |  |
| ✅ | KV: /en/garden second request is a HIT | 200 STALE → 200 HIT |
| ✅ | Fresh: /en/garden has the current templates (palette.js) | deploy in footer: f69792d |
| ✅ | KV: /en/runbooks second request is a HIT | 200 STALE → 200 HIT |
| ✅ | Fresh: /en/runbooks has the current templates (palette.js) | deploy in footer: f69792d |
| ✅ | KV: /en/series second request is a HIT | 200 STALE → 200 HIT |
| ✅ | Fresh: /en/series has the current templates (palette.js) | deploy in footer: f69792d |
| ✅ | KV: /en/glossary second request is a HIT | 200 STALE → 200 HIT |
| ✅ | Fresh: /en/glossary has the current templates (palette.js) | deploy in footer: f69792d |
| ✅ | KV: /en/reading second request is a HIT | 200 STALE → 200 HIT |
| ✅ | Fresh: /en/reading has the current templates (palette.js) | deploy in footer: f69792d |
| ✅ | Post: backlinks block (and related, when tags are shared) | no tag-mates |
| ✅ | /en/presentations/feed.xml 200 application/rss+xml | 200 |
| ✅ | /en/photos/feed.xml 200 application/rss+xml | 200 |
| ✅ | /en/certificates/feed.xml 200 application/rss+xml | 200 |
| ✅ | /en/glossary/feed.xml 200 application/rss+xml | 200 |
| ✅ | /en/reading/feed.xml 200 application/rss+xml | 200 |
| ✅ | Sitemap lists /en/about |  |
| ✅ | Sitemap lists /en/now |  |
| ✅ | Sitemap lists /en/certificates |  |
| ✅ | Sitemap lists /en/photos |  |
| ✅ | Sitemap lists /en/presentations |  |
| ✅ | Sitemap lists /en/garden |  |
| ✅ | Sitemap lists /en/glossary |  |
| ✅ | Sitemap lists /en/reading |  |
| ✅ | Photos: media served by the Worker | /media/01M58HK1SESZ0VB78KP1B97B9K/screenshot-2026-10-19-at-01-07-24.png 200 image/png |
| ✅ | Admin: /admin/presentations needs a session | 303 |
| ✅ | Admin: /admin/photos needs a session | 303 |
| ✅ | Admin: /admin/certificates needs a session | 303 |
| ✅ | Admin: /admin/series needs a session | 303 |
| ✅ | Admin: /admin/pages needs a session | 303 |
| ✅ | KV: /en/radar second request is a HIT | 200 HIT → 200 HIT |
| ✅ | Fresh: /en/radar has the current templates (palette.js) | deploy in footer: f69792d |
| ❌ | Radar: at least two editions in the switcher |  |
| ✅ | Radar: SVG with a text alternative and numbered blips |  |
| ✅ | Radar: list rows link item pages |  |
| ❌ | Radar: new or moved marks against the edition before | 0 new, 0 moved |
| ✅ | Radar: blip positions stable across renders |  |
| ✅ | KV: /en/radar/go second request is a HIT | 200 HIT → 200 HIT |
| ✅ | Fresh: /en/radar/go has the current templates (palette.js) | deploy in footer: f69792d |
| ✅ | Radar item: history across editions | /en/radar/go |
| ❌ | Stream: a row per radar edition | 1 rows |
| ✅ | /en/radar/feed.xml 200 application/rss+xml | 200 |
| ✅ | Sitemap lists /en/radar |  |
| ✅ | Header links the radar |  |
| ✅ | Admin: /admin/radar needs a session | 303 |
| ✅ | Palette: ⌘K opens and lists results | 2 rows |
| ✅ | JS errors at 1280px |  |
| ✅ | JS errors at 390px |  |
