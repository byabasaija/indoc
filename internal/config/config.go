package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// DefaultConfigFileName is the standard configuration file for indoc.
const DefaultConfigFileName = ".indoc.yaml"

// Config represents the top-level configuration options for indoc.
type Config struct {
	Version     string   `yaml:"version" json:"version"`
	ProjectName string   `yaml:"project_name" json:"project_name"`
	SourceDirs  []string `yaml:"source_dirs" json:"source_dirs"`
	OutputDir   string   `yaml:"output_dir" json:"output_dir"`
	AssetsDir   string   `yaml:"assets_dir,omitempty" json:"assets_dir,omitempty"`
	Provider    string   `yaml:"provider" json:"provider"` // e.g. "openai", "anthropic", "ollama"
	Model       string   `yaml:"model" json:"model"`       // e.g. "gpt-4o-mini", "claude-3-5-sonnet-20241022"
	Exclude     []string `yaml:"exclude,omitempty" json:"exclude,omitempty"`
}

// Default returns a Config struct populated with sensible default settings.
func Default() *Config {
	return &Config{
		Version:     "1",
		ProjectName: "API Documentation",
		SourceDirs:  []string{"."},
		OutputDir:   "./static/docs",
		AssetsDir:   "",
		Provider:    "openai",
		Model:       "gpt-4o-mini",
		Exclude: []string{
			"node_modules",
			"vendor",
			".git",
			".indoc",
			"dist",
			"build",
		},
	}
}

// Load attempts to read and parse a configuration file from the specified path.
// If configPath is empty, it looks for `.indoc.yaml` in the current working directory.
// If the file does not exist, it returns the default configuration and false (fileNotFound).
func Load(configPath string) (*Config, bool, error) {
	if configPath == "" {
		configPath = DefaultConfigFileName
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return Default(), false, nil
		}
		return nil, false, fmt.Errorf("failed to read configuration file: %w", err)
	}

	cfg := Default()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, true, fmt.Errorf("failed to parse YAML configuration: %w", err)
	}

	return cfg, true, nil
}

// Save writes a Config struct as formatted YAML to the specified path.
func (c *Config) Save(filePath string) error {
	dir := filepath.Dir(filePath)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory for config: %w", err)
		}
	}

	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("failed to marshal configuration to YAML: %w", err)
	}

	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write configuration file: %w", err)
	}

	return nil
}

