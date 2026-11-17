package cli

import (
	"github.com/byabasaija/indoc/internal/config"
	"github.com/spf13/cobra"
)

var (
	cfgFile string

	rootCmd = &cobra.Command{
		Use:   "indoc",
		Short: "Documentation that fails CI when it stops matching the code",
		Long: `indoc keeps Markdown documentation tied to the code it describes.

Pages cite code as path::Symbol and list the files they cover. indoc records
what each page was verified against in .indoc/lock.json, and ` + "`indoc check`" + `
fails when that code changes and the page does not. ` + "`indoc bundle`" + ` builds
a static site with reference facts (routes, environment variables) extracted
from source.

indoc never calls an AI model. Your coding agent writes the pages.`,
		SilenceUsage: true,
	}
)

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default "+config.DefaultPath+")")
}
