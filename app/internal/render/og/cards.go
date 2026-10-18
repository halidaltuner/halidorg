package og

import (
	"fmt"
	"strings"
	"time"

	"halid.org/engine/internal/i18n"
)

// Where cards live. Entry cards sit at a stable R2 key and are addressed
// with ?v={hash}, so a re-render replaces the object and the URL in the
// page changes with it. Site cards carry the hash in the key: they are
// written once per design Version and language, never rewritten.

// EntryKey is the R2 key of an entry's card; section is the kind's URL
// segment ("posts").
func EntryKey(lang, section, slug string) string {
	return "og/" + lang + "/" + section + "/" + slug + ".png"
}

// SiteKey is the R2 key of the site card for lang.
func SiteKey(lang string) string {
	return "og/" + lang + "/site-" + Site(lang).Hash() + ".png"
}

// URL is the public path of an R2 key with the card's hash, which the edge
// Worker serves from R2 (worker/src/og.ts).
func URL(key string, c Card) string {
	return "/" + key + "?v=" + c.Hash()
}

// Date formats unix seconds as the card's date.
func Date(sec int64) string {
	return time.Unix(sec, 0).UTC().Format("2006-01-02")
}

// Post is the card of a published post. The handler that writes og:image
// and the job that renders the PNG both build it here, so they agree on the
// hash without reading R2.
func Post(lang, title string, publishedAt int64, minutes int) Card {
	meta := "halid.org"
	if minutes > 0 {
		meta += " · " + fmt.Sprintf(i18n.T(lang, "post.min_short"), minutes)
	}
	return Card{Kind: "post", Title: title, Date: Date(publishedAt), Meta: meta}
}

// Entry is the card of a published entry of a kind with item pages (note,
// snippet). Untitled notes use their summary, else their slug.
func Entry(kind, lang, title, summary, slug string, publishedAt int64) Card {
	if title == "" {
		title = summary
	}
	if title == "" {
		title = slug
	}
	return Card{Kind: kind, Title: title, Date: Date(publishedAt), Meta: "halid.org"}
}

// LogSection is the key section of build log cards: a log's slug is unique
// per language, so its card stays put when its project's slug changes.
const LogSection = "logs"

// BuildLog is the card of a published build log.
func BuildLog(lang, title, slug string, publishedAt int64) Card {
	return Entry("build log", lang, title, "", slug, publishedAt)
}

// Site is the card for pages without their own: home, lists, tags.
func Site(lang string) Card {
	parts := make([]string, 0, 4)
	for _, k := range []string{"home.title1", "home.title2", "home.title3", "home.title4"} {
		parts = append(parts, i18n.T(lang, k))
	}
	return Card{Title: strings.Join(parts, " "), Meta: "halid.org"}
}
