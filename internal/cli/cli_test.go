package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/byabasaija/indoc/internal/doc"
)

func TestCommandsRegistered(t *testing.T) {
	commands := rootCmd.Commands()
	names := make(map[string]bool)
	for _, cmd := range commands {
		names[cmd.Name()] = true
	}

	expected := []string{"init", "scan", "bundle", "serve", "version"}
	for _, exp := range expected {
		if !names[exp] {
			t.Errorf("expected command '%s' to be registered on rootCmd", exp)
		}
	}
	if names["generate"] {
		t.Error("generate still calls an LLM and should not be registered")
	}
}

func TestVersionCommandOutput(t *testing.T) {
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"version"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("version command failed: %v", err)
	}
}

func TestInitCreatesAgentScaffoldingWithoutOverwritingSite(t *testing.T) {
	dir := t.TempDir()
	oldDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Chdir(oldDir); err != nil {
			t.Errorf("restore working directory: %v", err)
		}
	}()

	if err := runInit(nil, nil); err != nil {
		t.Fatalf("init: %v", err)
	}

	var site doc.Document
	siteData, err := os.ReadFile(siteFile)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(siteData, &site); err != nil {
		t.Fatalf("site template JSON: %v", err)
	}
	if site.ProjectName == "" || site.Overview.Markdown != "" {
		t.Fatalf("site template = %+v", site)
	}

	guide, err := os.ReadFile(agentGuideFile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(guide), "entire `reference` array") {
		t.Fatalf("agent guide does not explain reference handling:\n%s", guide)
	}
	schema, err := os.ReadFile(schemaFile)
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(schema) {
		t.Fatal("document schema is not valid JSON")
	}

	customSite := []byte("{\"custom\":true}\n")
	if err := os.WriteFile(siteFile, customSite, 0644); err != nil {
		t.Fatal(err)
	}
	forceInit = true
	defer func() { forceInit = false }()
	if err := runInit(nil, nil); err != nil {
		t.Fatalf("forced init: %v", err)
	}
	got, err := os.ReadFile(siteFile)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, customSite) {
		t.Fatalf("init overwrote existing site: %s", got)
	}
}

func TestScanThenBundleRemembersDocument(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "app")
	if err := os.Mkdir(source, 0755); err != nil {
		t.Fatal(err)
	}
	sourceFile := filepath.Join(source, "main.py")
	body := "import os\n\n@app.get(\"/health\")\ndef health():\n    return os.getenv(\"DATABASE_URL\")\n"
	if err := os.WriteFile(sourceFile, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}

	configPath := filepath.Join(dir, ".indoc.yaml")
	cachePath := filepath.Join(dir, "cache.json")
	outDir := filepath.Join(dir, "static", "docs")
	cfg := "version: \"1\"\nproject_name: Hospital API\nsource_dirs:\n  - " + source + "\noutput_dir: " + outDir + "\n"
	if err := os.WriteFile(configPath, []byte(cfg), 0644); err != nil {
		t.Fatal(err)
	}

	var scanBuf bytes.Buffer
	if err := executeScan(scanOptions{
		configPath: configPath,
		cachePath:  cachePath,
		asJSON:     true,
		out:        &scanBuf,
	}); err != nil {
		t.Fatalf("scan: %v", err)
	}

	var report scanReport
	if err := json.Unmarshal(scanBuf.Bytes(), &report); err != nil {
		t.Fatalf("scan json: %v\n%s", err, scanBuf.String())
	}
	if report.ProjectName != "Hospital API" || report.Previous != nil {
		t.Fatalf("report = %+v", report)
	}
	if len(report.Reference) != 2 || report.Reference[0].Entries[0].Name != "/health" {
		t.Fatalf("reference = %+v", report.Reference)
	}
	if report.Reference[1].Entries[0].Name != "DATABASE_URL" {
		t.Fatalf("config = %+v", report.Reference[1])
	}

	input := filepath.Join(dir, "site.json")
	document := map[string]any{
		"project_name": "Hospital API",
		"overview": map[string]string{
			"markdown": "Admits patients.\n\n## Flow\n\nRequest, then record.",
		},
		"guides": []map[string]string{{
			"title":    "Run it",
			"markdown": "Start the API.",
		}},
		"reference": report.Reference,
	}
	raw, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(input, raw, 0644); err != nil {
		t.Fatal(err)
	}

	var bundleBuf bytes.Buffer
	if err := executeBundle(bundleOptions{
		input:      input,
		outputDir:  outDir,
		configPath: configPath,
		cachePath:  cachePath,
		out:        &bundleBuf,
	}); err != nil {
		t.Fatalf("bundle: %v", err)
	}
	index, err := os.ReadFile(filepath.Join(outDir, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(index, []byte("Hospital API")) {
		t.Fatalf("index = %s", index)
	}

	if err := os.WriteFile(sourceFile, []byte(body+"\n# changed\n"), 0644); err != nil {
		t.Fatal(err)
	}
	scanBuf.Reset()
	if err := executeScan(scanOptions{
		configPath: configPath,
		cachePath:  cachePath,
		asJSON:     true,
		out:        &scanBuf,
	}); err != nil {
		t.Fatalf("second scan: %v", err)
	}
	report = scanReport{}
	if err := json.Unmarshal(scanBuf.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if report.Previous == nil || report.Previous.ProjectName != "Hospital API" {
		t.Fatalf("previous = %+v", report.Previous)
	}
	if len(report.Previous.Guides) != 1 || len(report.Stale) != 2 {
		t.Fatalf("guides=%d stale=%+v", len(report.Previous.Guides), report.Stale)
	}
}
