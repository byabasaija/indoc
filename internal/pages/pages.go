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
	Title           string   `yaml:"title"`
	Order           *int     `yaml:"order"`
	Audience        string   `yaml:"audience"`
	Layout          string   `yaml:"layout"`
	Sources         []string `yaml:"sources"`
	Strict          bool     `yaml:"strict"`
	SidebarLabel    string   `yaml:"sidebar_label"`    // Docusaurus
	SidebarPosition *int     `yaml:"sidebar_position"` // Docusaurus
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
