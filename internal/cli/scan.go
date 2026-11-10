package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/byabasaija/indoc/internal/config"
	"github.com/byabasaija/indoc/internal/doc"
	"github.com/byabasaija/indoc/internal/reference"
	"github.com/byabasaija/indoc/internal/scanner"
	"github.com/spf13/cobra"
)

var (
	scanSources []string
	scanJSON    bool
)

var scanCmd = &cobra.Command{
	Use:   "scan",
	Short: "Hash sources, extract routes and config, and return the previous document",
	RunE: func(cmd *cobra.Command, args []string) error {
		return executeScan(scanOptions{
			configPath: cfgFile,
			cachePath:  resolvedCachePath(),
			sources:    scanSources,
			asJSON:     scanJSON,
			out:        cmd.OutOrStdout(),
		})
	},
}

func init() {
	scanCmd.Flags().StringSliceVarP(&scanSources, "source", "s", nil, "source directories (repeat or comma-separate)")
	scanCmd.Flags().BoolVar(&scanJSON, "json", false, "print the scan report as JSON")
	rootCmd.AddCommand(scanCmd)
}

type scanOptions struct {
	configPath string
	cachePath  string
	sources    []string
	asJSON     bool
	out        io.Writer
}

type scanReport struct {
	ProjectName string             `json:"project_name"`
	OutputDir   string             `json:"output_dir"`
	Added       []scanner.FileMeta `json:"added"`
	Modified    []scanner.FileMeta `json:"modified"`
	Unchanged   int                `json:"unchanged"`
	Deleted     []string           `json:"deleted"`
	Reference   []doc.RefSection   `json:"reference"`
	Stale       []doc.StaleRef     `json:"stale_reference"`
	Previous    *doc.Document      `json:"previous"`
}

func executeScan(opts scanOptions) error {
	cfg, _, err := config.Load(opts.configPath)
	if err != nil {
		return err
	}
	if len(opts.sources) > 0 {
		cfg.SourceDirs = opts.sources
	}
	if len(cfg.SourceDirs) == 0 {
		return fmt.Errorf("at least one source directory is required")
	}
	if opts.out == nil {
		opts.out = io.Discard
	}

	sc := scanner.NewScanner(cfg.SourceDirs, cfg.Exclude, cfg.IncludeTests, opts.cachePath)
	result, err := sc.Scan()
	if err != nil {
		return err
	}

	manifest, err := scanner.LoadCache(opts.cachePath)
	if err != nil {
		return err
	}
	previous := manifest.Document
	manifest.FileCache = result.Files
	if err := scanner.SaveCache(opts.cachePath, manifest); err != nil {
		return err
	}

	files := make([]reference.SourceFile, 0, len(result.Files))
	for _, meta := range result.Files {
		files = append(files, reference.SourceFile{Path: meta.Path, Language: meta.Language})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })

	sections, err := reference.Extract(files)
	if err != nil {
		return err
	}

	report := scanReport{
		ProjectName: cfg.ProjectName,
		OutputDir:   cfg.OutputDir,
		Added:       sortedFiles(result.Diff.Added),
		Modified:    sortedFiles(result.Diff.Modified),
		Unchanged:   len(result.Diff.Unchanged),
		Deleted:     sortedStrings(result.Diff.Deleted),
		Reference:   sections,
		Stale:       doc.StaleReference(previous, pathsOf(result.Diff.Modified), result.Diff.Deleted),
		Previous:    previous,
	}
	if opts.asJSON {
		enc := json.NewEncoder(opts.out)
		enc.SetIndent("", "  ")
		enc.SetEscapeHTML(false)
		return enc.Encode(report)
	}
	writeScanText(opts.out, report)
	return nil
}

func sortedFiles(files []scanner.FileMeta) []scanner.FileMeta {
	if files == nil {
		files = []scanner.FileMeta{}
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files
}

func sortedStrings(values []string) []string {
	if values == nil {
		values = []string{}
	}
	sort.Strings(values)
	return values
}

func pathsOf(files []scanner.FileMeta) []string {
	paths := make([]string, len(files))
	for i, file := range files {
		paths[i] = file.Path
	}
	return paths
}

func writeScanText(out io.Writer, report scanReport) {
	fmt.Fprintf(out, "%s\noutput: %s\n\n", report.ProjectName, report.OutputDir)
	fmt.Fprintf(out, "files: %d added, %d modified, %d unchanged, %d deleted\n",
		len(report.Added), len(report.Modified), report.Unchanged, len(report.Deleted))
	for _, file := range report.Added {
		fmt.Fprintf(out, "  + %s\n", file.Path)
	}
	for _, file := range report.Modified {
		fmt.Fprintf(out, "  * %s\n", file.Path)
	}
	for _, path := range report.Deleted {
		fmt.Fprintf(out, "  - %s\n", path)
	}

	fmt.Fprintf(out, "\nreference: %s\n", summarizeReference(report.Reference))
	if report.Previous == nil {
		fmt.Fprintf(out, "previous document: none\n")
	} else {
		fmt.Fprintf(out, "previous document: overview, %d guides\n", len(report.Previous.Guides))
	}
	fmt.Fprintf(out, "stale reference: %d\n", len(report.Stale))
	for _, entry := range report.Stale {
		fmt.Fprintf(out, "  %s %s (%s) %s\n", entry.Detail, entry.Name, entry.Source, entry.Reason)
	}
}

func summarizeReference(sections []doc.RefSection) string {
	if len(sections) == 0 {
		return "none"
	}
	parts := make([]string, len(sections))
	for i, section := range sections {
		parts[i] = fmt.Sprintf("%d %s", len(section.Entries), section.Kind)
	}
	return strings.Join(parts, ", ")
}
