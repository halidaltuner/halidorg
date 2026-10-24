#!/bin/sh
# Makes sure the backups bucket of an env exists with its retention rules
# (Phase 9.1): d1/ dumps for 30 days, exports/ zips for 90. Safe to run on
# every deploy and backup. Runs from worker/ (wrangler from npm ci).
#   CLOUDFLARE_API_TOKEN=… CLOUDFLARE_ACCOUNT_ID=… scripts/backup-bucket.sh staging
set -eu
env=${1:?usage: backup-bucket.sh staging|prod}
bucket="halidorg-backups-$env"
wr() { npx wrangler "$@"; }
if ! wr r2 bucket info "$bucket" >/dev/null 2>&1; then
	wr r2 bucket create "$bucket"
fi
rules=$(wr r2 bucket lifecycle list "$bucket" 2>/dev/null || true)
case "$rules" in *d1-30d*) ;; *) wr r2 bucket lifecycle add "$bucket" d1-30d d1/ --expire-days 30 --force ;; esac
case "$rules" in *exports-90d*) ;; *) wr r2 bucket lifecycle add "$bucket" exports-90d exports/ --expire-days 90 --force ;; esac
