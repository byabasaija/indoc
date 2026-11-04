package scanner

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectLanguage(t *testing.T) {
	tests := []struct {
		path     string
		expected string
		ok       bool
	}{
		{"main.go", "Go", true},
		{"server/app.py", "Python", true},
		{"frontend/index.tsx", "TypeScript", true},
		{"db/schema.sql", "SQL", true},
		{"image.png", "", false},
		{"README.md", "", false},
		{"archive.tar.gz", "", false},
	}

	for _, tt := range tests {
		lang, ok := DetectLanguage(tt.path)
		if ok != tt.ok || lang != tt.expected {
			t.Errorf("DetectLanguage(%q) = (%q, %v); expected (%q, %v)", tt.path, lang, ok, tt.expected, tt.ok)
		}
	}
}

func TestSHA256Hashing(t *testing.T) {
	content := "package main\n\nfunc main() {}\n"
	expectedHash := ComputeStringSHA256(content)

	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "main.go")
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	fileHash, err := ComputeFileSHA256(filePath)
	if err != nil {
		t.Fatalf("ComputeFileSHA256 failed: %v", err)
	}

	if fileHash != expectedHash {
		t.Errorf("expected hash %q, got %q", expectedHash, fileHash)
	}
}

func TestComputeDiff(t *testing.T) {
	cached := &CacheManifest{
		Version:  CacheSchemaVersion,
		LastScan: "2026-01-01T00:00:00Z",
		FileCache: map[string]FileMeta{
			"unchanged.go": {Path: "unchanged.go", Hash: "hash-same", Language: "Go"},
			"modified.go":  {Path: "modified.go", Hash: "hash-old", Language: "Go"},
			"deleted.go":   {Path: "deleted.go", Hash: "hash-deleted", Language: "Go"},
		},
	}

	current := map[string]FileMeta{
		"unchanged.go": {Path: "unchanged.go", Hash: "hash-same", Language: "Go"},
		"modified.go":  {Path: "modified.go", Hash: "hash-new", Language: "Go"},
		"added.go":     {Path: "added.go", Hash: "hash-added", Language: "Go"},
	}

	diff := ComputeDiff(cached, current)

	if len(diff.Added) != 1 || diff.Added[0].Path != "added.go" {
		t.Errorf("expected 1 added file ('added.go'), got %v", diff.Added)
	}
	if len(diff.Modified) != 1 || diff.Modified[0].Path != "modified.go" {
		t.Errorf("expected 1 modified file ('modified.go'), got %v", diff.Modified)
	}
	if len(diff.Unchanged) != 1 || diff.Unchanged[0].Path != "unchanged.go" {
		t.Errorf("expected 1 unchanged file ('unchanged.go'), got %v", diff.Unchanged)
	}
	if len(diff.Deleted) != 1 || diff.Deleted[0] != "deleted.go" {
		t.Errorf("expected 1 deleted file ('deleted.go'), got %v", diff.Deleted)
	}
	if !diff.HasChanges() {
		t.Errorf("expected HasChanges() to be true")
	}
}

func TestScannerFullCycle(t *testing.T) {
	tempDir := t.TempDir()

	// 1. Setup mock source tree
	srcDir := filepath.Join(tempDir, "src")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatal(err)
	}
	vendorDir := filepath.Join(tempDir, "vendor")
	if err := os.MkdirAll(vendorDir, 0755); err != nil {
		t.Fatal(err)
	}

	file1 := filepath.Join(srcDir, "main.go")
	file2 := filepath.Join(srcDir, "auth.py")
	fileVendor := filepath.Join(vendorDir, "dep.go")
	fileTxt := filepath.Join(srcDir, "notes.txt")

	if err := os.WriteFile(file1, []byte("package main"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file2, []byte("def login(): pass"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fileVendor, []byte("package vendor"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fileTxt, []byte("ignore me"), 0644); err != nil {
		t.Fatal(err)
	}

	cacheFile := filepath.Join(tempDir, ".indoc", "cache.json")
	scanner := NewScanner([]string{srcDir, vendorDir}, []string{"vendor"}, false, cacheFile)

	// --- Pass 1: Fresh Scan ---
	result, err := scanner.Scan()
	if err != nil {
		t.Fatalf("first scan failed: %v", err)
	}

	// Expect 2 files (main.go and auth.py). vendor should be excluded, notes.txt should be ignored.
	if result.TotalFiles != 2 {
		t.Fatalf("expected 2 files discovered, got %d", result.TotalFiles)
	}
	if len(result.Diff.Added) != 2 {
		t.Fatalf("expected 2 added files on fresh scan, got %d", len(result.Diff.Added))
	}

	// Save to cache
	manifest := NewCacheManifest()
	manifest.FileCache = result.Files
	if err := SaveCache(cacheFile, manifest); err != nil {
		t.Fatalf("failed to save cache: %v", err)
	}

	// --- Pass 2: Unchanged Scan ---
	result2, err := scanner.Scan()
	if err != nil {
		t.Fatalf("second scan failed: %v", err)
	}
	if len(result2.Diff.Unchanged) != 2 {
		t.Fatalf("expected 2 unchanged files, got %d", len(result2.Diff.Unchanged))
	}
	if result2.Diff.HasChanges() {
		t.Errorf("expected no changes on identical scan")
	}

	// --- Pass 3: Modify a file ---
	if err := os.WriteFile(file1, []byte("package main\n// updated"), 0644); err != nil {
		t.Fatal(err)
	}

	result3, err := scanner.Scan()
	if err != nil {
		t.Fatalf("third scan failed: %v", err)
	}
	if len(result3.Diff.Modified) != 1 {
		t.Fatalf("expected 1 modified file, got %d", len(result3.Diff.Modified))
	}
	if len(result3.Diff.Unchanged) != 1 {
		t.Fatalf("expected 1 unchanged file, got %d", len(result3.Diff.Unchanged))
	}
}
