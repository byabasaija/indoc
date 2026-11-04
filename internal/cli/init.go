package cli

import (
	"fmt"
	"os"

	"github.com/byabasaija/indoc/internal/config"
	"github.com/spf13/cobra"
)

var forceInit bool

// initCmd represents the 'init' command
var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize a new .indoc.yaml configuration file in the current directory",
	Long: `Creates a default .indoc.yaml configuration file pre-populated with sensible defaults.
You can customize the project name, source directories, LLM provider, and output folder.`,
	RunE: runInit,
}

func init() {
	initCmd.Flags().BoolVarP(&forceInit, "force", "f", false, "overwrite existing .indoc.yaml if it exists")
	rootCmd.AddCommand(initCmd)
}

func runInit(cmd *cobra.Command, args []string) error {
	targetFile := config.DefaultConfigFileName

	// Check if file already exists
	if _, err := os.Stat(targetFile); err == nil && !forceInit {
		return fmt.Errorf("configuration file '%s' already exists. Use --force to overwrite", targetFile)
	}

	cfg := config.Default()
	if err := cfg.Save(targetFile); err != nil {
		return fmt.Errorf("failed to create %s: %w", targetFile, err)
	}

	fmt.Printf("✨ Created %s with default configuration!\n", targetFile)
	fmt.Println("Next steps:")
	fmt.Println("  1. Edit .indoc.yaml to match your source directories and project name.")
	fmt.Println("  2. Set your API key: export OPENAI_API_KEY=\"sk-...\"")
	fmt.Println("  3. Run: indoc generate")

	return nil
}

