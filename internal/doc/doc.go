package doc

import (
	"encoding/hex"
	"fmt"
	"strings"
	"unicode"
)

const (
	// KindRoutes is a reference section extracted from HTTP route declarations.
	KindRoutes = "routes"
	// KindConfig is a reference section extracted from environment variable reads.
	KindConfig = "config"
)

// Document is the site the agent writes and the bundler compiles.
// Overview and guides are prose. Reference is facts extracted from source.
type Document struct {
	ProjectName string       `json:"project_name" yaml:"project_name"`
	Appearance  *Appearance  `json:"appearance,omitempty" yaml:"appearance,omitempty"`
	Overview    Page         `json:"overview" yaml:"overview"`
	Guides      []Page       `json:"guides" yaml:"guides"`
	Reference   []RefSection `json:"reference" yaml:"reference"`
}

// Appearance contains optional branding for the generated site.
type Appearance struct {
	Logo   string  `json:"logo" yaml:"logo"`
	Colors *Colors `json:"colors,omitempty" yaml:"colors,omitempty"`
}

// Colors defines the site's primary and secondary accent colors.
type Colors struct {
	Primary string `json:"primary,omitempty" yaml:"primary,omitempty"`
	Accent  string `json:"accent,omitempty" yaml:"accent,omitempty"`
}

// Page is one narrative page.
type Page struct {
	ID       string `json:"id" yaml:"id"`
	Title    string `json:"title" yaml:"title"`
	Markdown string `json:"markdown" yaml:"markdown"`
}

// RefSection is one block of extracted facts.
type RefSection struct {
	ID      string     `json:"id" yaml:"id"`
	Title   string     `json:"title" yaml:"title"`
	Kind    string     `json:"kind" yaml:"kind"`
	Intro   string     `json:"intro,omitempty" yaml:"intro,omitempty"`
	Entries []RefEntry `json:"entries" yaml:"entries"`
}

// RefEntry is a single route or configuration value.
type RefEntry struct {
	Name        string `json:"name" yaml:"name"`
	Detail      string `json:"detail,omitempty" yaml:"detail,omitempty"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
	Source      string `json:"source,omitempty" yaml:"source,omitempty"`
}

// Heading is a markdown heading used by the table of contents.
type Heading struct {
	ID    string `json:"id"`
	Text  string `json:"text"`
	Level int    `json:"level"`
}

// StaleRef is a previously published reference entry whose source file changed.
type StaleRef struct {
	Kind   string `json:"kind"`
	Name   string `json:"name"`
	Detail string `json:"detail,omitempty"`
	Source string `json:"source"`
	Reason string `json:"reason"`
}

// Normalize fills ids and titles, then checks the document is bundleable.
func Normalize(d *Document) error {
	if d == nil {
		return fmt.Errorf("document is nil")
	}
	d.ProjectName = strings.TrimSpace(d.ProjectName)
	if d.Guides == nil {
		d.Guides = []Page{}
	}
	if d.Reference == nil {
		d.Reference = []RefSection{}
	}

	if strings.TrimSpace(d.Overview.Title) == "" {
		d.Overview.Title = "Overview"
	}
	d.Overview.ID = "overview"

	usedGuides := map[string]bool{"overview": true}
	for i := range d.Guides {
		title := strings.TrimSpace(d.Guides[i].Title)
		d.Guides[i].Title = title
		base := d.Guides[i].ID
		if strings.TrimSpace(base) == "" {
			base = Slug(title)
		}
		d.Guides[i].ID = uniqueSlug(Slug(base), usedGuides)
	}

	usedRefs := map[string]bool{}
	for i := range d.Reference {
		kind := strings.TrimSpace(d.Reference[i].Kind)
		d.Reference[i].Kind = kind
		if strings.TrimSpace(d.Reference[i].Title) == "" {
			d.Reference[i].Title = defaultRefTitle(kind)
		}
		base := d.Reference[i].ID
		if strings.TrimSpace(base) == "" {
			base = kind
		}
		d.Reference[i].ID = uniqueSlug(Slug(base), usedRefs)
		if d.Reference[i].Entries == nil {
			d.Reference[i].Entries = []RefEntry{}
		}
	}

	return Validate(d)
}

// Validate checks a normalized document.
func Validate(d *Document) error {
	if d == nil {
		return fmt.Errorf("document is nil")
	}
	if strings.TrimSpace(d.ProjectName) == "" {
		return fmt.Errorf("project_name is required")
	}
	if d.Appearance != nil && d.Appearance.Colors != nil {
		for _, color := range []struct {
			name  string
			value string
		}{
			{name: "primary", value: d.Appearance.Colors.Primary},
			{name: "accent", value: d.Appearance.Colors.Accent},
		} {
			if color.value != "" && !isHexColor(color.value) {
				return fmt.Errorf("appearance.colors.%s must be a 6-digit hex color", color.name)
			}
		}
	}
	if strings.TrimSpace(d.Overview.Markdown) == "" {
		return fmt.Errorf("overview markdown is required")
	}
	seenGuides := map[string]bool{}
	for i, page := range d.Guides {
		if page.ID == "" || page.ID == "overview" {
			return fmt.Errorf("guides[%d] needs an id other than overview", i)
		}
		if seenGuides[page.ID] {
			return fmt.Errorf("duplicate guide id %q", page.ID)
		}
		seenGuides[page.ID] = true
		if strings.TrimSpace(page.Title) == "" {
			return fmt.Errorf("guides[%d] title is required", i)
		}
		if strings.TrimSpace(page.Markdown) == "" {
			return fmt.Errorf("guides[%d] markdown is required", i)
		}
	}
	seenRefs := map[string]bool{}
	for i, section := range d.Reference {
		if section.Kind != KindRoutes && section.Kind != KindConfig {
			return fmt.Errorf("reference[%d] kind must be %q or %q", i, KindRoutes, KindConfig)
		}
		if section.ID == "" {
			return fmt.Errorf("reference[%d] id is required", i)
		}
		if seenRefs[section.ID] {
			return fmt.Errorf("duplicate reference id %q", section.ID)
		}
		seenRefs[section.ID] = true
		if len(section.Entries) == 0 {
			return fmt.Errorf("reference[%d] has no entries", i)
		}
		for j, entry := range section.Entries {
			if strings.TrimSpace(entry.Name) == "" {
				return fmt.Errorf("reference[%d].entries[%d] name is required", i, j)
			}
			if section.Kind == KindRoutes && strings.TrimSpace(entry.Detail) == "" {
				return fmt.Errorf("reference[%d].entries[%d] method is required", i, j)
			}
		}
	}
	return nil
}

func isHexColor(color string) bool {
	if len(color) != 7 || color[0] != '#' {
		return false
	}
	_, err := hex.DecodeString(color[1:])
	return err == nil
}

// StaleReference lists reference entries from a previous document whose source file changed.
func StaleReference(prev *Document, modified, deleted []string) []StaleRef {
	if prev == nil {
		return []StaleRef{}
	}
	mod := pathSet(modified)
	del := pathSet(deleted)
	var stale []StaleRef
	for _, section := range prev.Reference {
		for _, entry := range section.Entries {
			path := SourcePath(entry.Source)
			reason := ""
			if del[path] {
				reason = "deleted"
			} else if mod[path] {
				reason = "modified"
			}
			if reason == "" {
				continue
			}
			stale = append(stale, StaleRef{
				Kind:   section.Kind,
				Name:   entry.Name,
				Detail: entry.Detail,
				Source: entry.Source,
				Reason: reason,
			})
		}
	}
	if stale == nil {
		return []StaleRef{}
	}
	return stale
}

// SourcePath returns the file path from a "path:line" source citation.
func SourcePath(source string) string {
	source = strings.TrimSpace(source)
	i := strings.LastIndex(source, ":")
	if i <= 0 {
		return source
	}
	line := source[i+1:]
	if line == "" {
		return source
	}
	for _, r := range line {
		if r < '0' || r > '9' {
			return source
		}
	}
	return source[:i]
}

// Slug turns a title into a URL id.
func Slug(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	dash := false
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			dash = false
			continue
		}
		if !dash && b.Len() > 0 {
			b.WriteByte('-')
			dash = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "page"
	}
	return out
}

func uniqueSlug(base string, used map[string]bool) string {
	if base == "" {
		base = "page"
	}
	if !used[base] {
		used[base] = true
		return base
	}
	for i := 2; ; i++ {
		id := fmt.Sprintf("%s-%d", base, i)
		if !used[id] {
			used[id] = true
			return id
		}
	}
}

func defaultRefTitle(kind string) string {
	switch kind {
	case KindRoutes:
		return "Routes"
	case KindConfig:
		return "Configuration"
	default:
		return kind
	}
}

func pathSet(paths []string) map[string]bool {
	set := make(map[string]bool, len(paths))
	for _, path := range paths {
		set[strings.TrimSpace(path)] = true
	}
	return set
}
