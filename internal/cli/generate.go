package cli

import (
	"fmt"
	"strings"

	"github.com/byabasaija/indoc/internal/config"
	"github.com/spf13/cobra"
)

var (
	flagSources  []string
	flagOut      string
	flagProvider string
	flagModel    string
)

// generateCmd represents the 'generate' command
var generateCmd = &cobra.Command{
	Use:   "generate",
	Short: "Scan codebase and compile standalone documentation bundle",
	Long: `Inspects the specified source directories, uses the configured LLM provider
to generate structured documentation, and bundles a standalone Docusaurus-styled
HTML/CSS/JS viewer into the output directory.`,
	RunE: runGenerate,
}

func init() {
	generateCmd.Flags().StringSliceVarP(&flagSources, "source", "s", nil, "source directories to inspect (comma-separated or repeated)")
	generateCmd.Flags().StringVarP(&flagOut, "out", "o", "", "output directory for compiled documentation (default: ./static/docs)")
	generateCmd.Flags().StringVarP(&flagProvider, "provider", "p", "", "LLM provider: openai, anthropic, ollama (default: openai)")
	generateCmd.Flags().StringVarP(&flagModel, "model", "m", "", "model name (default: gpt-4o-mini)")

	rootCmd.AddCommand(generateCmd)
}

func runGenerate(cmd *cobra.Command, args []string) error {
	// 1. Load configuration file (or defaults if file not present)
	cfg, found, err := config.Load(cfgFile)
	if err != nil {
		return fmt.Errorf("configuration error: %w", err)
	}

	if verbose {
		if found {
			fmt.Printf("Loaded configuration from %s\n", cfgFile)
		} else {
			fmt.Println("No configuration file found; using default settings and CLI flags.")
		}
	}

	// 2. CLI flags override config file settings
	if len(flagSources) > 0 {
		cfg.SourceDirs = flagSources
	}
	if flagOut != "" {
		cfg.OutputDir = flagOut
	}
	if flagProvider != "" {
		cfg.Provider = strings.ToLower(flagProvider)
	}
	if flagModel != "" {
		cfg.Model = flagModel
	}

	// 3. Validate configuration
	if len(cfg.SourceDirs) == 0 {
		return fmt.Errorf("at least one source directory must be specified")
	}
	if cfg.OutputDir == "" {
		return fmt.Errorf("output directory cannot be empty")
	}

	fmt.Printf("🚀 Starting indoc documentation generation...\n")
	fmt.Printf("   Project:   %s\n", cfg.ProjectName)
	fmt.Printf("   Sources:   %s\n", strings.Join(cfg.SourceDirs, ", "))
	fmt.Printf("   Output:    %s\n", cfg.OutputDir)
	fmt.Printf("   Provider:  %s (%s)\n", cfg.Provider, cfg.Model)

	// Note: Later phases hook the scanner, LLM synthesis, and bundler here!
	return nil
}

