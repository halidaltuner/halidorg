package content

import (
	"strings"

	"halid.org/engine/internal/i18n"
)

// LangOrFallback is the language condition of a public list (Phase 7.3):
// entries in the page's language, and entries in another language with no
// published, public translation in it, so nothing written in one language
// only is missing from the other's lists. The alias is the entries table's;
// the condition takes the page's language twice.
func LangOrFallback(alias string) string {
	return `(` + alias + `.lang = ? OR NOT EXISTS (SELECT 1 FROM entries tr WHERE tr.translation_group_id = ` + alias + `.translation_group_id
  AND tr.id <> ` + alias + `.id AND tr.lang = ? AND tr.status = 'published' AND tr.visibility = 'public'))`
}

// TranslationMarks is the admin lists' translation column for an entry in
// lang: each other language with ✓ (published), … (a draft or archived
// copy) or ✕ (none). have maps a sibling's language to its status.
func TranslationMarks(lang string, have map[string]string) string {
	var marks []string
	for _, l := range i18n.Supported {
		if l == lang {
			continue
		}
		switch have[l] {
		case Published:
			marks = append(marks, l+" ✓")
		case "":
			marks = append(marks, l+" ✕")
		default:
			marks = append(marks, l+" …")
		}
	}
	return strings.Join(marks, " · ")
}
