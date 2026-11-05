package agent

import (
	"encoding/json"
	"fmt"
	"strings"
)

// RouteInfo represents an HTTP route extracted from source code.
type RouteInfo struct {
	Method      string `json:"method"`
	Path        string `json:"path"`
	Description string `json:"description"`
}

// ModuleDoc represents the synthesized documentation for a single source file.
type ModuleDoc struct {
	ID              string      `json:"id"`
	Title           string      `json:"title"`
	Summary         string      `json:"summary"`
	FilePath        string      `json:"file_path"`
	Language        string      `json:"language"`
	ContentMarkdown string      `json:"content_markdown"`
	Routes          []RouteInfo `json:"routes,omitempty"`
}

// ParseModuleDoc extracts a ModuleDoc struct from the LLM's text output.
// It handles raw JSON as well as JSON enclosed in markdown code fences (```json ... ```).
func ParseModuleDoc(raw string, defaultFilePath, defaultLang string) (*ModuleDoc, error) {
	clean := strings.TrimSpace(raw)

	// Strip markdown code fences if present
	if strings.HasPrefix(clean, "```json") {
		clean = strings.TrimPrefix(clean, "```json")
		if idx := strings.LastIndex(clean, "```"); idx != -1 {
			clean = clean[:idx]
		}
	} else if strings.HasPrefix(clean, "```") {
		clean = strings.TrimPrefix(clean, "```")
		if idx := strings.LastIndex(clean, "```"); idx != -1 {
			clean = clean[:idx]
		}
	}
	clean = strings.TrimSpace(clean)

	var doc ModuleDoc
	if err := json.Unmarshal([]byte(clean), &doc); err != nil {
		return nil, fmt.Errorf("failed to parse module doc JSON: %w", err)
	}

	// Fallbacks if LLM omitted standard metadata
	if doc.FilePath == "" {
		doc.FilePath = defaultFilePath
	}
	if doc.Language == "" {
		doc.Language = defaultLang
	}
	if doc.ID == "" {
		// Generate slug ID from file path
		slug := strings.ReplaceAll(defaultFilePath, "/", "-")
		slug = strings.ReplaceAll(slug, ".", "-")
		doc.ID = strings.ToLower(slug)
	}

	return &doc, nil
}
