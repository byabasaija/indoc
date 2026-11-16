package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/byabasaija/indoc/internal/bundler"
	"github.com/byabasaija/indoc/internal/config"
	"github.com/byabasaija/indoc/internal/doc"
	"github.com/byabasaija/indoc/internal/scanner"
	"github.com/spf13/cobra"
)

var (
	bundleInput string
	bundleOut   string
)

var bundleCmd = &cobra.Command{
	Use:   "bundle",
	Short: "Compile a document JSON file into a static site",
	RunE: func(cmd *cobra.Command, args []string) error {
		return executeBundle(bundleOptions{
			input:      bundleInput,
			outputDir:  bundleOut,
			configPath: cfgFile,
			cachePath:  resolvedCachePath(),
			out:        cmd.OutOrStdout(),
		})
	},
}

func init() {
	bundleCmd.Flags().StringVarP(&bundleInput, "input", "i", "", "path to the document JSON file")
	bundleCmd.Flags().StringVarP(&bundleOut, "out", "o", "", "output directory (default from .indoc.yaml or ./static/docs)")
	rootCmd.AddCommand(bundleCmd)
}

type bundleOptions struct {
	input      string
	outputDir  string
	configPath string
	cachePath  string
	out        io.Writer
}

func executeBundle(opts bundleOptions) error {
	if strings.TrimSpace(opts.input) == "" {
		return fmt.Errorf("--input is required")
	}
	data, err := os.ReadFile(opts.input)
	if err != nil {
		return fmt.Errorf("failed to read %s: %w", opts.input, err)
	}
	var document doc.Document
	if err := json.Unmarshal(data, &document); err != nil {
		return fmt.Errorf("failed to parse %s: %w", opts.input, err)
	}

	outputDir := opts.outputDir
	if outputDir == "" {
		cfg, _, err := config.Load(opts.configPath)
		if err != nil {
			return err
		}
		outputDir = cfg.OutputDir
	}
	if outputDir == "" {
		return fmt.Errorf("output directory is required")
	}
	if err := bundler.Write(&document, outputDir, filepath.Dir(opts.input)); err != nil {
		return err
	}

	manifest, err := scanner.LoadCache(opts.cachePath)
	if err != nil {
		return err
	}
	manifest.Document = &document
	if err := scanner.SaveCache(opts.cachePath, manifest); err != nil {
		return err
	}

	if opts.out == nil {
		opts.out = io.Discard
	}
	fmt.Fprintf(opts.out, "Wrote %s\nOptional local preview: indoc serve --dir %s\n", outputDir, outputDir)
	return nil
}
