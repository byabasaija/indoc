// Package config reads and writes .indoc/config.yaml.
package config

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/byabasaija/indoc/internal/doc"
	"gopkg.in/yaml.v3"
)

const (
	// Dir holds every file indoc owns.
	Dir = ".indoc"
	// DefaultPath is the configuration file, relative to the repository root.
	DefaultPath = ".indoc/config.yaml"
	// LockFile records what each page was verified against. Committed.
	LockFile = "lock.json"
	// DefaultOutputDir is the default build output, gitignored.
	DefaultOutputDir = ".indoc/site"
	// LegacyPath is the pre-roadmap configuration file.
	LegacyPath = ".indoc.yaml"
)

// Config is the contents of .indoc/config.yaml.
type Config struct {
	Version     string          `yaml:"version"`
	ProjectName string          `yaml:"project_name"`
	Pages       []string        `yaml:"pages"`
	OutputDir   string          `yaml:"output_dir"`
	Exclude     []string        `yaml:"exclude,omitempty"`
	Appearance  *doc.Appearance `yaml:"appearance,omitempty"`
}

// Default returns a configuration with defaults filled in.
func Default() *Config {
	return &Config{Version: "1", OutputDir: DefaultOutputDir}
}

// Paths locates the files around a configuration file.
type Paths struct {
	Root   string // repository root: the directory containing .indoc/
	Config string
	Lock   string
}

// Resolve returns paths for a configuration file (DefaultPath when empty).
func Resolve(configPath string) Paths {
	if configPath == "" {
		configPath = DefaultPath
	}
	dir := filepath.Dir(configPath)
	return Paths{Root: filepath.Dir(dir), Config: configPath, Lock: filepath.Join(dir, LockFile)}
}

// Load reads a configuration file.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		if _, legacy := os.Stat(LegacyPath); legacy == nil {
			return nil, fmt.Errorf("%s not found; %s is the old format. Run `indoc init` to create the new one", path, LegacyPath)
		}
		return nil, fmt.Errorf("%s not found; run `indoc init` first", path)
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	cfg := Default()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if cfg.OutputDir == "" {
		cfg.OutputDir = DefaultOutputDir
	}
	return cfg, nil
}

// Save writes the configuration.
func (c *Config) Save(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create %s: %w", filepath.Dir(path), err)
	}
	data, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// InferProjectName reads the project name from go.mod, pyproject.toml, or
// package.json in root, falling back to the directory name.
func InferProjectName(root string) string {
	if name := goModuleName(filepath.Join(root, "go.mod")); name != "" {
		return name
	}
	if name := pyprojectName(filepath.Join(root, "pyproject.toml")); name != "" {
		return name
	}
	if name := packageJSONName(filepath.Join(root, "package.json")); name != "" {
		return name
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return "Documentation"
	}
	return filepath.Base(abs)
}

func goModuleName(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		if f := strings.Fields(line); len(f) == 2 && f[0] == "module" {
			mod := strings.Trim(f[1], `"`)
			return mod[strings.LastIndex(mod, "/")+1:]
		}
	}
	return ""
}

var tomlName = regexp.MustCompile(`^\s*name\s*=\s*["']([^"']+)["']`)

// pyprojectName reads name from [project] or [tool.poetry] without a TOML
// dependency; those tables put name on its own line.
func pyprojectName(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	table := ""
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "[") {
			table = line
			continue
		}
		if table == "[project]" || table == "[tool.poetry]" {
			if m := tomlName.FindStringSubmatch(line); m != nil {
				return m[1]
			}
		}
	}
	return ""
}

func packageJSONName(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var pkg struct {
		Name string `json:"name"`
	}
	if json.Unmarshal(data, &pkg) != nil {
		return ""
	}
	name := pkg.Name
	return name[strings.LastIndex(name, "/")+1:]
}
