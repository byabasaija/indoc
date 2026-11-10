package cli

import (
	"fmt"

	"github.com/byabasaija/indoc/internal/version"
	"github.com/spf13/cobra"
)

// versionCmd represents the 'version' command
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version and build information of indoc",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println(version.Info())
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
