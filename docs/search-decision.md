# Search: D1 FTS5 or Pagefind

Status: **accepted** 2026-10-17, as ADR-010 in ARCHITECTURE.md (option A). The command palette needs search from Phase 1: posts first, then every kind with prefixes (`n: s: l: f: pr: h:`), in Turkish and English.

## What was tested (local D1, 2026-10-17)

| Check | Result |
|---|---|
| FTS5 in D1 | Supported, including `snippet()` for highlighted excerpts. |
| `unicode61 remove_diacritics 2` | `olcek*` finds "ölçeklendirmek", `kubernetes` finds "Kubernetes'e". Diacritics fold: ç→c, ğ→g, ö→o, ş→s, ü→u. |
| Turkish ı / İ / I | **Broken without help.** `istanbul` finds "İstanbul", but `ilik` does not find "ılık", and "Iğdır" is not found by `igdir` or `ığdır`. SQLite folds `I` to `i` and leaves `ı` alone. |
| `trigram` | Substring matches work (`ümes` finds "kümesi"), no diacritic folding. |

The ı problem has a small fix on our side: fold `ı`, `İ` and `I` to `i` in Go, both in the indexed text and in the query.

## The constraint that decides FTS5's shape

Cloudflare's D1 docs: "Export is not supported for virtual tables, including databases with virtual tables." One FTS5 table in the main database breaks the nightly `wrangler d1 export` in ARCHITECTURE.md §6 for the whole database. Their workaround is to drop the virtual tables, export, and recreate them.

So FTS5 should live in **its own D1 database** (`halidorg-search-{env}`, binding `SEARCH`). It holds only derived data, rebuilt from the main database by a job, so it needs no backup, and the main database keeps exporting.

## Options

| | A. FTS5 in a separate D1 | B. Pagefind |
|---|---|---|
| Where search runs | Worker queries `SEARCH` directly: at the edge, no container | In the browser: WASM plus index chunks from R2 |
| Index update | One row per entry, written on publish | Re-index the whole site per publish: dump rendered HTML, run the Pagefind binary in the container, upload the chunks to R2 |
| Turkish | Diacritic folding, our ı/İ folding, prefix matching (`ölçek*`). No stemming. | Snowball stemming for Turkish and English |
| Admin search (drafts, private entries) | Same index with a status filter | Not possible: the index is public |
| Excerpts, ranking | `snippet()`, `bm25()` with column weights | Built in |
| Extra moving parts | One D1 database, one job | A Rust binary in the image, an R2 prefix, a client bundle of a few hundred KB |
| Public reads without a warm container | Yes (Worker + D1) | Yes (static) |

## Recommendation

**A, FTS5 in a separate D1.** Admin search over drafts rules Pagefind out on its own, and per-entry updates on publish fit render-on-publish without a full-site rebuild. The cost is Turkish stemming: "kümeler" finds "küme" only through prefix search (`küme*`), which the palette does by default as the user types.

If stemming turns out to matter more than expected, Pagefind can be added later for public search only, with FTS5 kept for admin.
