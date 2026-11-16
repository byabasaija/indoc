package cli

import (
	"embed"
	"encoding/json"
	"fmt"
	"os"

	"github.com/byabasaija/indoc/internal/config"
	"github.com/byabasaija/indoc/internal/doc"
	"github.com/spf13/cobra"
)

//go:embed templates/AGENT_GUIDE.md templates/site.schema.json
var initTemplates embed.FS

var forceInit bool

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize indoc configuration and agent scaffolding",
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

	template := doc.Document{
		ProjectName: cfg.ProjectName,
		Overview:    doc.Page{Title: "Overview"},
		Guides:      []doc.Page{},
		Reference:   []doc.RefSection{},
	}
	site, err := json.MarshalIndent(template, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to create document template: %w", err)
	}
	site = append(site, '\n')

	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("failed to create %s: %w", configDir, err)
	}
	guide, err := initTemplates.ReadFile("templates/AGENT_GUIDE.md")
	if err != nil {
		return fmt.Errorf("failed to read agent guide template: %w", err)
	}
	schema, err := initTemplates.ReadFile("templates/site.schema.json")
	if err != nil {
		return fmt.Errorf("failed to read document schema template: %w", err)
	}
	files := []struct {
		path    string
		content []byte
	}{
		{path: siteFile, content: site},
		{path: agentGuideFile, content: guide},
		{path: schemaFile, content: schema},
	}
	for _, file := range files {
		if err := writeScaffoldFile(file.path, file.content); err != nil {
			return err
		}
	}

	fmt.Printf("Wrote %s\n", targetFile)
	for _, path := range []string{siteFile, agentGuideFile, schemaFile} {
		fmt.Printf("Prepared %s\n", path)
	}
	fmt.Println("Next:")
	fmt.Println("  1. Set project_name, source_dirs, and output_dir in .indoc.yaml.")
	fmt.Printf("  2. Ask your coding agent to read %s and follow it.\n", agentGuideFile)
	fmt.Printf("  3. Bundle the result with: indoc bundle --input %s\n", siteFile)

	return nil
}

const (
	configDir      = ".indoc"
	siteFile       = ".indoc/site.json"
	agentGuideFile = ".indoc/AGENT_GUIDE.md"
	schemaFile     = ".indoc/site.schema.json"
)

func writeScaffoldFile(path string, content []byte) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		if os.IsExist(err) {
			return nil
		}
		return fmt.Errorf("failed to create %s: %w", path, err)
	}
	if _, err := file.Write(content); err != nil {
		file.Close()
		os.Remove(path)
		return fmt.Errorf("failed to write %s: %w", path, err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("failed to close %s: %w", path, err)
	}
	return nil
}
