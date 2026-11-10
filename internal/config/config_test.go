package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := Default()
	if cfg.Version != "1" {
		t.Errorf("expected version '1', got '%s'", cfg.Version)
	}
	if cfg.OutputDir != "./static/docs" {
		t.Errorf("expected output_dir './static/docs', got '%s'", cfg.OutputDir)
	}
	if len(cfg.SourceDirs) != 1 || cfg.SourceDirs[0] != "." {
		t.Errorf("expected default source '.', got %#v", cfg.SourceDirs)
	}
}

func TestSaveAndLoadRoundtrip(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, ".indoc.yaml")

	original := &Config{
		Version:     "1",
		ProjectName: "Hospital Core API",
		SourceDirs:  []string{"./backend/controllers", "./backend/models"},
		OutputDir:   "./custom/docs",
		Exclude:     []string{"node_modules"},
	}

	if err := original.Save(configPath); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	loaded, found, err := Load(configPath)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}
	if !found {
		t.Fatal("expected config file to be found")
	}

	if loaded.ProjectName != original.ProjectName {
		t.Errorf("expected ProjectName '%s', got '%s'", original.ProjectName, loaded.ProjectName)
	}
	if loaded.OutputDir != original.OutputDir {
		t.Errorf("expected OutputDir '%s', got '%s'", original.OutputDir, loaded.OutputDir)
	}
	if len(loaded.SourceDirs) != 2 || loaded.SourceDirs[0] != original.SourceDirs[0] {
		t.Errorf("expected source dirs %#v, got %#v", original.SourceDirs, loaded.SourceDirs)
	}
}

func TestLoadNonExistent(t *testing.T) {
	cfg, found, err := Load("/path/to/non/existent/config.yaml")
	if err != nil {
		t.Fatalf("expected no error for missing config, got %v", err)
	}
	if found {
		t.Error("expected found to be false for missing config file")
	}
	if cfg == nil {
		t.Fatal("expected default config to be returned")
	}
	if cfg.OutputDir != "./static/docs" {
		t.Errorf("expected default output dir, got %s", cfg.OutputDir)
	}
}

func TestLoadInvalidYAML(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "invalid.yaml")

	if err := os.WriteFile(configPath, []byte("invalid: [unclosed yaml"), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	_, found, err := Load(configPath)
	if err == nil {
		t.Fatal("expected error parsing invalid YAML, got nil")
	}
	if !found {
		t.Error("expected found to be true since file exists")
	}
}
