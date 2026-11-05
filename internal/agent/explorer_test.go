package agent

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/byabasaija/indoc/internal/llm"
)

// mockAgentProvider simulates a 2-turn agent conversation:
// Turn 1: requests read_source_file
// Turn 2: returns final JSON doc
type mockAgentProvider struct {
	turn int
}

func (m *mockAgentProvider) Chat(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	m.turn++
	if m.turn == 1 {
		// Simulate agent calling tool
		return &llm.ChatResponse{
			ToolCalls: []llm.ToolCall{
				{
					ID:   "call_1",
					Type: "function",
					Function: llm.FunctionCall{
						Name:      "read_source_file",
						Arguments: `{"file_path": "models/user.go"}`,
					},
				},
			},
		}, nil
	}

	// Turn 2: Simulate agent returning final JSON
	jsonContent := `{
		"title": "Auth Controller",
		"summary": "Handles user authentication and JWT sessions",
		"content_markdown": "# Auth Controller\n\nUses User model to verify credentials.",
		"routes": [
			{"method": "POST", "path": "/api/v1/login", "description": "Authenticate user"}
		]
	}`

	return &llm.ChatResponse{
		Content: jsonContent,
	}, nil
}

func TestExplorerAgentReActLoop(t *testing.T) {
	tempDir := t.TempDir()

	// Create test files
	controllerFile := filepath.Join(tempDir, "auth.go")
	if err := os.WriteFile(controllerFile, []byte("package controllers\nfunc Login() {}"), 0644); err != nil {
		t.Fatal(err)
	}

	modelDir := filepath.Join(tempDir, "models")
	if err := os.MkdirAll(modelDir, 0755); err != nil {
		t.Fatal(err)
	}
	modelFile := filepath.Join(modelDir, "user.go")
	if err := os.WriteFile(modelFile, []byte("package models\ntype User struct {}"), 0644); err != nil {
		t.Fatal(err)
	}

	registry := NewToolRegistry(tempDir, []string{"auth.go", "models/user.go"})
	mockProv := &mockAgentProvider{}

	agent := NewExplorerAgent(mockProv, "test-model", registry, "TestApp", "", false)
	doc, err := agent.DocumentFile(context.Background(), "auth.go", "Go")
	if err != nil {
		t.Fatalf("agent DocumentFile failed: %v", err)
	}

	if doc.Title != "Auth Controller" {
		t.Errorf("expected Title 'Auth Controller', got: %s", doc.Title)
	}
	if len(doc.Routes) != 1 || doc.Routes[0].Path != "/api/v1/login" {
		t.Errorf("expected 1 route '/api/v1/login', got: %+v", doc.Routes)
	}
	if mockProv.turn != 2 {
		t.Errorf("expected 2 turns in ReAct loop, got %d", mockProv.turn)
	}
}
