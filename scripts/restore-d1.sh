#!/bin/sh
# Restores a nightly dump into a D1 database (docs/runbook-restore.md).
# Without a target it restores into a fresh scratch database and compares
# row counts with the source: the restore test. Runs from worker/.
#   scripts/restore-d1.sh staging 2026-10-25                   test: into halidorg-restore-test, compare, delete
#   scripts/restore-d1.sh staging 2026-10-25 halidorg-staging-2 restore into that (new, empty) database, keep it
set -eu
env=${1:?usage: restore-d1.sh staging|prod YYYY-MM-DD [target]}
day=${2:?usage: restore-d1.sh staging|prod YYYY-MM-DD [target]}
target=${3:-halidorg-restore-test}
scratch=$([ -z "${3:-}" ] && echo 1 || echo 0)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
wr() { npx wrangler "$@"; }

wr r2 object get "halidorg-backups-$env/d1/$day.sql.gz" --file "$tmp/dump.sql.gz" --remote
gunzip "$tmp/dump.sql.gz"
if wr d1 info "$target" >/dev/null 2>&1; then
	if [ "$scratch" = 1 ]; then
		wr d1 delete "$target" -y
	else
		echo "$target already exists; restore into a new database, then point wrangler.jsonc at it" >&2
		exit 1
	fi
fi
wr d1 create "$target"
wr d1 execute "$target" --remote --file "$tmp/dump.sql" -y

count() { # db table -> rows
	wr d1 execute "$1" --remote --json --command "SELECT COUNT(*) AS n FROM $2" | grep -o '"n": *[0-9]*' | grep -o '[0-9]*$'
}
fail=0
for t in entries revisions media tags job_runs; do
	got=$(count "$target" "$t")
	echo "$t: $got rows restored"
	[ -n "$got" ] || fail=1
done
# The scratch test also compares with the live database: equal unless it
# changed since the dump.
if [ "$scratch" = 1 ]; then
	for t in entries media; do
		live=$(count "halidorg-$env" "$t")
		got=$(count "$target" "$t")
		echo "$t: live $live, restored $got"
	done
	wr d1 delete "$target" -y
fi
exit $fail
