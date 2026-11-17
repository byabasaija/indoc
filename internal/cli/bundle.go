package cli

import (
	"fmt"
	"io"
	"path"
	"path/filepath"
	"strings"

	"github.com/byabasaija/indoc/internal/bundler"
	"github.com/byabasaija/indoc/internal/doc"
	"github.com/byabasaija/indoc/internal/drift"
	"github.com/byabasaija/indoc/internal/pages"
	"github.com/byabasaija/indoc/internal/reference"
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
	document := assemble(p, ref.Sections)
	return bundler.Write(document, outDir, p.paths.Root)
}

// assemble maps pages onto the viewer's document: the top-most index.md or
// README.md is the overview, every other page is a guide in page order.
func assemble(p *project, sections []doc.RefSection) *doc.Document {
	overview := overviewIndex(p.pages)
	d := &doc.Document{
		ProjectName: p.cfg.ProjectName,
		Appearance:  p.cfg.Appearance,
		Reference:   sections,
		Guides:      []doc.Page{},
	}
	if d.ProjectName == "" {
		d.ProjectName = "Documentation"
	}
	for i, pg := range p.pages {
		page := doc.Page{
			ID:       doc.Slug(strings.TrimSuffix(pg.Path, path.Ext(pg.Path))),
			Title:    pg.Title,
			Markdown: pg.Body,
		}
		if i == overview {
			d.Overview = page
			continue
		}
		d.Guides = append(d.Guides, page)
	}
	return d
}

func overviewIndex(ps []pages.Page) int {
	best, bestDepth := 0, -1
	for i, p := range ps {
		base := strings.ToLower(path.Base(p.Path))
		if base != "index.md" && base != "readme.md" {
			continue
		}
		depth := strings.Count(p.Path, "/")
		if bestDepth < 0 || depth < bestDepth {
			best, bestDepth = i, depth
		}
	}
	return best
}
