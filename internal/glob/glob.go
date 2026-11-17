// Package glob matches slash-separated paths against patterns with `**`.
//
//   - any run of characters except '/'
//     ?     one character except '/'
//     **    any run of characters including '/', as a whole segment
//     [..]  character class, as in filepath.Match
//
// A pattern without a '/' still matches only at the root: "*.md" does not
// match "docs/a.md". Use "**/*.md" for that.
package glob

import (
	"path"
	"strings"
)

// Match reports whether name matches pattern. Both use '/' separators.
// Malformed patterns never match.
func Match(pattern, name string) bool {
	return match(split(pattern), split(name))
}

// Valid reports whether every segment of pattern is well formed.
func Valid(pattern string) bool {
	for _, seg := range split(pattern) {
		if seg == "**" {
			continue
		}
		if _, err := path.Match(seg, ""); err != nil {
			return false
		}
	}
	return true
}

func split(p string) []string {
	p = strings.Trim(path.Clean("/"+strings.TrimPrefix(p, "./")), "/")
	if p == "" {
		return nil
	}
	return strings.Split(p, "/")
}

func match(pat, name []string) bool {
	for len(pat) > 0 {
		if pat[0] == "**" {
			rest := pat[1:]
			for i := 0; i <= len(name); i++ {
				if match(rest, name[i:]) {
					return true
				}
			}
			return false
		}
		if len(name) == 0 {
			return false
		}
		ok, err := path.Match(pat[0], name[0])
		if err != nil || !ok {
			return false
		}
		pat, name = pat[1:], name[1:]
	}
	return len(name) == 0
}
