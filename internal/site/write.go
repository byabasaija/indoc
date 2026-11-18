package site

import (
	"bytes"
	"fmt"
	"html/template"
	"regexp"
	"strings"

	"github.com/byabasaija/indoc/internal/doc"
	"github.com/byabasaija/indoc/internal/pages"
	"github.com/byabasaija/indoc/internal/render"
)

type navItem struct {
	Title  string
	Href   string
	Active bool
}

type navGroup struct {
	Title string
	Items []navItem
}

type action struct {
	Text, Href, Theme string
}

type feature struct {
	Title, Details, Href, LinkText string
}

type view struct {
	Site      *Site
	Page      *built
	Root      string
	DocTitle  string
	Logo      string
	Home      string
	Nav       []navGroup
	Links     []Link
	TOC       []doc.Heading
	Prev      *navItem
	Next      *navItem
	Footer    template.HTML
	Landing   bool
	Hero      *pages.Hero
	HeroImage string
	Actions   []action
	Features  []feature
	Featured  []navItem
	Sections  []section
	Mermaid   bool
	Brand     template.CSS
	Head      template.HTML
	CustomCSS bool
	Canonical string
	Updated   string
	NotFound  bool
}

var funcs = template.FuncMap{
	"lower": strings.ToLower,
	"methods": func(detail string) []string {
		if detail == "" {
			return []string{"ANY"}
		}
		return strings.Split(detail, ", ")
	},
	"inline": func(s string) template.HTML { return template.HTML(render.Inline(s)) },
	"add":    func(a, b int) int { return a + b },
	// words splits a detail into pieces that must not break inside, keeping
	// a parenthesized note with the word before it: "--config (global)".
	"words": func(detail string) []string {
		var out []string
		for _, f := range strings.Fields(detail) {
			if strings.HasPrefix(f, "(") && len(out) > 0 {
				out[len(out)-1] += " " + f
				continue
			}
			out = append(out, f)
		}
		return out
	},
}

var tmpl = template.Must(template.New("").Funcs(funcs).ParseFS(files, "templates/*.html"))

var hexColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

func (b *builder) writePages() error {
	brand := ""
	if c := b.site.Colors; c != nil {
		if hexColor.MatchString(c.Primary) {
			brand += "--brand:" + c.Primary + ";"
		}
		if hexColor.MatchString(c.Accent) {
			brand += "--accent:" + c.Accent + ";"
		}
	}
	for i, p := range b.pages {
		v := b.view(p, brand)
		if i > 0 {
			prev := b.pages[i-1]
			v.Prev = &navItem{Title: prev.Title, Href: relURL(p.Slug, prev.Slug)}
		}
		if i+1 < len(b.pages) {
			next := b.pages[i+1]
			v.Next = &navItem{Title: next.Title, Href: relURL(p.Slug, next.Slug)}
		}
		if err := b.execute(p.Slug+"/index.html", v); err != nil {
			return err
		}
	}
	notFound := &built{Page: &Page{Title: "Page not found"}, HTML: template.HTML(
		`<h1>Page not found</h1><p>This page does not exist. Try the search above, or go to the <a href="./">home page</a>.</p>`)}
	v := b.view(notFound, brand)
	v.NotFound = true
	if err := b.execute("404.html", v); err != nil {
		return err
	}
	return b.sitemap()
}

func (b *builder) execute(name string, v *view) error {
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "page.html", v); err != nil {
		return fmt.Errorf("render %s: %w", name, err)
	}
	return b.w.write(strings.TrimPrefix(name, "/"), buf.Bytes())
}

func (b *builder) view(p *built, brand string) *view {
	root := rootPrefix(p.Slug)
	v := &view{
		Site: b.site, Page: p, Root: root, Links: b.site.Links,
		Home: relURL(p.Slug, ""), Brand: template.CSS(brand),
		Head: template.HTML(b.site.HeadHTML), CustomCSS: len(b.site.CustomCSS) > 0,
		Mermaid: p.mermaid,
	}
	v.DocTitle = b.site.Name
	if p.Slug != "" {
		v.DocTitle = p.Title + " · " + b.site.Name
	}
	if b.logo != "" {
		v.Logo = b.logo
		if !strings.Contains(b.logo, "://") {
			v.Logo = root + b.logo
		}
	}
	if b.site.Footer != "" {
		v.Footer = template.HTML(render.Inline(b.site.Footer))
	}
	if b.site.SiteURL != "" && !v.NotFound {
		v.Canonical = strings.TrimSuffix(b.site.SiteURL, "/") + "/" + urlOf(p.Slug)
	}
	if p.Updated != "" {
		v.Updated = p.Updated
	}
	for _, h := range p.Headings {
		if h.Level == 2 || h.Level == 3 {
			v.TOC = append(v.TOC, h)
		}
	}
	v.Nav = b.nav(p)
	if p.Layout == "landing" {
		b.landing(p, v)
	}
	return v
}

// nav builds the sidebar: top-level pages, then one group per category.
func (b *builder) nav(current *built) []navGroup {
	var groups []navGroup
	for _, p := range b.pages {
		if p.Slug == "" {
			continue // the home page is the brand link
		}
		title := ""
		if p.Category != "" {
			title = b.categoryTitle(p.Category)
		}
		if len(groups) == 0 || groups[len(groups)-1].Title != title {
			groups = append(groups, navGroup{Title: title})
		}
		g := &groups[len(groups)-1]
		g.Items = append(g.Items, navItem{Title: p.Title, Href: relURL(current.Slug, p.Slug), Active: p == current})
	}
	return groups
}

func (b *builder) landing(p *built, v *view) {
	v.Landing = true
	v.Sections = p.sections
	v.Hero = p.Hero
	if v.Hero == nil {
		v.Hero = &pages.Hero{}
	}
	link := func(dest string) string {
		if r := b.resolveLink(p, dest); r != "" {
			return r
		}
		return dest
	}
	if v.Hero.Image != "" {
		if img := b.resolveImage(p, v.Hero.Image); img != "" {
			v.HeroImage = img
		} else {
			v.HeroImage = v.Hero.Image
		}
	} else {
		v.HeroImage = v.Logo
	}
	for _, a := range v.Hero.Actions {
		theme := a.Theme
		if theme == "" {
			theme = "primary"
			if len(v.Actions) > 0 {
				theme = "secondary"
			}
		}
		v.Actions = append(v.Actions, action{Text: a.Text, Href: link(a.Link), Theme: theme})
	}
	for _, f := range p.Features {
		text := f.LinkText
		if text == "" && f.Link != "" {
			text = "Learn more"
		}
		v.Features = append(v.Features, feature{Title: f.Title, Details: f.Details, Href: link(f.Link), LinkText: text})
	}
	for _, f := range p.Featured {
		v.Featured = append(v.Featured, navItem{Title: f.Text, Href: link(f.Link)})
	}
}

func (b *builder) sitemap() error {
	if b.site.SiteURL == "" {
		return nil
	}
	base := strings.TrimSuffix(b.site.SiteURL, "/") + "/"
	var buf bytes.Buffer
	buf.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n" + `<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">` + "\n")
	for _, p := range b.pages {
		fmt.Fprintf(&buf, "  <url><loc>%s%s</loc>", template.HTMLEscapeString(base), template.HTMLEscapeString(urlOf(p.Slug)))
		if p.Updated != "" {
			fmt.Fprintf(&buf, "<lastmod>%s</lastmod>", p.Updated)
		}
		buf.WriteString("</url>\n")
	}
	buf.WriteString("</urlset>\n")
	return b.w.write("sitemap.xml", buf.Bytes())
}
