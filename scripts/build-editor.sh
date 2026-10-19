#!/bin/sh
# Builds the vendored JS: the admin's (Tiptap block editor, CodeMirror for
# the Markdown view, htmx) into
# app/web/static/vendor, and Mermaid's ESM build into app/web/static/mermaid
# for posts with diagrams. The outputs are committed: the Go image embeds
# them and has no Node step.
#   cd editor && npm ci && npm run build
set -eu
cd "$(dirname "$0")/../editor"
out=../app/web/static/vendor
mkdir -p "$out"
npx esbuild src/cm.js --bundle --minify --format=esm --target=es2020 --legal-comments=eof --outfile="$out/cm.js"
npx esbuild src/tt.js --bundle --minify --format=esm --target=es2020 --legal-comments=eof --outfile="$out/tt.js"
cp node_modules/htmx.org/dist/htmx.min.js "$out/htmx.min.js"
cp node_modules/htmx.org/LICENSE "$out/htmx.LICENSE"
ls -l "$out"

# Mermaid: the ESM entry and its chunks (one per diagram type, loaded on
# demand) in one flat directory, since /static serves {dir}/{file}. The
# entry's imports are rewritten to match; the chunks import each other
# from the same directory already. No source maps.
mm=../app/web/static/mermaid
rm -rf "$mm"
mkdir -p "$mm"
src=node_modules/mermaid/dist
sed 's#\./chunks/mermaid\.esm\.min/#./#g' "$src/mermaid.esm.min.mjs" | sed '/^\/\/# sourceMappingURL=/d' > "$mm/mermaid.esm.min.mjs"
for f in "$src"/chunks/mermaid.esm.min/*.mjs; do
  sed '/^\/\/# sourceMappingURL=/d' "$f" > "$mm/$(basename "$f")"
done
cp node_modules/mermaid/LICENSE "$mm/LICENSE"
du -sh "$mm"
