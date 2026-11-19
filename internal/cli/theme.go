package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/byabasaija/indoc/internal/config"
	"github.com/byabasaija/indoc/internal/site"
	"github.com/byabasaija/indoc/internal/version"
)

// themeDir is where a project's own look lives. indoc reads it and never
// writes it, except when asked to eject a part.
func themeDir(p config.Paths) string { return filepath.Join(filepath.Dir(p.Config), "theme") }

type theme struct {
	css      []byte
	head     string
	parts    map[string]string
	warnings []string
}

// loadTheme reads .indoc/theme/: custom.css, head.html, and part templates.
// custom.css and head.html directly in .indoc/ are read as before.
func loadTheme(p *project) theme {
	th := theme{parts: map[string]string{}}
	dir := themeDir(p.paths)
	read := func(name string) ([]byte, bool) {
		for _, d := range []string{dir, filepath.Dir(p.paths.Config)} {
			if b, err := os.ReadFile(filepath.Join(d, name)); err == nil {
				return b, true
			}
		}
		return nil, false
	}
	if b, ok := read("custom.css"); ok {
		th.css = b
	}
	if b, ok := read("head.html"); ok {
		th.head = string(b)
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		name := strings.TrimSuffix(e.Name(), ".html")
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".html") || name == "head" {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		th.parts[name] = string(b)
		if from, old := site.Outdated(name, string(b)); old {
			th.warnings = append(th.warnings, fmt.Sprintf("theme part %s was copied from indoc %s and the built-in %s has changed since; compare with `indoc theme show %s`", name, from, name, name))
		}
	}
	return th
}

var themeForce bool

var themeCmd = &cobra.Command{
	Use:   "theme",
	Short: "List the parts of the site's look you can replace",
	Long: `The site's look is yours to change in .indoc/theme/, which indoc reads
and never overwrites:

  custom.css    loaded after the built-in stylesheet
  head.html     added to every page's <head> (analytics, web fonts)
  <part>.html   replaces a part of every page: ` + strings.Join(site.PartNames, ", ") + `

Colors, fonts, corner radius, content width, and the default light or dark
theme are settings under appearance: in .indoc/config.yaml.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runThemeList(cfgFile, cmd.OutOrStdout())
	},
}

var themeEjectCmd = &cobra.Command{
	Use:   "eject <part>",
	Short: "Copy a built-in part into .indoc/theme/ to edit it",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runThemeEject(cfgFile, cmd.OutOrStdout(), args[0], themeForce)
	},
}

var themeShowCmd = &cobra.Command{
	Use:   "show <part>",
	Short: "Print a built-in part",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		src, err := site.BuiltinPart(args[0])
		if err != nil {
			return err
		}
		fmt.Fprint(cmd.OutOrStdout(), src)
		return nil
	},
}

func init() {
	themeEjectCmd.Flags().BoolVarP(&themeForce, "force", "f", false, "replace an existing file")
	themeCmd.AddCommand(themeEjectCmd, themeShowCmd)
	rootCmd.AddCommand(themeCmd)
}

func runThemeList(configPath string, out io.Writer) error {
	paths := config.Resolve(configPath)
	dir := themeDir(paths)
	rel, _ := filepath.Rel(paths.Root, dir)
	fmt.Fprintf(out, "Theme folder: %s\n\n", rel)
	have := map[string]bool{}
	entries, _ := os.ReadDir(dir)
	var extra []string
	for _, e := range entries {
		have[e.Name()] = true
		if name := strings.TrimSuffix(e.Name(), ".html"); strings.HasSuffix(e.Name(), ".html") && name != "head" && !site.IsPart(name) {
			extra = append(extra, e.Name())
		}
	}
	for _, f := range []string{"custom.css", "head.html"} {
		state := "-"
		if have[f] {
			state = "yours"
		}
		fmt.Fprintf(out, "  %-14s %s\n", f, state)
	}
	for _, name := range site.PartNames {
		state := "built-in"
		if b, err := os.ReadFile(filepath.Join(dir, name+".html")); err == nil {
			state = "yours"
			if from, old := site.Outdated(name, string(b)); old {
				state = "yours (copied from " + from + "; the built-in has changed since)"
			}
		}
		fmt.Fprintf(out, "  %-14s %s\n", name+".html", state)
	}
	sort.Strings(extra)
	for _, f := range extra {
		fmt.Fprintf(out, "  %-14s not a part; it stops the build\n", f)
	}
	fmt.Fprintln(out, "\nCopy a part to edit it: indoc theme eject <part>")
	return nil
}

func runThemeEject(configPath string, out io.Writer, name string, force bool) error {
	src, err := site.BuiltinPart(name)
	if err != nil {
		return err
	}
	paths := config.Resolve(configPath)
	dir := themeDir(paths)
	dest := filepath.Join(dir, name+".html")
	if _, err := os.Stat(dest); err == nil && !force {
		return fmt.Errorf("%s already exists; edit it, or use --force to start again from the built-in", dest)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(dest, []byte(site.Stamp(name, version.Version)+src), 0o644); err != nil {
		return err
	}
	rel, _ := filepath.Rel(paths.Root, dest)
	fmt.Fprintf(out, "Copied the built-in %s to %s. Edit it; every page uses it from the next build. Commit it with the config.\n", name, rel)
	return nil
}
