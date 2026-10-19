package jobs

import (
	"context"
	"encoding/json"
	"fmt"

	"halid.org/engine/internal/content"
	"halid.org/engine/internal/i18n"
	"halid.org/engine/internal/search"
	"halid.org/engine/internal/store"
)

// TypeSearchRebuild recreates the SEARCH database from the main one (the
// admin's "rebuild search index"), a page of entries per job: the first
// page empties the index, the last indexes photos and certificates.
const TypeSearchRebuild = "search.rebuild"

// TypeSearchGallery re-indexes photos and certificates, which are not
// entries: their writes enqueue it.
const TypeSearchGallery = "search.gallery"

// SearchRebuild is the rebuild payload: entries after After (by id).
type SearchRebuild struct {
	After string `json:"after"`
}

// rebuildPage is how many entries one rebuild job indexes.
const rebuildPage = 100

// EnqueueSearchRebuild starts a rebuild.
func EnqueueSearchRebuild(ctx context.Context, j *Jobs) error {
	_, err := j.Enqueue(ctx, TypeSearchRebuild, SearchRebuild{})
	return err
}

// EnqueueSearchGallery re-indexes photos and certificates.
func EnqueueSearchGallery(ctx context.Context, j *Jobs) error {
	_, err := j.Enqueue(ctx, TypeSearchGallery, struct{}{})
	return err
}

func (e *EntryJobs) rebuild(ctx context.Context, raw json.RawMessage) error {
	var p SearchRebuild
	if err := json.Unmarshal(raw, &p); err != nil {
		return Permanent{fmt.Errorf("search.rebuild: bad payload: %s", raw)}
	}
	if p.After == "" {
		if _, err := e.search.Query(ctx, `DELETE FROM search_fts`); err != nil {
			return err
		}
	}
	res, err := e.db.Query(ctx, `SELECT id FROM entries WHERE id > ? ORDER BY id LIMIT ?`, p.After, rebuildPage)
	if err != nil {
		return err
	}
	ids, err := store.Scan[struct {
		ID string `json:"id"`
	}](res)
	if err != nil {
		return err
	}
	for _, x := range ids {
		row, err := load(ctx, e.db, x.ID)
		if err != nil {
			return err
		}
		if err := e.index(ctx, x.ID, row); err != nil {
			return err
		}
	}
	if len(ids) == rebuildPage {
		_, err := e.jobs.Enqueue(ctx, TypeSearchRebuild, SearchRebuild{After: ids[len(ids)-1].ID})
		return err
	}
	return e.gallery(ctx)
}

func (e *EntryJobs) galleryJob(ctx context.Context, _ json.RawMessage) error {
	return e.gallery(ctx)
}

// gallery replaces the photo and certificate rows: a photo by its title in
// each language (the other language's when one is empty) and its place, a
// certificate by its name and issuer, in both languages.
func (e *EntryJobs) gallery(ctx context.Context) error {
	stmts := []store.Stmt{{SQL: `DELETE FROM search_fts WHERE kind IN (?, ?)`, Args: []any{search.KindPhoto, search.KindCertificate}}}
	status := func(published int) string {
		if published == 1 {
			return content.Published
		}
		return content.Draft
	}
	res, err := e.db.Query(ctx, `SELECT id, title_tr, title_en, place, published FROM photos`)
	if err != nil {
		return err
	}
	photos, err := store.Scan[struct {
		ID        string `json:"id"`
		TitleTR   string `json:"title_tr"`
		TitleEN   string `json:"title_en"`
		Place     string `json:"place"`
		Published int    `json:"published"`
	}](res)
	if err != nil {
		return err
	}
	for _, p := range photos {
		for _, l := range i18n.Supported {
			title := p.TitleEN
			if l == "tr" && p.TitleTR != "" || title == "" {
				title = p.TitleTR
			}
			stmts = append(stmts, search.Doc{EntryID: p.ID, Kind: search.KindPhoto, Lang: l, Status: status(p.Published), Visibility: content.Public,
				Slug: p.ID, Title: title, Summary: p.Place}.Insert())
		}
	}
	res, err = e.db.Query(ctx, `SELECT c.id, c.name, i.name AS issuer, c.published FROM certificates c JOIN cert_issuers i ON i.id = c.issuer_id`)
	if err != nil {
		return err
	}
	certs, err := store.Scan[struct {
		ID        string `json:"id"`
		Name      string `json:"name"`
		Issuer    string `json:"issuer"`
		Published int    `json:"published"`
	}](res)
	if err != nil {
		return err
	}
	for _, c := range certs {
		for _, l := range i18n.Supported {
			stmts = append(stmts, search.Doc{EntryID: c.ID, Kind: search.KindCertificate, Lang: l, Status: status(c.Published), Visibility: content.Public,
				Slug: c.ID, Title: c.Name, Summary: c.Issuer}.Insert())
		}
	}
	for len(stmts) > 0 {
		n := min(len(stmts), 100)
		if _, err := e.search.Batch(ctx, stmts[:n]...); err != nil {
			return err
		}
		stmts = stmts[n:]
	}
	return nil
}
