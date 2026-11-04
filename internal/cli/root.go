package cli

import (
	"github.com/spf13/cobra"
)

var (
	// Global flag variables
	cfgFile string
	verbose bool

	// rootCmd represents the base command when called without any subcommands.
	rootCmd = &cobra.Command{
		Use:   "indoc",
		Short: "indoc is an automated, zero-server documentation compiler for any codebase",
		Long: `indoc is a single-binary CLI tool that automatically inspects your codebase,
uses an LLM (Bring Your Own API Key), and compiles a standalone, Docusaurus-styled
documentation bundle directly into your project's static folder with zero external servers.

Like FastAPI generates Swagger UI at startup, indoc gives your entire codebase
beautiful, searchable, offline-ready documentation.`,
	}
)

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main().
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	// Persistent flags are available to this command and all subcommands below it.
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file path (default is .indoc.yaml)")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "enable verbose output and debug logs")
}
