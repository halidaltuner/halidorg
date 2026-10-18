package github

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"halid.org/engine/internal/i18n"
	"halid.org/engine/internal/store"
)

// Section is the GitHub part of the projects page (design/Projects.dc.html):
// the 52-week strip, the language bar, pinned repos and recent releases.
type Section struct {
	Total    int
	Weeks    []Bar
	Labels   [3]string // first, middle and last month under the strip
	Langs    []Lang
	Pinned   []PinnedRepo
	Releases []ReleaseRow
}

// Bar is one week: its height in percent and its intensity bucket (1-3).
type Bar struct {
	Height, Level, Count int
	Title                string
}

// Lang is one language's share, with its color slot (1-4, 0 for other).
type Lang struct {
	Name    string
	Percent int
	Slot    int
}

// PinnedRepo is one card of the pinned grid.
type PinnedRepo struct {
	Name        string `json:"name"`
	URL         string `json:"url"`
	Description string `json:"description"`
	Language    string `json:"language"`
	Stars       int    `json:"stars"`
}

// ReleaseRow is one line of the releases list.
type ReleaseRow struct {
	Title, Date, Note, URL string
}

// Load reads the section, or nil before the first sync.
func Load(ctx context.Context, db store.DB, lang string) (*Section, error) {
	res, err := db.Batch(ctx,
		store.Stmt{SQL: `SELECT week_start, contributions FROM gh_weekly ORDER BY week_start DESC LIMIT 52`},
		store.Stmt{SQL: `SELECT l.language, sum(l.bytes) AS bytes FROM gh_languages l JOIN gh_repos r ON r.id = l.repo_id
WHERE r.fork = 0 AND r.gone = 0 GROUP BY l.language ORDER BY bytes DESC`},
		store.Stmt{SQL: `SELECT name, url, description, language, stars FROM gh_repos WHERE pinned = 1 AND gone = 0 ORDER BY pin_sort, name LIMIT 6`},
		store.Stmt{SQL: `SELECT a.title, a.note, a.url, a.occurred_at FROM gh_activity a JOIN gh_repos r ON r.full_name = a.repo
WHERE a.type = 'release' AND r.gone = 0 ORDER BY a.occurred_at DESC LIMIT 4`},
	)
	if err != nil {
		return nil, err
	}
	weeks, err := store.Scan[struct {
		Start int64 `json:"week_start"`
		N     int   `json:"contributions"`
	}](res[0])
	if err != nil {
		return nil, err
	}
	langs, err := store.Scan[struct {
		Language string `json:"language"`
		Bytes    int64  `json:"bytes"`
	}](res[1])
	if err != nil {
		return nil, err
	}
	pinned, err := store.Scan[PinnedRepo](res[2])
	if err != nil {
		return nil, err
	}
	rels, err := store.Scan[struct {
		Title string `json:"title"`
		Note  string `json:"note"`
		URL   string `json:"url"`
		At    int64  `json:"occurred_at"`
	}](res[3])
	if err != nil {
		return nil, err
	}
	if len(weeks) == 0 && len(langs) == 0 && len(pinned) == 0 && len(rels) == 0 {
		return nil, nil
	}
	s := &Section{Pinned: pinned}
	// Oldest first, left to right.
	sort.Slice(weeks, func(i, j int) bool { return weeks[i].Start < weeks[j].Start })
	top := 0
	for _, w := range weeks {
		s.Total += w.N
		top = max(top, w.N)
	}
	for _, w := range weeks {
		bar := Bar{Count: w.N, Height: 8, Level: 1,
			Title: fmt.Sprintf(i18n.T(lang, "github.week_title"), w.N, time.Unix(w.Start, 0).UTC().Format("2006-01-02"))}
		if top > 0 {
			bar.Height = max(8, w.N*100/top)
			switch {
			case w.N*3 > top*2:
				bar.Level = 3
			case w.N*3 > top:
				bar.Level = 2
			}
		}
		s.Weeks = append(s.Weeks, bar)
	}
	if n := len(weeks); n > 0 {
		month := func(i int) string { return time.Unix(weeks[i].Start, 0).UTC().Format("2006-01") }
		s.Labels = [3]string{month(0), month(n / 2), month(n - 1)}
	}
	var sum int64
	for _, l := range langs {
		sum += l.Bytes
	}
	if sum > 0 {
		var other int64
		for i, l := range langs {
			if i < 4 {
				s.Langs = append(s.Langs, Lang{Name: l.Language, Percent: int(l.Bytes * 100 / sum), Slot: i + 1})
			} else {
				other += l.Bytes
			}
		}
		if other > 0 {
			s.Langs = append(s.Langs, Lang{Name: i18n.T(lang, "github.other"), Percent: int(other * 100 / sum)})
		}
	}
	for _, r := range rels {
		s.Releases = append(s.Releases, ReleaseRow{Title: r.Title, Note: r.Note, URL: r.URL, Date: time.Unix(r.At, 0).UTC().Format("2006-01-02")})
	}
	return s, nil
}

// RepoStat is what a project card shows of its repo.
type RepoStat struct {
	Stars    int
	Activity string // "pushed 2h ago"
}

// Stats reads the repos linked to projects, by full name (owner/name, any
// case), with the push time relative to now.
func Stats(ctx context.Context, db store.DB, lang string, names []string, now time.Time) (map[string]RepoStat, error) {
	out := map[string]RepoStat{}
	if len(names) == 0 {
		return out, nil
	}
	args := make([]any, len(names))
	for i, n := range names {
		args[i] = strings.ToLower(n)
	}
	res, err := db.Query(ctx, `SELECT full_name, stars, pushed_at FROM gh_repos WHERE gone = 0 AND lower(full_name) IN (?`+
		strings.Repeat(", ?", len(names)-1)+`)`, args...)
	if err != nil {
		return nil, err
	}
	rows, err := store.Scan[struct {
		Name   string `json:"full_name"`
		Stars  int    `json:"stars"`
		Pushed *int64 `json:"pushed_at"`
	}](res)
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		st := RepoStat{Stars: r.Stars}
		if r.Pushed != nil {
			st.Activity = fmt.Sprintf(i18n.T(lang, "github.pushed"), Ago(lang, now.Sub(time.Unix(*r.Pushed, 0))))
		}
		out[strings.ToLower(r.Name)] = st
	}
	return out, nil
}

// Ago is a short duration: "2h", "6d", "3mo", "2y".
func Ago(lang string, d time.Duration) string {
	h := int(d.Hours())
	switch {
	case h < 1:
		return i18n.T(lang, "github.now")
	case h < 24:
		return fmt.Sprintf(i18n.T(lang, "github.h"), h)
	case h < 24*30:
		return fmt.Sprintf(i18n.T(lang, "github.d"), h/24)
	case h < 24*365:
		return fmt.Sprintf(i18n.T(lang, "github.mo"), h/(24*30))
	}
	return fmt.Sprintf(i18n.T(lang, "github.y"), h/(24*365))
}

// Month is one month of the activity list on /github.
type Month struct {
	Month string // 2026-10
	Items []ActivityRow
}

// ActivityRow is a release or a new repo.
type ActivityRow struct {
	Type, Title, Note, URL, Date string
}

// Activity reads the newest limit activity rows of repos still on GitHub,
// grouped by month, newest first.
func Activity(ctx context.Context, db store.DB, limit int) ([]Month, error) {
	res, err := db.Query(ctx, `SELECT a.type, a.title, a.note, a.url, a.occurred_at FROM gh_activity a
JOIN gh_repos r ON r.full_name = a.repo WHERE r.gone = 0 ORDER BY a.occurred_at DESC, a.id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	rows, err := store.Scan[struct {
		Type  string `json:"type"`
		Title string `json:"title"`
		Note  string `json:"note"`
		URL   string `json:"url"`
		At    int64  `json:"occurred_at"`
	}](res)
	if err != nil {
		return nil, err
	}
	var out []Month
	for _, r := range rows {
		t := time.Unix(r.At, 0).UTC()
		row := ActivityRow{Type: r.Type, Title: r.Title, Note: r.Note, URL: r.URL, Date: t.Format("2006-01-02")}
		if n := len(out); n > 0 && out[n-1].Month == t.Format("2006-01") {
			out[n-1].Items = append(out[n-1].Items, row)
			continue
		}
		out = append(out, Month{Month: t.Format("2006-01"), Items: []ActivityRow{row}})
	}
	return out, nil
}
