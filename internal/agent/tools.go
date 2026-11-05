package agent

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/byabasaija/indoc/internal/llm"
)

// ToolRegistry manages the tools available to indoc agents.
type ToolRegistry struct {
	projectRoot string
	files       []string
}

// NewToolRegistry constructs a registry rooted at projectRoot with known files.
func NewToolRegistry(projectRoot string, knownFiles []string) *ToolRegistry {
	return &ToolRegistry{
		projectRoot: projectRoot,
		files:       knownFiles,
	}
}

// GetDefinitions returns the JSON Schema definitions sent to the LLM.
func (r *ToolRegistry) GetDefinitions() []llm.Tool {
	return []llm.Tool{
		{
			Type: "function",
			Function: llm.FunctionDefinition{
				Name:        "read_source_file",
				Description: "Read the contents of a source code file in the project to inspect types, structs, or functions.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"file_path": map[string]interface{}{
							"type":        "string",
							"description": "Relative path to the file from project root (e.g. 'models/user.go')",
						},
					},
					"required": []string{"file_path"},
				},
			},
		},
		{
			Type: "function",
			Function: llm.FunctionDefinition{
				Name:        "search_code",
				Description: "Search the codebase for a type, struct, class, function, or symbol name.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"query": map[string]interface{}{
							"type":        "string",
							"description": "The exact symbol or keyword to find (e.g. 'type User', 'class AuthService')",
						},
					},
					"required": []string{"query"},
				},
			},
		},
		{
			Type: "function",
			Function: llm.FunctionDefinition{
				Name:        "list_source_files",
				Description: "List all recognized source files in the project to discover related modules.",
				Parameters: map[string]interface{}{
					"type":       "object",
					"properties": map[string]interface{}{},
				},
			},
		},
	}
}

// Execute executes a requested tool call safely on the local filesystem.
func (r *ToolRegistry) Execute(name, argsJSON string) (string, error) {
	switch name {
	case "read_source_file":
		var args struct {
			FilePath string `json:"file_path"`
		}
		if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
			return "", fmt.Errorf("invalid arguments for read_source_file: %w", err)
		}
		return r.readSourceFile(args.FilePath)

	case "search_code":
		var args struct {
			Query string `json:"query"`
		}
		if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
			return "", fmt.Errorf("invalid arguments for search_code: %w", err)
		}
		return r.searchCode(args.Query)

	case "list_source_files":
		return r.listSourceFiles(), nil

	default:
		return "", fmt.Errorf("unknown tool: %s", name)
	}
}

// readSourceFile securely reads a file ensuring no path traversal escapes projectRoot.
func (r *ToolRegistry) readSourceFile(relPath string) (string, error) {
	// Clean and normalize path
	cleanRel := filepath.Clean(relPath)
	fullPath := filepath.Join(r.projectRoot, cleanRel)

	// Security check: ensure path does not escape projectRoot
	relToRoot, err := filepath.Rel(r.projectRoot, fullPath)
	if err != nil || strings.HasPrefix(relToRoot, "..") {
		return "", fmt.Errorf("access denied: path '%s' is outside project root", relPath)
	}

	data, err := os.ReadFile(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Sprintf("File '%s' not found.", relPath), nil
		}
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	// Limit output to prevent blowing up the LLM context window (first 400 lines)
	lines := strings.Split(string(data), "\n")
	if len(lines) > 400 {
		return strings.Join(lines[:400], "\n") + "\n\n... [truncated remaining lines]", nil
	}

	return string(data), nil
}

// searchCode searches through project source files for a query string.
func (r *ToolRegistry) searchCode(query string) (string, error) {
	if query == "" {
		return "Query cannot be empty", nil
	}

	var results []string
	queryLower := strings.ToLower(query)

	for _, file := range r.files {
		fullPath := filepath.Join(r.projectRoot, file)
		f, err := os.Open(fullPath)
		if err != nil {
			continue
		}

		scanner := bufio.NewScanner(f)
		lineNum := 1
		for scanner.Scan() {
			line := scanner.Text()
			if strings.Contains(strings.ToLower(line), queryLower) {
				results = append(results, fmt.Sprintf("%s:%d: %s", file, lineNum, strings.TrimSpace(line)))
				if len(results) >= 20 { // Cap results to 20 lines to preserve tokens
					break
				}
			}
			lineNum++
		}
		f.Close()

		if len(results) >= 20 {
			break
		}
	}

	if len(results) == 0 {
		return fmt.Sprintf("No matches found for '%s'", query), nil
	}

	return strings.Join(results, "\n"), nil
}

// listSourceFiles returns a formatted list of all discovered files.
func (r *ToolRegistry) listSourceFiles() string {
	if len(r.files) == 0 {
		return "No source files registered."
	}
	return strings.Join(r.files, "\n")
}
