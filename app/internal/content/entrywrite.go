package content

import (
	"context"
	"net/url"
	"strings"

	"halid.org/engine/internal/store"
)

// Shared write paths for kinds that keep one row (or a few) of their own next
// to the entry. post predates these and keeps its own copy in post/repo.go.

// slugTaken maps a UNIQUE failure on entries (kind, lang, slug) to the error
// the editor shows; other errors pass through.
func slugTaken(err error) error {
	if err != nil && strings.Contains(strings.ToLower(err.Error()), "unique constraint") {
		return ValidationError{"slug": "already used by another entry of this kind in this language"}
	}
	return err
}

// CreateEntry inserts e (as a draft unless it says otherwise; never as
// published, which only Publish sets), its first revision, its own rows and
// its tags in one batch. own receives the new entry id.
func CreateEntry(ctx context.Context, db store.DB, e *Entry, b Body, tags []string, own func(id string) []store.Stmt) error {
	if e.ID == "" {
		e.ID = NewID()
	}
	if e.Status == "" || e.Status == Published {
		e.Status = Draft
	}
	if e.Visibility == "" {
		e.Visibility = Public
	}
	stmts := InsertEntry(e, b, OriginManual)
	stmts = append(stmts, own(e.ID)...)
	stmts = append(stmts, SetTagsStmts(e.ID, tags)...)
	_, err := db.Batch(ctx, stmts...)
	return slugTaken(err)
}

// SaveEntry writes the entry fields, its own rows and tags, then the body as
// a revision (autosaves coalesce, SaveRevision). A save never publishes: a
// published status is kept only if the entry already has it (cur).
func SaveEntry(ctx context.Context, db store.DB, cur Entry, e *Entry, b Body, origin string, tags []string, own []store.Stmt) (string, error) {
	if e.Status == Published && cur.Status != Published {
		e.Status = cur.Status
	}
	e.PublishedAt = cur.PublishedAt
	stmts := []store.Stmt{UpdateEntry(e)}
	stmts = append(stmts, own...)
	stmts = append(stmts, SetTagsStmts(e.ID, tags)...)
	if _, err := db.Batch(ctx, stmts...); err != nil {
		return "", slugTaken(err)
	}
	rev, err := SaveRevision(ctx, db, e.ID, b, origin)
	if err == nil {
		e.CurrentRevisionID = &rev
	}
	return rev, err
}

// PublishEntry validates e as published (validate is the kind's check) and
// points its published revision at the current one.
func PublishEntry(ctx context.Context, db store.DB, e Entry, validate func(Entry) error) error {
	now := Clock().Unix()
	e.Status = Published
	if e.PublishedAt == nil {
		e.PublishedAt = &now
	}
	if err := validate(e); err != nil {
		return err
	}
	if e.CurrentRevisionID == nil {
		return ValidationError{"body": "nothing to publish"}
	}
	_, err := db.Batch(ctx, PublishStmts(e.ID, *e.CurrentRevisionID, now)...)
	return err
}

// ArchiveEntry takes an entry off the site and keeps it.
func ArchiveEntry(ctx context.Context, db store.DB, kind, id string) error {
	res, err := db.Query(ctx, `UPDATE entries SET status = 'archived', updated_at = ? WHERE id = ? AND kind = ?`, Clock().Unix(), id, kind)
	if err != nil {
		return err
	}
	if res.Changes == 0 {
		return NotFound(kind, id)
	}
	return nil
}

// DeleteEntry removes an entry; its revisions, tags and kind rows cascade.
func DeleteEntry(ctx context.Context, db store.DB, kind, id string) error {
	res, err := db.Query(ctx, `DELETE FROM entries WHERE id = ? AND kind = ?`, id, kind)
	if err != nil {
		return err
	}
	if res.Changes == 0 {
		return NotFound(kind, id)
	}
	return nil
}

// HTTPSURL reports whether s is an absolute https URL with a host
// (CONTENT_MODEL.md §6).
func HTTPSURL(s string) bool {
	u, err := url.Parse(s)
	return err == nil && u.Scheme == "https" && u.Host != "" && !strings.ContainsAny(s, " \t\n")
}

// Domain is the host of an https URL without "www.", for display.
func Domain(s string) string {
	u, err := url.Parse(s)
	if err != nil {
		return ""
	}
	return strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
}

// NullIfEmpty turns "" into a NULL argument.
func NullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
