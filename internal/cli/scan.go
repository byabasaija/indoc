package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/byabasaija/indoc/internal/code"
	"github.com/byabasaija/indoc/internal/doc"
	"github.com/byabasaija/indoc/internal/drift"
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
	// Files is the number of repository files; Parsed of those have a
	// language parser and support path::Symbol citations.
	Files  int `json:"files"`
	Parsed int `json:"parsed"`
}

func runScan(configPath string, out io.Writer, asJSON bool) error {
	p, err := openProject(configPath)
	if err != nil {
		return err
	}
	sections, err := reference.Extract(p.paths.Root, reference.Sources(p.repo.Files))
	if err != nil {
		return err
	}
	report := scanReport{
		ProjectName: p.cfg.ProjectName,
		OutputDir:   p.cfg.OutputDir,
		Findings:    nonNil(drift.Check(p.repo, p.pages, p.lock)),
		Reference:   sections,
		Files:       len(p.repo.Files),
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
	fmt.Fprintf(out, "%s: %d pages, %d files (%d parsed)\n\n", report.ProjectName, len(report.Pages), report.Files, report.Parsed)
	for _, sp := range report.Pages {
		fmt.Fprintf(out, "  %-40s %d sources, %d citations\n", sp.Path, len(sp.Sources), len(sp.Citations))
	}
	fmt.Fprintf(out, "\nreference: %s\n\n", summarizeReference(sections))
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
