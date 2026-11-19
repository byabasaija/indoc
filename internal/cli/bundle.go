package cli

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/byabasaija/indoc/internal/doc"
	"github.com/byabasaija/indoc/internal/drift"
	"github.com/byabasaija/indoc/internal/pages"
	"github.com/byabasaija/indoc/internal/reference"
	"github.com/byabasaija/indoc/internal/site"
	"github.com/spf13/cobra"
)

var bundleOut string

var bundleCmd = &cobra.Command{
	Use:   "bundle",
	Short: "Build the static docs site from the pages",
	Long: `bundle validates every page, extracts the reference (routes, environment
variables) from source, and writes the static site to output_dir.

It refuses to build when a citation or sources glob is broken. After writing
the site it records new and edited pages in .indoc/lock.json, exactly as
` + "`indoc ack`" + ` with no arguments would.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runBundle(cfgFile, bundleOut, cmd.OutOrStdout())
	},
}

func init() {
	bundleCmd.Flags().StringVarP(&bundleOut, "out", "o", "", "output directory (default: output_dir in .indoc/config.yaml)")
	rootCmd.AddCommand(bundleCmd)
}

func runBundle(configPath, outDir string, out io.Writer) error {
	p, err := openProject(configPath)
	if err != nil {
		return err
	}
	if outDir == "" {
		outDir = filepath.Join(p.paths.Root, p.cfg.OutputDir)
	}
	if err := buildSite(p, outDir); err != nil {
		return err
	}
	res, err := drift.Ack(p.repo, p.pages, p.lock, nil)
	if err != nil {
		return err
	}
	if err := p.lock.Save(p.paths.Lock); err != nil {
		return err
	}
	fmt.Fprintf(out, "Wrote %d pages to %s\n", len(p.pages), outDir)
	if n := len(res.Stamped); n > 0 {
		fmt.Fprintf(out, "Recorded %d new or edited pages in %s\n", n, p.paths.Lock)
	}
	fmt.Fprintln(out, "Preview locally with: indoc preview")
	return nil
}

// buildSite writes the site without touching the lock.
func buildSite(p *project, outDir string) error {
	var broken []string
	for _, f := range drift.Check(p.repo, p.pages, p.lock) {
		if f.Kind == drift.Broken {
			broken = append(broken, fmt.Sprintf("  %s: %s %s", f.Page, f.Ref, f.Detail))
		}
	}
	if len(broken) > 0 {
		return fmt.Errorf("broken citations or sources:\n%s", strings.Join(broken, "\n"))
	}

	ref, err := reference.Extract(p.paths.Root, p.repo.Files)
	if err != nil {
		return err
	}
	linkSources(p, ref.Sections)
	return site.Build(siteFor(p, ref.Sections), outDir)
}

// linkSources sets each reference entry's URL from source_url.
func linkSources(p *project, sections []doc.RefSection) {
	for i := range sections {
		for j := range sections[i].Entries {
			e := &sections[i].Entries[j]
			file, line := splitSource(e.Source)
			e.URL = p.cfg.PublicLink(file, line)
		}
	}
}

func splitSource(source string) (string, int) {
	file := doc.SourcePath(source)
	line, _ := strconv.Atoi(strings.TrimPrefix(source[len(file):], ":"))
	return file, line
}

// citationLink resolves a citation to its symbol's current line, or "" when
// the site does not link to the code.
func citationLink(p *project) func(pages.Citation) string {
	return func(c pages.Citation) string {
		if !p.cfg.SourceLinks || p.cfg.SourceURL == "" {
			return ""
		}
		f, ok := p.repo.Analyze(c.Path)
		if !ok {
			return ""
		}
		line := 0
		if sym, found := f.Lookup(c.Symbol); found {
			line = sym.DecoratorLine
		}
		return p.cfg.SourceLink(c.Path, line)
	}
}

// citationTarget is where a citation in the text points: the code when the
// site links to it, otherwise the page's "Checked against the code" box,
// which shows the full path.
func citationTarget(p *project) func(pages.Citation) string {
	link := citationLink(p)
	return func(c pages.Citation) string {
		if u := link(c); u != "" {
			return u
		}
		return "#verified"
	}
}

// verified lists what a page was checked against, for the box at its end.
func verified(p *project, pg pages.Page) []site.Ref {
	link := citationLink(p)
	seen := map[string]bool{}
	var out []site.Ref
	for _, c := range pg.Citations {
		if seen[c.Ref()] {
			continue
		}
		seen[c.Ref()] = true
		out = append(out, site.Ref{Label: c.Ref(), URL: link(c)})
	}
	return out
}

// siteFor collects what the site is built from.
func siteFor(p *project, sections []doc.RefSection) *site.Site {
	s := &site.Site{
		Name:        p.cfg.ProjectName,
		Footer:      p.cfg.Footer,
		SiteURL:     p.cfg.SiteURL,
		AllowHTML:   p.cfg.AllowHTML,
		ShowAuthors: p.cfg.ShowAuthors,
		FileURLs:    p.cfg.FileURLs(),
		RepoRoot:    p.paths.Root,
		Reference:   sections,
	}
	if p.cfg.SourceLinks {
		s.Repo = githubRepo(p.cfg.SourceURL)
	}
	if s.Name == "" {
		s.Name = "Documentation"
	}
	if a := p.cfg.Appearance; a != nil {
		s.Logo, s.Colors = a.Logo, a.Colors
	}
	for _, l := range p.cfg.Links {
		s.Links = append(s.Links, site.Link{Text: l.Text, URL: l.URL})
	}
	dir := filepath.Dir(p.paths.Config)
	if css, err := os.ReadFile(filepath.Join(dir, "custom.css")); err == nil {
		s.CustomCSS = css
	}
	if head, err := os.ReadFile(filepath.Join(dir, "head.html")); err == nil {
		s.HeadHTML = string(head)
	}
	editURL, editLabel := "", ""
	for _, pg := range p.pages {
		if p.cfg.SourceLinks {
			editURL, editLabel = editLink(p.cfg.SourceLink(pg.Path, 0))
		}
		style := pg.CitationStyle
		if style == "" {
			style = p.cfg.CitationStyle()
		}
		var checked []site.Ref
		var covers []string
		if style != pages.CitationsHidden {
			checked, covers = verified(p, pg), pg.Sources
		}
		updated, author := gitLastChange(p.paths.Root, pg.Path)
		if !p.cfg.ShowAuthors {
			author = ""
		}
		s.Pages = append(s.Pages, site.Page{
			Source: pg.Path, Title: pg.Title, Description: pg.Description,
			Order: pg.Order, HasOrder: pg.HasOrder, Layout: pg.Layout,
			Markdown: pages.RenderCitations(pg.Body, style, citationTarget(p)),
			Verified: checked, Covers: covers,
			Hero: pg.Hero, Features: pg.Features, Featured: pg.Featured, Sections: pg.Sections,
			EditURL: editURL, EditLabel: editLabel, Updated: updated, Author: author,
		})
	}
	return s
}

// editLink turns a source link into an edit link where the host has one.
func editLink(view string) (string, string) {
	switch {
	case strings.Contains(view, "github.com/") && strings.Contains(view, "/blob/"):
		return strings.Replace(view, "/blob/", "/edit/", 1), "Edit this page"
	case strings.Contains(view, "/-/blob/"):
		return strings.Replace(view, "/-/blob/", "/-/edit/", 1), "Edit this page"
	case view != "":
		return view, "View source"
	}
	return "", ""
}

// githubRepo extracts owner/repo from a GitHub source_url.
func githubRepo(sourceURL string) string {
	rest, ok := strings.CutPrefix(sourceURL, "https://github.com/")
	if !ok {
		return ""
	}
	parts := strings.SplitN(rest, "/", 3)
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return ""
	}
	return parts[0] + "/" + parts[1]
}

// gitLastChange returns the date (YYYY-MM-DD) and author of the last commit
// that touched file, or empty strings outside git.
func gitLastChange(root, file string) (string, string) {
	out, err := exec.Command("git", "-C", root, "log", "-1", "--format=%cs%x1f%an", "--", file).Output()
	if err != nil {
		return "", ""
	}
	date, author, _ := strings.Cut(strings.TrimSpace(string(out)), "\x1f")
	return date, author
}
