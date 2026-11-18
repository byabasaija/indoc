package cli

import (
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/byabasaija/indoc/internal/code"
	"github.com/byabasaija/indoc/internal/config"
	"github.com/byabasaija/indoc/internal/glob"
	"github.com/spf13/cobra"
)

var (
	initForce  bool
	initPages  []string
	initOutput string
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Create .indoc/config.yaml",
	Long: `init creates .indoc/config.yaml, inferring the project name and where pages
live. Publishing is opt-in: only Markdown files matched by ` + "`pages:`" + ` are
built into the site.

When the repository already has Markdown docs, init lists them instead of
publishing them all. Choose which to include with --pages, or edit the config.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runInit(cfgFile, cmd.OutOrStdout(), initPages, initOutput, initForce)
	},
}

func init() {
	initCmd.Flags().BoolVarP(&initForce, "force", "f", false, "overwrite an existing .indoc/config.yaml")
	initCmd.Flags().StringSliceVar(&initPages, "pages", nil, "page globs to publish (repeat or comma-separate)")
	initCmd.Flags().StringVar(&initOutput, "output", "", "build output directory (default "+config.DefaultOutputDir+")")
	rootCmd.AddCommand(initCmd)
}

var docusaurusConfigs = []string{"docusaurus.config.js", "docusaurus.config.ts", "docusaurus.config.mjs"}

func runInit(configPath string, out io.Writer, pagesFlag []string, output string, force bool) error {
	paths := config.Resolve(configPath)
	if _, err := os.Stat(paths.Config); err == nil && !force {
		return fmt.Errorf("%s already exists; use --force to replace it", paths.Config)
	}

	cfg := config.Default()
	cfg.ProjectName = config.InferProjectName(paths.Root)
	cfg.SourceURL = config.InferSourceURL(paths.Root)
	if output != "" {
		cfg.OutputDir = output
	}

	files, err := code.ListFiles(paths.Root, nil)
	if err != nil {
		return err
	}
	var markdown, inPagesDir []string
	for _, f := range files {
		if !strings.EqualFold(path.Ext(f), ".md") {
			continue
		}
		markdown = append(markdown, f)
		if strings.HasPrefix(f, PagesDir+"/") {
			inPagesDir = append(inPagesDir, f)
		}
	}

	reason := ""
	switch {
	case len(pagesFlag) > 0:
		cfg.Pages = pagesFlag
	case hasAny(paths.Root, docusaurusConfigs):
		cfg.Pages = []string{"docs/**/*.md"}
		reason = "Docusaurus project: using its docs/ folder."
	case len(inPagesDir) == 0:
		// Not docs/: that is where plans, reviews, and agent notes collect.
		cfg.Pages = []string{PagesDir + "/**/*.md"}
		reason = "New pages go in " + PagesDir + "/. Markdown elsewhere (docs/, notes, plans) is not published."
	default:
		cfg.Pages = []string{}
		reason = PagesDir + "/ already has Markdown. Nothing is published until you choose."
	}

	if err := cfg.Save(paths.Config); err != nil {
		return err
	}
	ignore := filepath.Join(filepath.Dir(paths.Config), ".gitignore")
	if _, err := os.Stat(ignore); os.IsNotExist(err) {
		if err := os.WriteFile(ignore, []byte("# Build output and local state. lock.json and config.yaml are committed.\nsite/\n"), 0o644); err != nil {
			return err
		}
	}

	fmt.Fprintf(out, "Wrote %s (project: %s)\n", paths.Config, cfg.ProjectName)
	if reason != "" {
		fmt.Fprintln(out, reason)
	}
	if len(cfg.Pages) > 0 {
		fmt.Fprintf(out, "pages: %s\n", strings.Join(cfg.Pages, ", "))
	}
	if cfg.SourceURL != "" {
		fmt.Fprintf(out, "source_url: %s\nThe site does not link to the code unless you set source_links: true (do it if your readers can open the repository).\n", cfg.SourceURL)
	}
	var unpublished []string
	for _, f := range markdown {
		if !matchesAny(cfg.Pages, f) {
			unpublished = append(unpublished, f)
		}
	}
	if len(unpublished) > 0 {
		fmt.Fprintln(out, "\nExisting Markdown files (not published):")
		for i, f := range unpublished {
			if i == 30 {
				fmt.Fprintf(out, "  ... and %d more\n", len(unpublished)-30)
				break
			}
			fmt.Fprintf(out, "  %s\n", f)
		}
		if len(cfg.Pages) == 0 {
			fmt.Fprintln(out, "\nChoose which to publish, for example:\n  indoc init --force --pages '"+PagesDir+"/**/*.md,docs/partner-integration.md'")
		} else {
			fmt.Fprintln(out, "\nTo publish any of them too, add them under `pages:`.")
		}
	}
	if _, err := os.Stat(filepath.Join(paths.Root, config.LegacyPath)); err == nil {
		fmt.Fprintf(out, "\n%s is the old config format and is no longer read; you can delete it.\n", config.LegacyPath)
	}
	fmt.Fprintf(out, "\nBuild output: %s (gitignored). Commit %s and, once created, .indoc/lock.json.\n", cfg.OutputDir, paths.Config)
	return nil
}

func matchesAny(patterns []string, name string) bool {
	for _, p := range patterns {
		if glob.Match(p, name) {
			return true
		}
	}
	return false
}

// PagesDir is where new pages go when the project has no Docusaurus site.
// It is deliberately not docs/, which in most repositories also holds plans,
// reviews, and agent notes that must never be published.
const PagesDir = "documentation"

func hasAny(root string, names []string) bool {
	for _, n := range names {
		if _, err := os.Stat(filepath.Join(root, n)); err == nil {
			return true
		}
	}
	return false
}
