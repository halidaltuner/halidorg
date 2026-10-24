# Runbook: backups and restore

Phase 9.1. What is backed up, where, and how to get it back.

## What exists

| What | When | Where | Kept |
|---|---|---|---|
| SQL dump of the main D1 database (`halidorg-{env}`) | nightly 02:23 UTC, `.github/workflows/backup.yml` | `halidorg-backups-{env}/d1/YYYY-MM-DD.sql.gz` | 30 days |
| Full export: entries as Markdown, every table as JSON, media manifest | Sundays 01:41 UTC, `export.weekly` job (Worker cron) | `halidorg-backups-{env}/exports/YYYY-MM-DD.zip` | 90 days |
| Media (images, PDFs, decks, OG cards, favicons, avatars) | as uploaded | `halidorg-media-{env}` | not copied |

- The backups bucket is never served: no Worker route reads it, only the container writes exports through the bridge (`/backups/...`). `scripts/backup-bucket.sh` creates it and its lifecycle rules (`d1-30d`, `exports-90d`); deploy and backup run it every time.
- The SEARCH database (`halidorg-search-{env}`) is not backed up. It is derived from the main database and rebuilt by the `search.rebuild` job (Settings, "rebuild search index").
- KV pages are not backed up either: every page re-renders from the database on its next request.
- R2 media is not copied. The export's `media/manifest.json` lists every key with its alt texts. Use R2 object versioning or a bucket copy if media loss becomes a concern.

## Restore test (do this once per env, and after schema changes)

GitHub → Actions → **restore-test** → Run workflow → env, day (empty for today).

The job downloads that day's dump, creates `halidorg-restore-test`, loads the dump, prints the row counts next to the live database's and deletes the scratch database. It fails if the dump does not load.

Locally, from `worker/` with `CLOUDFLARE_API_TOKEN` and `CLOUDFLARE_ACCOUNT_ID` set:

```sh
sh ../scripts/restore-d1.sh staging 2026-10-25
```

## Restoring for real

1. Pick the dump: the last day before the damage (`wrangler r2 object get halidorg-backups-prod/d1/2026-10-25.sql.gz --file x.sql.gz --remote` to look at one).
2. Restore it into a **new** database, keep it:

   ```sh
   sh ../scripts/restore-d1.sh prod 2026-10-25 halidorg-prod-2
   ```

   The script refuses an existing target, so the damaged database stays as it was until you switch.
3. Point the env's `DB` binding in `wrangler.jsonc` at the new database (`database_name` and `database_id` from `wrangler d1 info halidorg-prod-2`), commit, and deploy (Actions → deploy → prod). Migrations apply on top; the dump includes `d1_migrations`, so only newer ones run.
4. Rebuild the search index: Admin → Settings → "rebuild search index". Until it finishes, search finds only what is already indexed.
5. Refresh the edge: the deploy already runs `scripts/refresh-edge.sh`. Pages for entries the restore removed go when their next re-render 404s.
6. Check: home, a post, the admin dashboard, a search.
7. When it has run for a while, delete the damaged database (`wrangler d1 delete halidorg-prod -y`) or keep it renamed for a post-mortem.

Rows written between the dump and the switch are lost; the weekly export or the damaged database may still have them.

## API token

The deploy token (`CLOUDFLARE_API_TOKEN`) needs, besides Workers and D1 edit: **Workers R2 Storage: Edit** (bucket, lifecycle rules and object put/get).
