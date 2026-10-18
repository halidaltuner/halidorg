#!/bin/sh
# After a deploy, request every public page once so the Worker re-renders
# pages stored by the previous deploy (stale-while-revalidate, see
# worker/src/cache.ts isStale). Visitors never wait either way; this only
# makes the first visitor see the new templates too.
#   ORIGIN=https://staging.halid.org [CF_ACCESS_CLIENT_ID=… CF_ACCESS_CLIENT_SECRET=…] scripts/refresh-edge.sh
set -eu
origin=${ORIGIN:?ORIGIN is required}
set -- -sS --max-time 20
if [ -n "${CF_ACCESS_CLIENT_ID:-}" ]; then
	set -- "$@" -H "CF-Access-Client-Id: $CF_ACCESS_CLIENT_ID" -H "CF-Access-Client-Secret: $CF_ACCESS_CLIENT_SECRET"
fi
# Give the new Worker version a moment to take traffic everywhere.
sleep 20
urls=$(curl "$@" "$origin/sitemap.xml" | grep -o '<loc>[^<]*</loc>' | sed 's#<loc>##; s#</loc>##; s#^https\?://[^/]*##' || true)
n=0; stale=0
for path in /en/ /tr/ /en/feed.xml /tr/feed.xml /en/atom.xml /tr/atom.xml /en/feed.json /tr/feed.json /sitemap.xml /robots.txt $urls; do
	cache=$(curl "$@" -o /dev/null -D - "$origin$path" | tr -d '\r' | sed -n 's/^[Xx]-[Cc]ache: //p')
	n=$((n + 1))
	[ "$cache" = "STALE" ] && stale=$((stale + 1))
done
echo "requested $n pages, $stale were from an older deploy and are re-rendering"
