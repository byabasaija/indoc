// Package pages loads Markdown documentation pages with optional YAML
// frontmatter and finds the code citations in them.
package pages

import (
	"bufio"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/byabasaija/indoc/internal/glob"
	"gopkg.in/yaml.v3"
)

// Page is one Markdown documentation page.
type Page struct {
	Path     string // slash-separated, relative to the repository root
	Title    string
	Order    int
	HasOrder bool
	Audience string
	Layout   string
	Sources  []string // globs of code this page describes
	// Strict makes any change to a cited symbol, including its body, fail
	// check. For pages describing externally visible behavior.
	Strict    bool
	Body      string // Markdown without frontmatter
	Raw       []byte // whole file, for hashing
	Citations []Citation
	// Description is a one-line summary for search results and meta tags.
	Description string
	// Hero, Features, and Featured fill the landing layout (layout: landing).
	Hero     *Hero
	Features []Feature
	Featured []Action
	// Sections orders the landing page's blocks; nil means hero, features, body.
	Sections []Section
}

// Hero is the top of a landing page.
type Hero struct {
	Title   string   `yaml:"title"`
	Tagline string   `yaml:"tagline"`
	Image   string   `yaml:"image"`
	Actions []Action `yaml:"actions"`
}

// Action is a button or link: primary actions are filled, others outlined.
type Action struct {
	Text  string `yaml:"text"`
	Link  string `yaml:"link"`
	Theme string `yaml:"theme"` // primary or secondary
}

// Feature is a landing page card.
type Feature struct {
	Title    string `yaml:"title"`
	Details  string `yaml:"details"`
	Link     string `yaml:"link"`
	LinkText string `yaml:"link_text"`
}

// Section is one block of a landing page, in the order listed under
// `sections:`. In frontmatter it is either a bare type ("hero") or a map
// with a type and its options.
type Section struct {
	Type  string     `yaml:"type"`
	Title string     `yaml:"title"`
	Body  string     `yaml:"body"`  // markdown: Markdown content
	Repo  string     `yaml:"repo"`  // contributors: owner/repo; defaults to the GitHub repo in source_url
	Limit int        `yaml:"limit"` // contributors: how many to show (default 48, at most 100)
	Items []LogoItem `yaml:"items"` // logos
}

// LogoItem is one logo in a logos section, such as a company using the project.
type LogoItem struct {
	Image string `yaml:"image"` // path relative to the page, repository path, or URL
	Name  string `yaml:"name"`  // alt text and tooltip
	Link  string `yaml:"link"`
}

// SectionTypes are the landing blocks indoc knows.
var SectionTypes = []string{"hero", "features", "body", "markdown", "contributors", "logos"}

// UnmarshalYAML accepts "hero" as shorthand for {type: hero}.
func (s *Section) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind == yaml.ScalarNode {
		s.Type = n.Value
		return nil
	}
	type plain Section
	return n.Decode((*plain)(s))
}

func validSections(ss []Section) error {
	seen := map[string]bool{}
	for i, s := range ss {
		known := false
		for _, t := range SectionTypes {
			known = known || s.Type == t
		}
		switch {
		case !known:
			return fmt.Errorf("sections[%d]: unknown type %q (use %s)", i, s.Type, strings.Join(SectionTypes, ", "))
		case (s.Type == "hero" || s.Type == "features" || s.Type == "body") && seen[s.Type]:
			return fmt.Errorf("sections[%d]: %s appears twice", i, s.Type)
		case s.Type == "markdown" && strings.TrimSpace(s.Body) == "":
			return fmt.Errorf("sections[%d]: markdown needs a body", i)
		case s.Type == "logos" && len(s.Items) == 0:
			return fmt.Errorf("sections[%d]: logos needs items", i)
		case s.Type == "contributors" && s.Repo != "" && strings.Count(s.Repo, "/") != 1:
			return fmt.Errorf("sections[%d]: contributors repo must be owner/repo, got %q", i, s.Repo)
		case s.Limit < 0 || s.Limit > 100:
			return fmt.Errorf("sections[%d]: limit must be between 1 and 100", i)
		}
		for j, it := range s.Items {
			if strings.TrimSpace(it.Image) == "" {
				return fmt.Errorf("sections[%d].items[%d]: image is required", i, j)
			}
		}
		seen[s.Type] = true
	}
	return nil
}

// Citation is a `path::Symbol` reference in a page.
type Citation struct {
	Path   string // file, relative to the repository root
	Symbol string // qualified name within the file
	Line   int    // line in the page file
}

// Ref is the citation as written: "path::Symbol".
func (c Citation) Ref() string { return c.Path + "::" + c.Symbol }

// Anchored reports whether the page is tied to any code.
func (p Page) Anchored() bool { return len(p.Sources) > 0 || len(p.Citations) > 0 }

type frontmatter struct {
	Title           string    `yaml:"title"`
	Order           *int      `yaml:"order"`
	Audience        string    `yaml:"audience"`
	Layout          string    `yaml:"layout"`
	Sources         []string  `yaml:"sources"`
	Strict          bool      `yaml:"strict"`
	SidebarLabel    string    `yaml:"sidebar_label"`    // Docusaurus
	SidebarPosition *int      `yaml:"sidebar_position"` // Docusaurus
	Description     string    `yaml:"description"`
	Hero            *Hero     `yaml:"hero"`
	Features        []Feature `yaml:"features"`
	Featured        []Action  `yaml:"featured"`
	Sections        []Section `yaml:"sections"`
}

// Match returns the files (from a ListFiles result) selected by globs.
func Match(files, globs []string) []string {
	var out []string
	for _, f := range files {
		if !strings.EqualFold(path.Ext(f), ".md") {
			continue
		}
		for _, g := range globs {
			if glob.Match(g, f) {
				out = append(out, f)
				break
			}
		}
	}
	return out
}

// Load reads and parses the given page files under root, sorted by order and
// then path.
func Load(root string, files []string) ([]Page, error) {
	pages := make([]Page, 0, len(files))
	for _, f := range files {
		raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(f)))
		if err != nil {
			return nil, fmt.Errorf("read page %s: %w", f, err)
		}
		p, err := Parse(f, raw)
		if err != nil {
			return nil, err
		}
		pages = append(pages, p)
	}
	sort.SliceStable(pages, func(i, j int) bool {
		a, b := pages[i], pages[j]
		if a.HasOrder != b.HasOrder {
			return a.HasOrder
		}
		if a.Order != b.Order {
			return a.Order < b.Order
		}
		return a.Path < b.Path
	})
	return pages, nil
}

// Parse parses one page.
func Parse(pagePath string, raw []byte) (Page, error) {
	p := Page{Path: pagePath, Raw: raw}
	text := strings.ReplaceAll(string(raw), "\r\n", "\n")
	body, front, bodyLine, ok := splitFrontmatter(text)
	if ok {
		var fm frontmatter
		if err := yaml.Unmarshal([]byte(front), &fm); err != nil {
			return Page{}, fmt.Errorf("%s: invalid frontmatter: %w", pagePath, err)
		}
		p.Title = firstNonEmpty(fm.Title, fm.SidebarLabel)
		p.Audience = fm.Audience
		p.Layout = fm.Layout
		p.Strict = fm.Strict
		p.Description = strings.TrimSpace(fm.Description)
		p.Hero, p.Features, p.Featured = fm.Hero, fm.Features, fm.Featured
		if err := validSections(fm.Sections); err != nil {
			return Page{}, fmt.Errorf("%s: %w", pagePath, err)
		}
		p.Sections = fm.Sections
		for _, s := range fm.Sources {
			s = strings.TrimSpace(s)
			if s == "" {
				continue
			}
			if !glob.Valid(s) {
				return Page{}, fmt.Errorf("%s: invalid sources glob %q", pagePath, s)
			}
			p.Sources = append(p.Sources, s)
		}
		switch {
		case fm.Order != nil:
			p.Order, p.HasOrder = *fm.Order, true
		case fm.SidebarPosition != nil:
			p.Order, p.HasOrder = *fm.SidebarPosition, true
		}
	}
	p.Body = body
	if p.Title == "" {
		p.Title = firstHeading(body)
	}
	if p.Title == "" {
		p.Title = titleFromPath(pagePath)
	}
	p.Citations = findCitations(body, bodyLine)
	return p, nil
}

// splitFrontmatter separates a leading `---` YAML block. bodyLine is the
// 1-based line in the file where the body starts.
func splitFrontmatter(text string) (body, front string, bodyLine int, ok bool) {
	if !strings.HasPrefix(text, "---\n") {
		return text, "", 1, false
	}
	rest := text[4:]
	end := -1
	if strings.HasPrefix(rest, "---\n") || rest == "---" {
		end = 0
	} else if i := strings.Index(rest, "\n---\n"); i >= 0 {
		end = i + 1
	} else if strings.HasSuffix(rest, "\n---") {
		end = len(rest) - 3
	}
	if end < 0 {
		return text, "", 1, false
	}
	front = rest[:end]
	after := rest[end+3:]
	after = strings.TrimPrefix(after, "\n")
	bodyLine = 1 + strings.Count(text[:len(text)-len(after)], "\n")
	return after, front, bodyLine, true
}

// citationRe matches path::Symbol where path has a file extension.
var citationRe = regexp.MustCompile(`([A-Za-z0-9_][A-Za-z0-9_./-]*\.[A-Za-z0-9]+)::([A-Za-z_][A-Za-z0-9_]*(?:\.[A-Za-z_][A-Za-z0-9_]*)*)`)

func findCitations(body string, firstLine int) []Citation {
	var out []Citation
	seen := map[string]bool{}
	inFence := false
	fence := ""
	sc := bufio.NewScanner(strings.NewReader(body))
	sc.Buffer(make([]byte, 64*1024), 4<<20)
	for n := firstLine; sc.Scan(); n++ {
		line := sc.Text()
		trimmed := strings.TrimSpace(line)
		if marker := fenceMarker(trimmed); marker != "" {
			switch {
			case !inFence:
				inFence, fence = true, marker
			case strings.HasPrefix(trimmed, fence):
				inFence = false
			}
			continue
		}
		if inFence {
			continue
		}
		for _, m := range citationRe.FindAllStringSubmatch(line, -1) {
			c := Citation{Path: strings.TrimPrefix(m[1], "./"), Symbol: m[2], Line: n}
			if seen[c.Ref()] {
				continue
			}
			seen[c.Ref()] = true
			out = append(out, c)
		}
	}
	return out
}

func fenceMarker(line string) string {
	for _, m := range []string{"```", "~~~"} {
		if strings.HasPrefix(line, m) {
			return m
		}
	}
	return ""
}

func firstHeading(body string) string {
	inFence := false
	for _, line := range strings.Split(body, "\n") {
		t := strings.TrimSpace(line)
		if fenceMarker(t) != "" {
			inFence = !inFence
			continue
		}
		if !inFence && strings.HasPrefix(t, "# ") {
			return strings.TrimSpace(t[2:])
		}
	}
	return ""
}

func titleFromPath(p string) string {
	base := strings.TrimSuffix(path.Base(p), path.Ext(p))
	if strings.EqualFold(base, "index") || strings.EqualFold(base, "readme") {
		if dir := path.Base(path.Dir(p)); dir != "." && dir != "/" {
			base = dir
		} else {
			return "Overview"
		}
	}
	base = strings.NewReplacer("-", " ", "_", " ").Replace(base)
	if base == "" {
		return "Untitled"
	}
	return strings.ToUpper(base[:1]) + base[1:]
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// Linkify turns citations outside fenced code blocks into Markdown links.
// link returns the URL for a citation, or "" to leave it as written. A
// citation wrapped in backticks keeps them inside the link text.
func Linkify(body string, link func(Citation) string) string {
	lines := strings.Split(body, "\n")
	inFence, fence := false, ""
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if marker := fenceMarker(trimmed); marker != "" {
			switch {
			case !inFence:
				inFence, fence = true, marker
			case strings.HasPrefix(trimmed, fence):
				inFence = false
			}
			continue
		}
		if inFence {
			continue
		}
		lines[i] = linkifyLine(line, link)
	}
	return strings.Join(lines, "\n")
}

func linkifyLine(line string, link func(Citation) string) string {
	var b strings.Builder
	last := 0
	for _, m := range citationRe.FindAllStringSubmatchIndex(line, -1) {
		start, end := m[0], m[1]
		c := Citation{Path: strings.TrimPrefix(line[m[2]:m[3]], "./"), Symbol: line[m[4]:m[5]]}
		ticked := start > 0 && end < len(line) && line[start-1] == '`' && line[end] == '`'
		if ticked {
			start, end = start-1, end+1
		}
		// Leave citations that are already link text ([x]) or a link target (](x)) alone.
		if start > 0 && line[start-1] == '[' || start > 1 && line[start-2:start] == "](" {
			continue
		}
		url := link(c)
		if url == "" || start < last {
			continue
		}
		b.WriteString(line[last:start])
		b.WriteString("[" + line[start:end] + "](" + url + ")")
		last = end
	}
	b.WriteString(line[last:])
	return b.String()
}
