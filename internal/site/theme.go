package site

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"html/template"
	"regexp"
	"strings"
)

// PartNames are the parts of a page a theme can replace, in page order.
var PartNames = []string{"header", "sidebar", "hero", "features", "footer"}

// IsPart reports whether name is a replaceable part.
func IsPart(name string) bool {
	for _, p := range PartNames {
		if p == name {
			return true
		}
	}
	return false
}

// BuiltinPart returns the built-in template of a part, without its define
// wrapper: what `indoc theme eject` copies for editing.
func BuiltinPart(name string) (string, error) {
	if !IsPart(name) {
		return "", fmt.Errorf("unknown part %q (parts: %s)", name, strings.Join(PartNames, ", "))
	}
	data, err := files.ReadFile("templates/parts/" + name + ".html")
	if err != nil {
		return "", err
	}
	s := strings.TrimSpace(string(data))
	s = strings.TrimPrefix(s, `{{define "`+name+`"}}`)
	s = strings.TrimSuffix(s, "{{end}}")
	return strings.Trim(s, "\n") + "\n", nil
}

// PartHash fingerprints a built-in part, so an ejected copy can tell when
// the built-in changed after it was copied.
func PartHash(name string) string {
	src, err := BuiltinPart(name)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256([]byte(src))
	return hex.EncodeToString(sum[:4])
}

var stampRe = regexp.MustCompile(`indoc theme part "([a-z]+)", ejected from indoc ([^;]+); built-in ([0-9a-f]{8})`)

// Stamp is the comment an ejected part starts with.
func Stamp(name, version string) string {
	return fmt.Sprintf("{{/* indoc theme part %q, ejected from indoc %s; built-in %s.\n   Edit freely. Delete this file to use the built-in part again. */}}\n", name, version, PartHash(name))
}

// Outdated reports, for an ejected part's source, the version it was copied
// from when the built-in part has changed since. ok is false otherwise,
// including for parts written from scratch without a stamp.
func Outdated(name, src string) (from string, ok bool) {
	m := stampRe.FindStringSubmatch(src)
	if m == nil || m[1] != name || m[3] == PartHash(name) {
		return "", false
	}
	return m[2], true
}

// templates returns the page templates with the site's theme parts applied.
func (b *builder) templates() (*template.Template, error) {
	t, err := tmpl.Clone()
	if err != nil {
		return nil, err
	}
	for _, name := range PartNames {
		src, ok := b.site.Parts[name]
		if !ok {
			continue
		}
		if _, err := t.New(name).Parse(src); err != nil {
			return nil, fmt.Errorf("theme part %s: %w", name, err)
		}
	}
	for name := range b.site.Parts {
		if !IsPart(name) {
			return nil, fmt.Errorf("theme part %q does not exist (parts: %s)", name, strings.Join(PartNames, ", "))
		}
	}
	return t, nil
}
