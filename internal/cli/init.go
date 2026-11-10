package cli

import (
	"fmt"
	"os"

	"github.com/byabasaija/indoc/internal/config"
	"github.com/spf13/cobra"
)

var forceInit bool

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Write a .indoc.yaml configuration file in the current directory",
	RunE:  runInit,
}

func init() {
	initCmd.Flags().BoolVarP(&forceInit, "force", "f", false, "overwrite existing .indoc.yaml if it exists")
	rootCmd.AddCommand(initCmd)
}

func runInit(cmd *cobra.Command, args []string) error {
	targetFile := config.DefaultConfigFileName
	if _, err := os.Stat(targetFile); err == nil && !forceInit {
		return fmt.Errorf("configuration file '%s' already exists. Use --force to overwrite", targetFile)
	}

	cfg := config.Default()
	if err := cfg.Save(targetFile); err != nil {
		return fmt.Errorf("failed to create %s: %w", targetFile, err)
	}

	fmt.Printf("Created %s\n", targetFile)
	fmt.Println("Next:")
	fmt.Println("  1. Set project_name, source_dirs, and output_dir.")
	fmt.Println("  2. Ask your editor agent to document this project.")
	fmt.Println("  3. Or run: indoc scan --json")

	return nil
}
