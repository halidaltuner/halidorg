package page

import (
	"context"
	"net/http"
	"slices"
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

// Base, Text and TagList let the shared editor (shortadmin) edit a page.
func (d *Draft) Base() *content.Entry { return &d.Entry }
func (d *Draft) Text() *string        { return &d.Markdown }
func (d *Draft) TagList() *[]string   { return &d.Tags }

// row is one repeated form row as strings, so a rejected form shows what
// was typed.
type row struct {
	A, B, C, D, E, F, G string
	Tone                string
}

// options is what templates/page/admin_fields.html reads.
type options struct {
	Templates                         []shortadmin.Option
	Images, Docs                      []shortadmin.Option // "none" first
	Tones                             []string
	CTAs, Skills, Jobs                []row
	Building, Reading, Learning, Nots []row
}

// NewEditor is the page list and editor at /admin/pages (Phase 5.1,
// derive): About and Now, one per language. A new page picks its template
// (?template= presets it); the template's sections are rows like project
// facts: fill an empty row to add, empty one to remove, order to sort.
func NewEditor(db store.DB, u *admin.UI, a *auth.Auth, j *jobs.Jobs) (*shortadmin.Admin, error) {
	r := NewRepo(db)
	return shortadmin.New(shortadmin.Kind{
		Name: Kind, Section: "pages", Fields: "templates/page/admin_fields.html", Revised: true,
		HasBody: func(d shortadmin.Draft) bool { return d.(*Draft).Template == About },
		Public:  func(d shortadmin.Draft) string { e := d.Base(); return "/" + e.Lang + "/" + d.(*Draft).Template },
		New:     func() shortadmin.Draft { return &Draft{Page: Page{Entry: content.Entry{Kind: Kind}}} },
		Prefill: func(req *http.Request, d shortadmin.Draft) {
			p := d.(*Draft)
			if t := req.URL.Query().Get("template"); slices.Contains(Templates, t) {
				p.Template = t
				p.Title = i18n.T(p.Lang, "page.title."+t)
			}
		},
		Load: func(ctx context.Context, id string) (shortadmin.Draft, error) {
			p, err := r.Get(ctx, id)
			return &Draft{Page: p}, err
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
		Options: func(ctx context.Context, lang string, d shortadmin.Draft) (any, error) {
			return formOptions(ctx, db, lang, d.(*Draft))
		},
	}, db, u, a, j)
}

func num(s string) int {
	n, err := strconv.Atoi(strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(s), "%")))
	if err != nil {
		return -1 // Validate reports it
	}
	return n
}

func numOr0(s string) int {
	if strings.TrimSpace(s) == "" {
		return 0
	}
	return num(s)
}

func split(s string) []string { return strings.Split(s, ",") }

func readForm(req *http.Request, d shortadmin.Draft) {
	p := d.(*Draft)
	if p.ID == "" { // the template is chosen once
		p.Template = req.FormValue("template")
	}
	x := Data{Name: req.FormValue("name"), Portrait: req.FormValue("portrait"), CV: req.FormValue("cv"), Place: req.FormValue("place")}
	for _, r := range shortadmin.Rows(req, "cta_order", "cta_label", "cta_url") {
		x.CTAs = append(x.CTAs, Link{Label: r[0], URL: r[1]})
	}
	for _, r := range shortadmin.Rows(req, "skill_order", "skill_group", "skill_items", "skill_tone") {
		x.Skills = append(x.Skills, SkillGroup{Group: r[0], Items: split(r[1]), Tone: r[2]})
	}
	for _, r := range shortadmin.Rows(req, "job_order", "job_from", "job_to", "job_role", "job_org", "job_note", "job_tone") {
		x.Experience = append(x.Experience, Job{From: r[0], To: r[1], Role: r[2], Org: r[3], Note: r[4], Tone: r[5]})
	}
	for _, r := range shortadmin.Rows(req, "build_order", "build_title", "build_note", "build_stage", "build_pct", "build_href", "build_cover", "build_tone") {
		x.Building = append(x.Building, Build{Title: r[0], Note: r[1], Stage: r[2], Pct: numOr0(r[3]), Href: r[4], Cover: r[5], Tone: r[6]})
	}
	for _, r := range shortadmin.Rows(req, "book_order", "book_title", "book_author", "book_pct", "book_cover", "book_tone") {
		x.Reading = append(x.Reading, Book{Title: r[0], Author: r[1], Pct: numOr0(r[2]), Cover: r[3], Tone: r[4]})
	}
	for _, r := range shortadmin.Rows(req, "learn_order", "learn_title", "learn_note", "learn_level", "learn_hours", "learn_tone") {
		x.Learning = append(x.Learning, Learning{Title: r[0], Note: r[1], Level: num(r[2]), Hours: numOr0(r[3]), Tone: r[4]})
	}
	for _, r := range shortadmin.Rows(req, "not_order", "not_text") {
		x.NotDoing = append(x.NotDoing, r[0])
	}
	// Rows left empty read as level -1: keep them out of the check.
	learning := x.Learning[:0]
	for _, l := range x.Learning {
		if strings.TrimSpace(l.Title) != "" || strings.TrimSpace(l.Note) != "" {
			learning = append(learning, l)
		}
	}
	x.Learning = learning
	p.Data = x
}

func pct(n int) string {
	if n == 0 {
		return ""
	}
	return strconv.Itoa(n)
}

func formOptions(ctx context.Context, db store.DB, lang string, p *Draft) (options, error) {
	o := options{Tones: content.Tones}
	for _, t := range Templates {
		o.Templates = append(o.Templates, shortadmin.Option{Value: t, Label: i18n.T(lang, "nav."+t), On: p.Template == t})
	}
	res, err := db.Query(ctx, `SELECT id, name, mime, trim(alt_tr) <> '' AND trim(alt_en) <> '' AS ready FROM media
ORDER BY created_at DESC LIMIT 300`)
	if err != nil {
		return o, err
	}
	media, err := store.Scan[struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		Mime  string `json:"mime"`
		Ready int    `json:"ready"`
	}](res)
	if err != nil {
		return o, err
	}
	none := i18n.T(lang, "kind.project.no_image")
	o.Images = []shortadmin.Option{{Value: "", Label: none}}
	o.Docs = []shortadmin.Option{{Value: "", Label: none, On: p.Data.CV == ""}}
	for _, m := range media {
		name := m.Name
		if name == "" {
			name = m.ID
		}
		switch {
		case m.Mime == "application/pdf":
			o.Docs = append(o.Docs, shortadmin.Option{Value: m.ID, Label: name, On: p.Data.CV == m.ID})
		case strings.HasPrefix(m.Mime, "image/") && m.Ready == 1:
			o.Images = append(o.Images, shortadmin.Option{Value: m.ID, Label: name})
		}
	}
	d := p.Data
	for _, l := range d.CTAs {
		o.CTAs = append(o.CTAs, row{A: l.Label, B: l.URL})
	}
	for _, s := range d.Skills {
		o.Skills = append(o.Skills, row{A: s.Group, B: strings.Join(s.Items, ", "), Tone: s.Tone})
	}
	for _, j := range d.Experience {
		o.Jobs = append(o.Jobs, row{A: j.From, B: j.To, C: j.Role, D: j.Org, E: j.Note, Tone: j.Tone})
	}
	for _, b := range d.Building {
		o.Building = append(o.Building, row{A: b.Title, B: b.Note, C: b.Stage, D: pct(b.Pct), E: b.Href, F: b.Cover, Tone: b.Tone})
	}
	for _, b := range d.Reading {
		o.Reading = append(o.Reading, row{A: b.Title, B: b.Author, C: pct(b.Pct), D: b.Cover, Tone: b.Tone})
	}
	for _, l := range d.Learning {
		o.Learning = append(o.Learning, row{A: l.Title, B: l.Note, C: strconv.Itoa(l.Level), D: pct(l.Hours), Tone: l.Tone})
	}
	for _, s := range d.NotDoing {
		o.Nots = append(o.Nots, row{A: s})
	}
	o.CTAs = append(o.CTAs, row{}, row{})
	o.Skills = append(o.Skills, row{}, row{})
	o.Jobs = append(o.Jobs, row{}, row{})
	o.Building = append(o.Building, row{})
	o.Reading = append(o.Reading, row{}, row{})
	o.Learning = append(o.Learning, row{})
	o.Nots = append(o.Nots, row{})
	return o, nil
}
