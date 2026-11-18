// Package site builds the static documentation site: one pre-rendered HTML
// file per page, readable without JavaScript, with relative links so it works
// from any path, a static host, or file://. JavaScript only adds search, the
// theme toggle, copy buttons, syntax highlighting, diagrams, and the GitHub
// star count.
package site

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/byabasaija/indoc/internal/doc"
	"github.com/byabasaija/indoc/internal/pages"
	"github.com/byabasaija/indoc/internal/render"
)

//go:embed templates/*.html assets/indoc.css assets/indoc.js assets/vendor/prism.js assets/vendor/mermaid.min.js.gz
var files embed.FS

// Site is everything the site is built from.
type Site struct {
	Name        string
	Logo        string // http(s) URL, or a path relative to RepoRoot
	Colors      *doc.Colors
	Links       []Link
	Footer      string // Markdown
	SiteURL     string
	Repo        string // "owner/repo" on GitHub, for the star count
	AllowHTML   bool
	CustomCSS   []byte
	HeadHTML    string
	RepoRoot    string
	Pages       []Page
	Reference   []doc.RefSection
	ShowAuthors bool
}

// Link is a header link.
type Link struct{ Text, URL string }

// Page is one Markdown page with what the site needs about it.
type Page struct {
	Source      string // repository-relative path of the Markdown file
	Title       string
	Description string
	Order       int
	HasOrder    bool
	Markdown    string
	Layout      string
	Hero        *pages.Hero
	Features    []pages.Feature
	Featured    []pages.Action
	Sections    []pages.Section // landing blocks in order; nil means hero, features, body
	EditURL     string
	EditLabel   string
	Updated     string // YYYY-MM-DD from git
	Author      string
}

// built is a page ready for a template.
type built struct {
	*Page
	Slug      string // "" for home, "dev/checkout" otherwise
	Category  string
	HTML      template.HTML
	Headings  []doc.Heading
	Text      string
	Reference *doc.RefSection
	mermaid   bool
	sections  []section // landing blocks, rendered
}

// section is a landing block ready for the template.
type section struct {
	Type  string
	Title string
	HTML  template.HTML // markdown
	Repo  string        // contributors
	Limit int           // contributors
	Logos []logo        // logos
}

type logo struct{ Src, Name, Href string }

const manifestName = ".indoc-site"

// Build writes the site to outDir.
func Build(s *Site, outDir string) error {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	w := &writer{out: outDir, written: map[string]bool{}}
	b := &builder{site: s, w: w}
	if err := b.plan(); err != nil {
		return err
	}
	if err := b.render(); err != nil {
		return err
	}
	if err := b.assets(); err != nil {
		return err
	}
	if err := b.writePages(); err != nil {
		return err
	}
	return w.finish()
}

type builder struct {
	site     *Site
	w        *writer
	pages    []*built // navigation order, home first
	bySource map[string]*built
	images   map[string]string // repo path -> site path
	logo     string            // site-relative logo path or absolute URL
	mermaid  bool
}

// plan assigns URLs and navigation order.
func (b *builder) plan() error {
	if len(b.site.Pages) == 0 {
		return fmt.Errorf("no pages to build")
	}
	base := commonDir(b.site.Pages)
	b.bySource = map[string]*built{}
	var home *built
	for i := range b.site.Pages {
		p := &b.site.Pages[i]
		rel := strings.TrimPrefix(p.Source, base)
		rel = strings.TrimSuffix(rel, path.Ext(rel))
		name := path.Base(rel)
		if strings.EqualFold(name, "index") || strings.EqualFold(name, "readme") {
			rel = strings.TrimSuffix(path.Dir(rel), ".")
		}
		bp := &built{Page: p, Slug: slugPath(rel)}
		if i := strings.Index(bp.Slug, "/"); i > 0 {
			bp.Category = bp.Slug[:i]
		} else if bp.Slug != "" && b.isFolder(bp.Slug, base) {
			bp.Category = bp.Slug
		}
		if bp.Slug == "" && home == nil {
			home = bp
		}
		b.bySource[p.Source] = bp
		b.pages = append(b.pages, bp)
	}
	sort.SliceStable(b.pages, func(i, j int) bool { return b.less(b.pages[i], b.pages[j]) })
	if home == nil {
		home = b.pages[0] // no index.md: the first page is the home page
		home.Slug, home.Category = "", ""
	}
	ordered := []*built{home}
	for _, p := range b.pages {
		if p != home {
			ordered = append(ordered, p)
		}
	}
	for i := range b.site.Reference {
		sec := &b.site.Reference[i]
		ordered = append(ordered, &built{
			Page:      &Page{Title: sec.Title, Source: ""},
			Slug:      "reference/" + doc.Slug(sec.ID),
			Category:  "reference",
			Reference: sec,
		})
	}
	b.pages = ordered
	return b.checkCollisions()
}

// checkCollisions refuses two pages at one URL: the later would silently
// overwrite the earlier. It happens with guides.md next to guides/index.md,
// or a page at reference/routes.md next to the generated Routes page.
func (b *builder) checkCollisions() error {
	seen := map[string]*built{}
	for _, p := range b.pages {
		prev, ok := seen[p.Slug]
		if !ok {
			seen[p.Slug] = p
			continue
		}
		name := func(x *built) string {
			if x.Reference != nil {
				return fmt.Sprintf("the generated %q reference page", x.Title)
			}
			return x.Source
		}
		fix := "rename or move one of them"
		if p.Reference != nil || prev.Reference != nil {
			fix = "rename or move your page; generated reference pages live under reference/"
		}
		return fmt.Errorf("%s and %s would both be published at %s; %s", name(prev), name(p), "/"+urlOf(p.Slug), fix)
	}
	return nil
}

func (b *builder) isFolder(slug, base string) bool {
	for _, p := range b.site.Pages {
		if strings.HasPrefix(strings.TrimPrefix(p.Source, base), slug+"/") {
			return true
		}
	}
	return false
}

// less orders pages: top level first, then categories, each by order then title.
func (b *builder) less(x, y *built) bool {
	cx, cy := b.categoryRank(x.Category), b.categoryRank(y.Category)
	if x.Category != y.Category {
		if cx.order != cy.order {
			return cx.order < cy.order
		}
		return cx.title < cy.title
	}
	// The folder's own index comes first in its category.
	if (x.Slug == x.Category) != (y.Slug == y.Category) {
		return x.Slug == x.Category
	}
	return pageLess(x.Page, y.Page)
}

type rank struct {
	order int
	title string
}

func (b *builder) categoryRank(cat string) rank {
	if cat == "" {
		return rank{order: -1 << 30}
	}
	r := rank{order: 1 << 30, title: b.categoryTitle(cat)}
	for _, p := range b.site.Pages {
		bp := b.bySource[p.Source]
		if bp == nil || bp.Category != cat || !p.HasOrder {
			continue
		}
		if bp.Slug == cat {
			return rank{order: p.Order, title: r.title}
		}
		if p.Order < r.order {
			r.order = p.Order
		}
	}
	return r
}

func (b *builder) categoryTitle(cat string) string {
	if cat == "reference" {
		return "Reference"
	}
	for _, bp := range b.bySource {
		if bp.Slug == cat {
			return bp.Title
		}
	}
	return humanize(cat)
}

func pageLess(a, c *Page) bool {
	if a.HasOrder != c.HasOrder {
		return a.HasOrder
	}
	if a.Order != c.Order {
		return a.Order < c.Order
	}
	return a.Title < c.Title
}

func commonDir(ps []Page) string {
	dirs := make([]string, len(ps))
	for i, p := range ps {
		dirs[i] = path.Dir(p.Source)
	}
	common := dirs[0]
	for _, d := range dirs[1:] {
		for common != "." && d != common && !strings.HasPrefix(d, common+"/") {
			common = path.Dir(common)
		}
	}
	if common == "." {
		return ""
	}
	return common + "/"
}

func slugPath(rel string) string {
	if rel == "" {
		return ""
	}
	parts := strings.Split(rel, "/")
	for i, p := range parts {
		parts[i] = doc.Slug(p)
	}
	return strings.Join(parts, "/")
}

func humanize(s string) string {
	s = strings.NewReplacer("-", " ", "_", " ").Replace(s)
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// render converts every page's Markdown.
func (b *builder) render() error {
	b.images = map[string]string{}
	for _, p := range b.pages {
		if p.Reference != nil {
			p.Text = refText(p.Reference)
			if p.Reference.Intro != "" {
				res, err := render.Render(p.Reference.Intro, render.Options{AllowHTML: b.site.AllowHTML})
				if err != nil {
					return err
				}
				p.HTML = template.HTML(res.HTML)
			}
			continue
		}
		page := p
		res, err := render.Render(p.Markdown, render.Options{
			AllowHTML: b.site.AllowHTML,
			Link:      func(dest string) string { return b.resolveLink(page, dest) },
			Image:     func(src string) string { return b.resolveImage(page, src) },
		})
		if err != nil {
			return fmt.Errorf("%s: %w", p.Source, err)
		}
		p.HTML, p.Headings, p.Text, p.mermaid = template.HTML(res.HTML), res.Headings, res.Text, res.UsesMermaid
		b.mermaid = b.mermaid || res.UsesMermaid
		if p.Layout == "landing" {
			if err := b.renderSections(p); err != nil {
				return fmt.Errorf("%s: %w", p.Source, err)
			}
		}
	}
	return nil
}

// renderSections prepares a landing page's blocks in their listed order.
// Without a list the order is hero, features, body; the body is appended
// when the list leaves it out, so page content is never dropped.
func (b *builder) renderSections(p *built) error {
	list := p.Sections
	if list == nil {
		list = []pages.Section{{Type: "hero"}, {Type: "features"}, {Type: "body"}}
	}
	hasBody := false
	for _, s := range list {
		hasBody = hasBody || s.Type == "body"
	}
	if !hasBody {
		list = append(list, pages.Section{Type: "body"})
	}
	for i, s := range list {
		out := section{Type: s.Type, Title: s.Title}
		switch s.Type {
		case "markdown":
			res, err := render.Render(s.Body, render.Options{
				AllowHTML: b.site.AllowHTML,
				Link:      func(dest string) string { return b.resolveLink(p, dest) },
				Image:     func(src string) string { return b.resolveImage(p, src) },
			})
			if err != nil {
				return err
			}
			out.HTML = template.HTML(res.HTML)
			p.Text += " " + res.Text
			p.mermaid = p.mermaid || res.UsesMermaid
			b.mermaid = b.mermaid || res.UsesMermaid
		case "contributors":
			out.Repo = s.Repo
			if out.Repo == "" {
				out.Repo = b.site.Repo
			}
			if out.Repo == "" {
				return fmt.Errorf("sections[%d]: contributors needs repo: owner/repo (source_url is not a GitHub URL)", i)
			}
			out.Limit = s.Limit
			if out.Limit == 0 {
				out.Limit = 48
			}
			if out.Title == "" {
				out.Title = "Contributors"
			}
		case "logos":
			for j, it := range s.Items {
				src := it.Image
				if !strings.Contains(src, "://") {
					if src = b.resolveImage(p, it.Image); src == "" {
						if sitePath, err := b.copyFile(path.Clean(strings.TrimPrefix(it.Image, "/")), "assets/img"); err == nil {
							src = rootPrefix(p.Slug) + sitePath
						}
					}
					if src == "" {
						return fmt.Errorf("sections[%d].items[%d]: image %q not found", i, j, it.Image)
					}
				}
				href := it.Link
				if r := b.resolveLink(p, it.Link); r != "" {
					href = r
				}
				out.Logos = append(out.Logos, logo{Src: src, Name: it.Name, Href: href})
			}
		}
		p.sections = append(p.sections, out)
	}
	return nil
}

// resolveLink turns a link to another page's Markdown file into its URL.
func (b *builder) resolveLink(from *built, dest string) string {
	u, err := url.Parse(dest)
	if err != nil || u.Scheme != "" || u.Host != "" || u.Path == "" || strings.HasPrefix(dest, "/") {
		return ""
	}
	target := path.Clean(path.Join(path.Dir(from.Source), u.Path))
	to, ok := b.bySource[target]
	if !ok {
		return ""
	}
	href := relURL(from.Slug, to.Slug)
	if u.Fragment != "" {
		href += "#" + u.Fragment
	}
	return href
}

// resolveImage copies a local image into the site.
func (b *builder) resolveImage(from *built, src string) string {
	u, err := url.Parse(src)
	if err != nil || u.Scheme != "" || u.Host != "" || u.Path == "" {
		return ""
	}
	repoPath := path.Clean(path.Join(path.Dir(from.Source), u.Path))
	sitePath, err := b.copyFile(repoPath, "assets/img")
	if err != nil {
		return ""
	}
	return rootPrefix(from.Slug) + sitePath
}

// copyFile copies a repository file into the site under dir, named by
// content hash, and returns its site path.
func (b *builder) copyFile(repoPath, dir string) (string, error) {
	if p, ok := b.images[repoPath]; ok {
		return p, nil
	}
	data, err := os.ReadFile(filepath.Join(b.site.RepoRoot, filepath.FromSlash(repoPath)))
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	name := path.Join(dir, strings.TrimSuffix(path.Base(repoPath), path.Ext(repoPath))+"-"+hex.EncodeToString(sum[:4])+strings.ToLower(path.Ext(repoPath)))
	if err := b.w.write(name, data); err != nil {
		return "", err
	}
	b.images[repoPath] = name
	return name, nil
}

func (b *builder) assets() error {
	for _, name := range []string{"assets/indoc.css", "assets/indoc.js", "assets/vendor/prism.js"} {
		data, err := files.ReadFile(name)
		if err != nil {
			return err
		}
		if err := b.w.write(strings.Replace(name, "vendor/", "", 1), data); err != nil {
			return err
		}
	}
	if b.mermaid {
		gz, err := files.Open("assets/vendor/mermaid.min.js.gz")
		if err != nil {
			return err
		}
		zr, err := gzip.NewReader(gz)
		if err != nil {
			return err
		}
		data, err := io.ReadAll(zr)
		gz.Close()
		if err != nil {
			return err
		}
		if err := b.w.write("assets/mermaid.min.js", data); err != nil {
			return err
		}
	}
	if len(b.site.CustomCSS) > 0 {
		if err := b.w.write("assets/custom.css", b.site.CustomCSS); err != nil {
			return err
		}
	}
	switch logo := b.site.Logo; {
	case logo == "":
	case strings.HasPrefix(logo, "http://") || strings.HasPrefix(logo, "https://"):
		b.logo = logo
	default:
		p, err := b.copyFile(path.Clean(logo), "assets")
		if err != nil {
			return fmt.Errorf("logo %s: %w", logo, err)
		}
		b.logo = p
	}
	return b.searchIndex()
}

type searchEntry struct {
	Title    string   `json:"t"`
	URL      string   `json:"u"`
	Section  string   `json:"s,omitempty"`
	Headings []string `json:"h,omitempty"`
	Text     string   `json:"x"`
}

func (b *builder) searchIndex() error {
	var entries []searchEntry
	for _, p := range b.pages {
		e := searchEntry{Title: p.Title, URL: urlOf(p.Slug), Text: p.Text}
		if p.Category != "" {
			e.Section = b.categoryTitle(p.Category)
		}
		for _, h := range p.Headings {
			if h.Level > 1 {
				e.Headings = append(e.Headings, h.Text)
			}
		}
		if len(e.Text) > 4000 {
			e.Text = e.Text[:4000]
		}
		entries = append(entries, e)
	}
	data, err := json.Marshal(entries)
	if err != nil {
		return err
	}
	return b.w.write("assets/search-index.js", append(append([]byte("window.INDOC_SEARCH="), data...), ";\n"...))
}

// urlOf is a page's URL relative to the site root.
func urlOf(slug string) string {
	if slug == "" {
		return ""
	}
	return slug + "/"
}

// rootPrefix leads from a page back to the site root.
func rootPrefix(slug string) string {
	if slug == "" {
		return ""
	}
	return strings.Repeat("../", strings.Count(slug, "/")+1)
}

// relURL links from one page to another.
func relURL(from, to string) string {
	href := rootPrefix(from) + urlOf(to)
	if href == "" {
		return "./"
	}
	return href
}

func refText(sec *doc.RefSection) string {
	var b strings.Builder
	b.WriteString(sec.Intro)
	for _, e := range sec.Entries {
		fmt.Fprintf(&b, " %s %s %s %s", e.Detail, e.Name, e.Handler, e.Description)
	}
	return b.String()
}

// writer writes files and removes those a previous build wrote but this one
// did not. It never deletes anything it did not write itself.
type writer struct {
	out     string
	written map[string]bool
}

func (w *writer) write(name string, data []byte) error {
	full := filepath.Join(w.out, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(full, data, 0o644); err != nil {
		return err
	}
	w.written[name] = true
	return nil
}

func (w *writer) finish() error {
	manifest := filepath.Join(w.out, manifestName)
	if prev, err := os.ReadFile(manifest); err == nil {
		for _, name := range strings.Split(string(prev), "\n") {
			if name == "" || w.written[name] || strings.Contains(name, "..") {
				continue
			}
			os.Remove(filepath.Join(w.out, filepath.FromSlash(name)))
		}
	} else if legacy, err := os.ReadFile(filepath.Join(w.out, "docs.js")); err == nil &&
		bytes.HasPrefix(legacy, []byte("window.INDOC_DOCS")) {
		// Output of the single-page viewer indoc shipped before 0.1,
		// recognized by its payload, so a user's own files are never touched.
		for _, name := range []string{"docs.js", "app.js", "style.css"} {
			if !w.written[name] {
				os.Remove(filepath.Join(w.out, name))
			}
		}
	}
	names := make([]string, 0, len(w.written))
	for n := range w.written {
		names = append(names, n)
	}
	sort.Strings(names)
	return os.WriteFile(manifest, []byte(strings.Join(names, "\n")+"\n"), 0o644)
}
