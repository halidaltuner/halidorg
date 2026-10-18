#!/bin/sh
# Builds the admin's vendored JS into app/web/static/vendor. The outputs are
# committed: the Go image embeds them and has no Node step.
#   cd editor && npm ci && npm run build
set -eu
cd "$(dirname "$0")/../editor"
out=../app/web/static/vendor
mkdir -p "$out"
npx esbuild src/cm.js --bundle --minify --format=esm --target=es2020 --legal-comments=eof --outfile="$out/cm.js"
cp node_modules/htmx.org/dist/htmx.min.js "$out/htmx.min.js"
cp node_modules/htmx.org/LICENSE "$out/htmx.LICENSE"
ls -l "$out"
