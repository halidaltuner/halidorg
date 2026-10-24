#!/bin/sh
# Nightly backup (Phase 9.1): a full SQL dump of the env's main database,
# gzipped into its backups bucket as d1/{YYYY-MM-DD}.sql.gz. The SEARCH
# database is not dumped: it is derived and rebuilt from the main one
# (docs/runbook-restore.md). Runs from worker/.
#   CLOUDFLARE_API_TOKEN=… CLOUDFLARE_ACCOUNT_ID=… scripts/backup-d1.sh staging
set -eu
env=${1:?usage: backup-d1.sh staging|prod}
day=$(date -u +%F)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
npx wrangler d1 export DB --remote --env "$env" -c ../wrangler.jsonc --output "$tmp/dump.sql" --skip-confirmation
# A dump without the schema of entries is not a backup of this site.
grep -q 'CREATE TABLE.*entries' "$tmp/dump.sql" || { echo "dump has no entries table" >&2; exit 1; }
gzip -9 "$tmp/dump.sql"
npx wrangler r2 object put "halidorg-backups-$env/d1/$day.sql.gz" --file "$tmp/dump.sql.gz" \
	--content-type application/gzip --remote
echo "backed up halidorg-$env to halidorg-backups-$env/d1/$day.sql.gz ($(wc -c <"$tmp/dump.sql.gz") bytes)"
