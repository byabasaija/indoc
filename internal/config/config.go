// Package config reads and writes .indoc/config.yaml.
package config

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
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
	// SourceURL is the template for links to the code, e.g.
	// https://github.com/org/repo/blob/HEAD/{path}#L{line}
	SourceURL string `yaml:"source_url,omitempty"`
	// SourceLinks publishes links to the code: citations, reference entries,
	// "Edit this page", and the repository in the header. Off by default:
	// in a private repository those links are 404s for most readers.
	SourceLinks bool `yaml:"source_links,omitempty"`
	// Citations is how citations appear in pages: short (the symbol name,
	// default), full (path::Symbol), or hidden. Pages can override it.
	Citations string `yaml:"citations,omitempty"`
	// Links appear in the site header, e.g. the repository or a website.
	Links []Link `yaml:"links,omitempty"`
	// Footer is a line of Markdown shown at the bottom of every page.
	Footer string `yaml:"footer,omitempty"`
	// SiteURL is where the site is published; enables sitemap.xml and
	// canonical links.
	SiteURL string `yaml:"site_url,omitempty"`
	// ShowAuthors adds the last author's name from git to "Last updated".
	// Off by default: it publishes contributors' names.
	ShowAuthors bool `yaml:"show_authors,omitempty"`
	// AllowHTML passes raw HTML in pages through. Off by default.
	AllowHTML bool `yaml:"allow_html,omitempty"`
	// URLs is how pages link to each other: pretty (guides/setup/, the
	// default) needs a server that serves a folder's index.html; files
	// (guides/setup/index.html) works on any server and from disk.
	URLs string `yaml:"urls,omitempty"`
	// Publish records where the site is served; see `indoc publish`.
	Publish *Publish `yaml:"publish,omitempty"`
}

// Publish is where the docs live.
type Publish struct {
	// Target: pages (GitHub Pages), host (any static host), app (served by
	// the project's own web app), or none.
	Target string `yaml:"target"`
	// Path is the URL path in the app, such as /guide/.
	Path string `yaml:"path,omitempty"`
	// Access in the app: public, or login (only signed-in users; staff
	// users in Django).
	Access string `yaml:"access,omitempty"`
}

// PublishTargets and PublishAccess are the valid values.
var (
	PublishTargets = []string{"pages", "host", "app", "none"}
	PublishAccess  = []string{"public", "login"}
	URLStyles      = []string{"pretty", "files"}
)

// FileURLs reports whether pages link as guides/setup/index.html.
func (c *Config) FileURLs() bool { return c.URLs == "files" }

// Link is a header link.
type Link struct {
	Text string `yaml:"text"`
	URL  string `yaml:"url"`
}

// CitationStyles are the values of citations; the first is the default.
var CitationStyles = []string{"short", "full", "hidden"}

// CitationStyle returns the configured citations style, or the default.
func (c *Config) CitationStyle() string {
	if c.Citations == "" {
		return CitationStyles[0]
	}
	return c.Citations
}

// PublicLink is SourceLink when source_links is on, and "" otherwise.
func (c *Config) PublicLink(path string, line int) string {
	if !c.SourceLinks {
		return ""
	}
	return c.SourceLink(path, line)
}

// SourceLink fills the source_url template, or returns "" when unset.
func (c *Config) SourceLink(path string, line int) string {
	if c.SourceURL == "" || path == "" {
		return ""
	}
	u := strings.ReplaceAll(c.SourceURL, "{path}", path)
	if line > 0 {
		return strings.ReplaceAll(u, "{line}", strconv.Itoa(line))
	}
	// No line: drop a trailing line anchor such as #L{line} or #lines-{line}.
	if i := strings.LastIndex(u, "#"); i >= 0 && strings.Contains(u[i:], "{line}") {
		return u[:i]
	}
	return strings.ReplaceAll(u, "{line}", "1")
}

var remoteURL = regexp.MustCompile(`^\s*url\s*=\s*(\S+)`)

// InferSourceURL builds a source_url template from the origin remote in
// .git/config, for GitHub, GitLab, and Bitbucket. It returns "" otherwise.
func InferSourceURL(root string) string {
	f, err := os.Open(filepath.Join(root, ".git", "config"))
	if err != nil {
		return ""
	}
	defer f.Close()
	inOrigin := false
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "[") {
			inOrigin = line == `[remote "origin"]`
			continue
		}
		if m := remoteURL.FindStringSubmatch(line); inOrigin && m != nil {
			return sourceTemplate(m[1])
		}
	}
	return ""
}

func sourceTemplate(remote string) string {
	r := strings.TrimSuffix(strings.TrimSuffix(remote, "/"), ".git")
	var host, repoPath string
	switch {
	case strings.HasPrefix(r, "git@"):
		host, repoPath, _ = strings.Cut(strings.TrimPrefix(r, "git@"), ":")
	case strings.HasPrefix(r, "https://"), strings.HasPrefix(r, "http://"), strings.HasPrefix(r, "ssh://"):
		rest := r[strings.Index(r, "://")+3:]
		if at := strings.LastIndex(rest, "@"); at >= 0 {
			rest = rest[at+1:]
		}
		host, repoPath, _ = strings.Cut(rest, "/")
		host, _, _ = strings.Cut(host, ":")
	default:
		return ""
	}
	if host == "" || strings.Count(repoPath, "/") < 1 {
		return ""
	}
	switch host {
	case "github.com":
		return "https://github.com/" + repoPath + "/blob/HEAD/{path}#L{line}"
	case "gitlab.com":
		return "https://gitlab.com/" + repoPath + "/-/blob/HEAD/{path}#L{line}"
	case "bitbucket.org":
		return "https://bitbucket.org/" + repoPath + "/src/HEAD/{path}#lines-{line}"
	}
	return ""
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
	if err := cfg.validatePublishing(); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if cfg.Citations != "" && !contains(CitationStyles, cfg.Citations) {
		return nil, fmt.Errorf("%s: citations must be one of %s, got %q", path, strings.Join(CitationStyles, ", "), cfg.Citations)
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

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// Validate checks the settings that have a fixed set of values.
func (c *Config) Validate() error {
	if err := c.validatePublishing(); err != nil {
		return err
	}
	if c.Citations != "" && !contains(CitationStyles, c.Citations) {
		return fmt.Errorf("citations must be one of %s, got %q", strings.Join(CitationStyles, ", "), c.Citations)
	}
	return nil
}

func (c *Config) validatePublishing() error {
	if c.URLs != "" && !contains(URLStyles, c.URLs) {
		return fmt.Errorf("urls must be one of %s, got %q", strings.Join(URLStyles, ", "), c.URLs)
	}
	p := c.Publish
	if p == nil {
		return nil
	}
	if !contains(PublishTargets, p.Target) {
		return fmt.Errorf("publish.target must be one of %s, got %q", strings.Join(PublishTargets, ", "), p.Target)
	}
	if p.Access != "" && !contains(PublishAccess, p.Access) {
		return fmt.Errorf("publish.access must be one of %s, got %q", strings.Join(PublishAccess, ", "), p.Access)
	}
	if p.Target == "app" && !strings.HasPrefix(p.Path, "/") {
		return fmt.Errorf("publish.path must start with / (for example /guide/), got %q", p.Path)
	}
	return nil
}
