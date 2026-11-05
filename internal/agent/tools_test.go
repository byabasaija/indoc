package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestToolRegistryReadAndSecurity(t *testing.T) {
	tempDir := t.TempDir()

	// Create test file
	subDir := filepath.Join(tempDir, "models")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatal(err)
	}

	userGo := filepath.Join(subDir, "user.go")
	content := "package models\n\ntype User struct {\n\tID int\n\tEmail string\n}\n"
	if err := os.WriteFile(userGo, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	registry := NewToolRegistry(tempDir, []string{"models/user.go"})

	// 1. Read existing file
	out, err := registry.Execute("read_source_file", `{"file_path": "models/user.go"}`)
	if err != nil {
		t.Fatalf("unexpected error reading file: %v", err)
	}
	if !strings.Contains(out, "type User struct") {
		t.Errorf("expected content to contain 'type User struct', got: %s", out)
	}

	// 2. Read missing file
	outMissing, err := registry.Execute("read_source_file", `{"file_path": "models/missing.go"}`)
	if err != nil {
		t.Fatalf("unexpected error on missing file: %v", err)
	}
	if !strings.Contains(outMissing, "not found") {
		t.Errorf("expected 'not found' message, got: %s", outMissing)
	}

	// 3. Path traversal attack attempt
	_, errTraversal := registry.Execute("read_source_file", `{"file_path": "../../etc/passwd"}`)
	if errTraversal == nil {
		t.Fatal("expected security error on path traversal, got nil")
	}
	if !strings.Contains(errTraversal.Error(), "access denied") {
		t.Errorf("expected 'access denied' in error, got: %v", errTraversal)
	}
}

func TestToolRegistrySearch(t *testing.T) {
	tempDir := t.TempDir()

	file1 := filepath.Join(tempDir, "auth.go")
	if err := os.WriteFile(file1, []byte("func Login() {}\nfunc Logout() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}

	registry := NewToolRegistry(tempDir, []string{"auth.go"})

	// 1. Successful search
	out, err := registry.Execute("search_code", `{"query": "Logout"}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "auth.go:2: func Logout() {}") {
		t.Errorf("expected search match on line 2, got: %s", out)
	}

	// 2. No match search
	outNoMatch, err := registry.Execute("search_code", `{"query": "NonExistentSymbol"}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(outNoMatch, "No matches found") {
		t.Errorf("expected 'No matches found', got: %s", outNoMatch)
	}
}

func TestToolRegistryListFiles(t *testing.T) {
	registry := NewToolRegistry(".", []string{"main.go", "config.go"})
	out, err := registry.Execute("list_source_files", `{}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "main.go") || !strings.Contains(out, "config.go") {
		t.Errorf("expected list to contain files, got: %s", out)
	}
}

