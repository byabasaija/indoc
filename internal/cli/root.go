package cli

import (
	"strings"

	"github.com/byabasaija/indoc/internal/scanner"
	"github.com/spf13/cobra"
)

var (
	cfgFile   string
	cacheFile string

	rootCmd = &cobra.Command{
		Use:   "indoc",
		Short: "Compile a static documentation site from agent-written pages and extracted code facts",
		Long: `indoc scans a project, extracts routes and environment variables, and compiles
overview pages, guides, and that reference into a static site the application can mount.

The host agent writes the prose. indoc does not call a model and does not need an API key.`,
	}
)

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file path (default is .indoc.yaml)")
	rootCmd.PersistentFlags().StringVar(&cacheFile, "cache", "", "cache manifest path (default is .indoc/cache.json)")
}

func resolvedCachePath() string {
	if strings.TrimSpace(cacheFile) == "" {
		return scanner.DefaultCacheFile
	}
	return cacheFile
}
