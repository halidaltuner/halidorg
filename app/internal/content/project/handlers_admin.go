package project

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"halid.org/engine/internal/admin"
	"halid.org/engine/internal/auth"
	"halid.org/engine/internal/content"
	"halid.org/engine/internal/content/shortadmin"
	"halid.org/engine/internal/i18n"
	"halid.org/engine/internal/jobs"
	"halid.org/engine/internal/store"
)

// Base, Text and TagList let the shared editor (shortadmin) edit a project.
func (d *Draft) Base() *content.Entry { return &d.Entry }
func (d *Draft) Text() *string        { return &d.Markdown }
func (d *Draft) TagList() *[]string   { return &d.Tags }

type partRow struct{ Name, Qty, Price, Currency, URL string }

type logRow struct{ Href, Title, Date, Status, StatusKey string }

// options is what templates/project/admin_fields.html reads: the choices,
// and every row group with empty rows to add to.
type options struct {
	Types, Statuses, Categories, Covers, Media []shortadmin.Option
	Tones                                      []string
	Tone                                       *string
	Facts                                      []Fact
	Stack                                      []string
	Parts                                      []partRow
	Links                                      []Link
	Gallery                                    []string
	Started, Ended, Cost                       string
	Logs                                       []logRow
	NewLog                                     string // only once the project is saved
}

// NewEditor is the project list and editor at /admin/projects (Phase 4.1,
// derive). Facts, stack, parts, links and gallery are rows like snippet
// files: fill an empty row to add, empty a row to remove, order to sort.
// Deleting a project takes its build logs with it.
func NewEditor(db store.DB, u *admin.UI, a *auth.Auth, j *jobs.Jobs) (*shortadmin.Admin, error) {
	r := NewRepo(db)
	return shortadmin.New(shortadmin.Kind{
		Name: Kind, Section: "projects", Body: true, Fields: "templates/project/admin_fields.html",
		Extra:  []string{"templates/taxonomy/tone.html"},
		Public: func(d shortadmin.Draft) string { e := d.Base(); return "/" + e.Lang + "/projects/" + e.Slug },
		New: func() shortadmin.Draft {
			return &Draft{Project: Project{Entry: content.Entry{Kind: Kind}, Type: Software, ProjectStatus: "active", Currency: "TRY"}}
		},
		Load: func(ctx context.Context, id string) (shortadmin.Draft, error) {
			p, err := r.Get(ctx, id)
			return &Draft{Project: p}, err
		},
		Read: readForm,
		Create: func(ctx context.Context, d shortadmin.Draft) (string, error) {
			p, err := r.Create(ctx, *d.(*Draft))
			return p.ID, err
		},
		Save: func(ctx context.Context, d shortadmin.Draft, origin string) error {
			_, err := r.Save(ctx, *d.(*Draft), origin)
			return err
		},
		Publish: func(ctx context.Context, id string) error {
			_, err := r.Publish(ctx, id)
			return err
		},
		DeleteStmts: func(e content.Entry) []store.Stmt { return DeleteStmts(e.ID) },
		Options: func(ctx context.Context, lang string, d shortadmin.Draft) (any, error) {
			return formOptions(ctx, db, lang, d.(*Draft))
		},
	}, db, u, a, j)
}

func readForm(req *http.Request, d shortadmin.Draft) {
	p := d.(*Draft)
	p.Type = req.FormValue("project_type")
	p.ProjectStatus = req.FormValue("project_status")
	p.CategoryID = shortadmin.Ptr(req, "category")
	p.Tagline = req.FormValue("tagline")
	p.KindLine = req.FormValue("kind_line")
	p.GitHubRepo = req.FormValue("github_repo")
	p.StartedAt = ParseMonth(req.FormValue("started"))
	p.EndedAt = ParseMonth(req.FormValue("ended"))
	p.Featured = req.FormValue("featured") == "1"
	p.Currency = req.FormValue("currency")
	p.TimeSpent = req.FormValue("time_spent")
	p.CostTotal = money(req.FormValue("cost_total"))
	p.Tone = shortadmin.Ptr(req, "tone")
	p.CoverMediaID = shortadmin.Ptr(req, "cover")
	p.Facts = nil
	for _, x := range shortadmin.Rows(req, "fact_order", "fact_key", "fact_value") {
		p.Facts = append(p.Facts, Fact{Key: x[0], Value: x[1]})
	}
	p.Stack = nil
	for _, x := range shortadmin.Rows(req, "stack_order", "stack_name") {
		p.Stack = append(p.Stack, x[0])
	}
	p.Parts = nil
	for _, x := range shortadmin.Rows(req, "part_order", "part_name", "part_qty", "part_price", "part_currency", "part_url") {
		qty, _ := strconv.Atoi(x[1])
		p.Parts = append(p.Parts, Part{Name: x[0], Qty: qty, Price: money(x[2]), Currency: x[3], URL: x[4]})
	}
	p.Links = nil
	for _, x := range shortadmin.Rows(req, "link_order", "link_label", "link_url") {
		p.Links = append(p.Links, Link{Label: x[0], URL: x[1]})
	}
	p.Gallery = nil
	for _, x := range shortadmin.Rows(req, "gallery_order", "gallery_media") {
		p.Gallery = append(p.Gallery, x[0])
	}
}

// money is a typed amount in minor units: nil when empty, -1 when it is not
// an amount, which Validate reports.
func money(s string) *int64 {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	v, ok := ParseMoney(s)
	if !ok {
		v = -1
	}
	return &v
}

func formOptions(ctx context.Context, db store.DB, lang string, p *Draft) (options, error) {
	o := options{Tones: content.Tones, Tone: p.Tone, Started: FormatMonth(p.StartedAt), Ended: FormatMonth(p.EndedAt), Cost: FormatMoney(p.CostTotal)}
	for _, t := range []string{Software, Homelab} {
		o.Types = append(o.Types, shortadmin.Option{Value: t, Label: i18n.T(lang, "kind.project.type."+t), On: p.Type == t})
	}
	for _, s := range Statuses {
		o.Statuses = append(o.Statuses, shortadmin.Option{Value: s, Label: i18n.T(lang, "kind.project.status."+s), On: p.ProjectStatus == s})
	}
	var err error
	if o.Categories, err = shortadmin.CategoryOptions(ctx, db, "homelab", lang, p.CategoryID); err != nil {
		return o, err
	}
	// Images with both alt texts: the cover and the gallery choose from them.
	res, err := db.Query(ctx, `SELECT id, name FROM media WHERE trim(alt_tr) <> '' AND trim(alt_en) <> '' ORDER BY created_at DESC LIMIT 200`)
	if err != nil {
		return o, err
	}
	media, err := store.Scan[struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}](res)
	if err != nil {
		return o, err
	}
	none := i18n.T(lang, "kind.project.no_image")
	o.Covers = []shortadmin.Option{{Value: "", Label: none, On: p.CoverMediaID == nil}}
	o.Media = []shortadmin.Option{{Value: "", Label: none}}
	for _, m := range media {
		name := m.Name
		if name == "" {
			name = m.ID
		}
		o.Covers = append(o.Covers, shortadmin.Option{Value: m.ID, Label: name, On: p.CoverMediaID != nil && *p.CoverMediaID == m.ID})
		o.Media = append(o.Media, shortadmin.Option{Value: m.ID, Label: name})
	}
	o.Facts = append(append([]Fact(nil), p.Facts...), Fact{}, Fact{})
	o.Stack = append(append([]string(nil), p.Stack...), "", "")
	for _, x := range p.Parts {
		o.Parts = append(o.Parts, partRow{Name: x.Name, Qty: strconv.Itoa(x.Qty), Price: FormatMoney(x.Price), Currency: x.Currency, URL: x.URL})
	}
	o.Parts = append(o.Parts, partRow{}, partRow{})
	o.Links = append(append([]Link(nil), p.Links...), Link{})
	o.Gallery = append(append([]string(nil), p.Gallery...), "")
	if p.ID == "" {
		return o, nil
	}
	o.NewLog = "/admin/buildlogs/new?project=" + p.ID
	// Its build logs, read here so project does not import buildlog.
	res, err = db.Query(ctx, `SELECT e.id, e.title, e.status, b.log_date FROM buildlogs b JOIN entries e ON e.id = b.entry_id
WHERE b.project_entry_id = ? ORDER BY b.log_date DESC, e.id DESC LIMIT 100`, p.ID)
	if err != nil {
		return o, err
	}
	logs, err := store.Scan[struct {
		ID     string `json:"id"`
		Title  string `json:"title"`
		Status string `json:"status"`
		Date   int64  `json:"log_date"`
	}](res)
	if err != nil {
		return o, err
	}
	for _, l := range logs {
		o.Logs = append(o.Logs, logRow{Href: admin.EditorHref("buildlog", l.ID), Title: l.Title, Date: FormatDay(l.Date),
			Status: i18n.T(lang, "admin.status."+l.Status), StatusKey: l.Status})
	}
	return o, nil
}
