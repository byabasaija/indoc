package doc

import (
	"strings"
	"unicode"
)

const (
	// KindRoutes is a reference section extracted from HTTP route declarations.
	KindRoutes = "routes"
	// KindConfig is a reference section extracted from environment variable reads.
	KindConfig = "config"
	// KindCommands is a reference section of command-line commands.
	KindCommands = "commands"
)

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

// RefSection is one block of extracted facts.
type RefSection struct {
	ID      string     `json:"id" yaml:"id"`
	Title   string     `json:"title" yaml:"title"`
	Kind    string     `json:"kind" yaml:"kind"`
	Intro   string     `json:"intro,omitempty" yaml:"intro,omitempty"`
	Entries []RefEntry `json:"entries" yaml:"entries"`
}

// RefEntry is a single route, configuration value, or command.
type RefEntry struct {
	Name        string `json:"name" yaml:"name"`
	Detail      string `json:"detail,omitempty" yaml:"detail,omitempty"`
	Handler     string `json:"handler,omitempty" yaml:"handler,omitempty"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
	Source      string `json:"source,omitempty" yaml:"source,omitempty"`
	URL         string `json:"url,omitempty" yaml:"url,omitempty"` // link to Source, when source_url is set
}

// Heading is a markdown heading used by the table of contents.
type Heading struct {
	ID    string `json:"id"`
	Text  string `json:"text"`
	Level int    `json:"level"`
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
