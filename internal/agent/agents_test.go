package agent

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/byabasaija/indoc/internal/llm"
)

type staticMockProvider struct {
	response string
}

func (s *staticMockProvider) Chat(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	return &llm.ChatResponse{
		Content: s.response,
	}, nil
}

func TestArchitectAgent(t *testing.T) {
	mockJSON := `{"title": "Hospital API Architecture", "summary": "Core patient and billing systems", "content_markdown": "# Hospital System"}`

	agent := NewArchitectAgent(&staticMockProvider{response: mockJSON}, "gpt-4o-mini", "Hospital App", false)
	modules := []*ModuleDoc{
		{
			Title:    "Patient Controller",
			FilePath: "controllers/patients.go",
			Summary:  "Handles admissions",
		},
	}

	doc, err := agent.SynthesizeOverview(context.Background(), modules)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if doc.Title != "Hospital API Architecture" {
		t.Errorf("expected Title 'Hospital API Architecture', got: %s", doc.Title)
	}
}

func TestRefinerAgent(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "auth.go")
	if err := os.WriteFile(filePath, []byte("package auth"), 0644); err != nil {
		t.Fatal(err)
	}

	registry := NewToolRegistry(tempDir, []string{"auth.go"})

	mockJSON := `{
		"title": "Auth Controller (Updated)",
		"summary": "Updated with curl examples",
		"content_markdown": "# Auth\n\ncurl -X POST http://localhost:8080/login",
		"routes": []
	}`

	refiner := NewRefinerAgent(&staticMockProvider{response: mockJSON}, "gpt-4o-mini", registry, false)
	currentDoc := &ModuleDoc{
		ID:              "auth-go",
		Title:           "Auth Controller",
		FilePath:        "auth.go",
		Language:        "Go",
		ContentMarkdown: "# Auth",
	}

	updated, err := refiner.RefineModule(context.Background(), currentDoc, "Add curl examples")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if updated.Title != "Auth Controller (Updated)" {
		t.Errorf("expected updated title, got: %s", updated.Title)
	}
}

func TestParseModuleDocMarkdownFences(t *testing.T) {
	raw := "```json\n{\n  \"title\": \"Fenced Doc\",\n  \"summary\": \"Test summary\",\n  \"content_markdown\": \"# Hello\"\n}\n```"
	doc, err := ParseModuleDoc(raw, "file.go", "Go")
	if err != nil {
		t.Fatalf("failed to parse fenced JSON: %v", err)
	}
	if doc.Title != "Fenced Doc" {
		t.Errorf("expected 'Fenced Doc', got: %s", doc.Title)
	}
}
