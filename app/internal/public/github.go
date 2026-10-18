package public

import (
	"net/http"

	"halid.org/engine/internal/github"
	"halid.org/engine/internal/i18n"
)

// /{lang}/github (derive): the projects page's GitHub section on its own,
// with every activity row (releases, new repos) by month. github.sync
// refreshes it.

type githubPage struct {
	page
	GitHub *github.Section
	Months []github.Month
}

// githubActivity bounds the activity list.
const githubActivity = 100

func (h *Handlers) github(w http.ResponseWriter, r *http.Request, lang string) {
	p := githubPage{page: h.newPage(lang, "projects", i18n.T(lang, "github.title")+" · halid.org", "/github", nil)}
	p.Head.Description = i18n.T(lang, "github.sub")
	var err error
	if p.GitHub, err = github.Load(r.Context(), h.db, lang); err != nil {
		h.fail(w, r, err)
		return
	}
	if p.Months, err = github.Activity(r.Context(), h.db, githubActivity); err != nil {
		h.fail(w, r, err)
		return
	}
	h.render(w, r, "github", p)
}
