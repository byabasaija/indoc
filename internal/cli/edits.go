package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/byabasaija/indoc/internal/site"
)

// keepEdits moves hand edits in the output into the theme folder, where
// they survive rebuilds. It builds a fresh copy to compare against, so it
// knows exactly what changed:
//
//   - CSS: rules that are new or changed are appended to theme/custom.css,
//     which loads after the built-in stylesheet.
//   - Anything else: the edited file is saved under theme/edited/, because
//     a page is regenerated from its Markdown and an HTML edit cannot be
//     merged into it automatically.
func keepEdits(p *project, outDir string, out io.Writer) error {
	edited := site.Edited(outDir)
	if len(edited) == 0 {
		return nil
	}
	fresh, err := os.MkdirTemp("", "indoc-fresh-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(fresh)
	if err := buildSiteWith(p, fresh, true); err != nil {
		return err
	}
	dir := themeDir(p.paths)
	var css []string
	for _, name := range edited {
		mine, err := os.ReadFile(filepath.Join(outDir, filepath.FromSlash(name)))
		if err != nil {
			return err
		}
		if strings.HasSuffix(name, ".css") {
			orig, _ := os.ReadFile(filepath.Join(fresh, filepath.FromSlash(name)))
			if rules := changedRules(string(orig), string(mine)); len(rules) > 0 {
				css = append(css, fmt.Sprintf("/* Kept from a hand edit to %s on %s */\n%s", name, time.Now().Format("2006-01-02"), strings.Join(rules, "\n\n")))
				fmt.Fprintf(out, "kept: %s -> %d rule(s) in theme/custom.css\n", name, len(rules))
			} else {
				fmt.Fprintf(out, "kept: %s had no new or changed rules (removed rules cannot be kept as overrides)\n", name)
			}
			continue
		}
		dest := filepath.Join(dir, "edited", filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(dest, mine, 0o644); err != nil {
			return err
		}
		fmt.Fprintf(out, "kept: %s -> theme/edited/%s (move the change into a theme part: indoc theme eject %s)\n", name, name, guessPart(name))
	}
	if len(css) > 0 {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		path := filepath.Join(dir, "custom.css")
		prev, _ := os.ReadFile(path)
		if len(prev) == 0 {
			// Start from the old location, so its rules are not lost.
			prev, _ = os.ReadFile(filepath.Join(filepath.Dir(p.paths.Config), "custom.css"))
		}
		text := strings.TrimRight(string(prev), "\n")
		if text != "" {
			text += "\n\n"
		}
		text += strings.Join(css, "\n\n") + "\n"
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			return err
		}
	}
	return nil
}

// guessPart names the theme part an HTML edit most likely belongs in.
func guessPart(name string) string {
	if name == "index.html" {
		return "hero"
	}
	return "header"
}

var cssComment = regexp.MustCompile(`(?s)/\*.*?\*/`)

// cssBlocks splits a stylesheet into its top-level rules ("a { ... }" or a
// whole "@media ... { ... }"), comments removed.
func cssBlocks(css string) []string {
	css = cssComment.ReplaceAllString(css, "")
	var blocks []string
	depth, start := 0, 0
	for i, r := range css {
		switch r {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				if b := strings.TrimSpace(css[start : i+1]); b != "" {
					blocks = append(blocks, b)
				}
				start = i + 1
			}
		}
	}
	return blocks
}

var spaces = regexp.MustCompile(`\s+`)

// changedRules returns the rules in mine that are not in orig, in order.
func changedRules(orig, mine string) []string {
	have := map[string]bool{}
	for _, b := range cssBlocks(orig) {
		have[spaces.ReplaceAllString(b, " ")] = true
	}
	var out []string
	for _, b := range cssBlocks(mine) {
		if !have[spaces.ReplaceAllString(b, " ")] {
			out = append(out, b)
		}
	}
	return out
}
