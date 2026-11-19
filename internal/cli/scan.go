package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/byabasaija/indoc/internal/code"
	"github.com/byabasaija/indoc/internal/doc"
	"github.com/byabasaija/indoc/internal/drift"
	"github.com/byabasaija/indoc/internal/projectmap"
	"github.com/byabasaija/indoc/internal/reference"
	"github.com/spf13/cobra"
)

var scanJSON bool

var scanCmd = &cobra.Command{
	Use:   "scan",
	Short: "Report pages, what needs updating, and extracted reference facts (for agents)",
	Long: `scan is the agent's starting point. It reports every page with its sources and
citations, every check finding (stale, drift, broken, ...) with the reason,
and the reference facts extracted from source.

scan never writes anything.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runScan(cfgFile, cmd.OutOrStdout(), scanJSON)
	},
}

func init() {
	scanCmd.Flags().BoolVar(&scanJSON, "json", false, "print the report as JSON")
	rootCmd.AddCommand(scanCmd)
}

type scanPage struct {
	Path      string   `json:"path"`
	Title     string   `json:"title"`
	Audience  string   `json:"audience,omitempty"`
	Sources   []string `json:"sources"`
	Citations []string `json:"citations"`
}

type scanReport struct {
	ProjectName string           `json:"project_name"`
	OutputDir   string           `json:"output_dir"`
	Pages       []scanPage       `json:"pages"`
	Findings    []drift.Finding  `json:"findings"`
	Reference   []doc.RefSection `json:"reference"`
	// Coverage says what each extractor detected and could not resolve.
	Coverage []reference.Coverage `json:"coverage"`
	// Map summarizes the repository for choosing what to document.
	Map *projectmap.Map `json:"map"`
	// Logo is appearance.logo from the config; empty when none is set.
	Logo string `json:"logo"`
	// Publish is the publish target (pages, host, app, none); empty when
	// not chosen yet.
	Publish string `json:"publish"`
	// Files is the number of repository files; Parsed of those have a
	// language parser and support path::Symbol citations.
	Files  int `json:"files"`
	Parsed int `json:"parsed"`
}

func runScan(configPath string, out io.Writer, asJSON bool) error {
	p, err := openProjectForScan(configPath)
	if err != nil {
		return err
	}
	ref, err := reference.Extract(p.paths.Root, p.repo.Files)
	if err != nil {
		return err
	}
	sections := ref.Sections
	report := scanReport{
		ProjectName: p.cfg.ProjectName,
		OutputDir:   p.cfg.OutputDir,
		Pages:       []scanPage{},
		Findings:    nonNil(drift.Check(p.repo, p.pages, p.lock)),
		Reference:   sections,
		Coverage:    ref.Coverage,
		Map:         projectmap.Build(p.paths.Root, p.repo.Files, pagePaths(p)),
		Files:       len(p.repo.Files),
	}
	if a := p.cfg.Appearance; a != nil {
		report.Logo = a.Logo
	}
	if p.cfg.Publish != nil {
		report.Publish = p.cfg.Publish.Target
	}
	for _, f := range p.repo.Files {
		if code.Parsable(f) {
			report.Parsed++
		}
	}
	for _, pg := range p.pages {
		sp := scanPage{Path: pg.Path, Title: pg.Title, Audience: pg.Audience, Sources: pg.Sources, Citations: []string{}}
		if sp.Sources == nil {
			sp.Sources = []string{}
		}
		for _, c := range pg.Citations {
			sp.Citations = append(sp.Citations, c.Ref())
		}
		report.Pages = append(report.Pages, sp)
	}

	if asJSON {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		enc.SetEscapeHTML(false)
		return enc.Encode(report)
	}
	fmt.Fprintf(out, "%s: %d pages, %d files (%d parsed)\n", report.ProjectName, len(report.Pages), report.Files, report.Parsed)
	printMap(out, report.Map)
	fmt.Fprintln(out)
	for _, sp := range report.Pages {
		fmt.Fprintf(out, "  %-40s %d sources, %d citations\n", sp.Path, len(sp.Sources), len(sp.Citations))
	}
	fmt.Fprintf(out, "\nreference: %s\n", summarizeReference(sections))
	for _, c := range ref.Coverage {
		fmt.Fprintf(out, "  %s: %d entries from %d files", c.Extractor, c.Entries, c.Files)
		if len(c.Unresolved) > 0 {
			fmt.Fprintf(out, ", %d unresolved", len(c.Unresolved))
		}
		fmt.Fprintln(out)
		for _, w := range c.Warnings {
			fmt.Fprintf(out, "    warning: %s\n", w)
		}
	}
	fmt.Fprintln(out)
	printFindings(out, len(p.pages), report.Findings, false)
	return nil
}

func summarizeReference(sections []doc.RefSection) string {
	if len(sections) == 0 {
		return "none extracted"
	}
	parts := make([]string, len(sections))
	for i, s := range sections {
		parts[i] = fmt.Sprintf("%d %s", len(s.Entries), s.Kind)
	}
	return strings.Join(parts, ", ")
}

func pagePaths(p *project) []string {
	out := make([]string, len(p.pages))
	for i, pg := range p.pages {
		out[i] = pg.Path
	}
	return out
}

func printMap(out io.Writer, m *projectmap.Map) {
	var langs []string
	for _, l := range m.Languages {
		mark := ""
		if l.Parsed {
			mark = "*"
		}
		langs = append(langs, fmt.Sprintf("%s%s %d", l.Name, mark, l.Files))
	}
	if len(langs) > 0 {
		fmt.Fprintf(out, "languages: %s  (* = parsed)\n", strings.Join(langs, ", "))
	}
	if len(m.Frameworks) > 0 {
		fmt.Fprintf(out, "frameworks: %s\n", strings.Join(m.Frameworks, ", "))
	}
	if len(m.EntryPoints) > 0 {
		fmt.Fprintf(out, "entry points: %s\n", strings.Join(m.EntryPoints, ", "))
	}
	if len(m.Tasks) > 0 {
		var ts []string
		for _, t := range m.Tasks {
			ts = append(ts, t.Command)
		}
		fmt.Fprintf(out, "tasks: %s\n", strings.Join(ts, ", "))
	}
	if len(m.Docs) > 0 {
		fmt.Fprintf(out, "unpublished docs: %d Markdown files\n", len(m.Docs))
	}
	var hot []string
	for _, h := range m.HotFiles {
		if len(hot) == 5 {
			break
		}
		hot = append(hot, fmt.Sprintf("%s (%d)", h.Path, h.Count))
	}
	if len(hot) > 0 {
		fmt.Fprintf(out, "most changed (90 days): %s\n", strings.Join(hot, ", "))
	}
}
