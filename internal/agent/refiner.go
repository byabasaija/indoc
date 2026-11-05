package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/byabasaija/indoc/internal/llm"
)

// RefinerAgent updates and refines existing documentation based on direct user instructions.
type RefinerAgent struct {
	provider llm.Provider
	model    string
	registry *ToolRegistry
	verbose  bool
}

// NewRefinerAgent creates a RefinerAgent.
func NewRefinerAgent(provider llm.Provider, model string, registry *ToolRegistry, verbose bool) *RefinerAgent {
	return &RefinerAgent{
		provider: provider,
		model:    model,
		registry: registry,
		verbose:  verbose,
	}
}

// RefineModule updates an existing module documentation according to a user's prompt instruction.
func (r *RefinerAgent) RefineModule(ctx context.Context, currentDoc *ModuleDoc, instruction string) (*ModuleDoc, error) {
	codeContent, err := r.registry.readSourceFile(currentDoc.FilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read source file '%s': %w", currentDoc.FilePath, err)
	}

	systemPrompt := `You are indoc's Refinement Agent.
Your job is to update and polish existing developer documentation according to direct user instructions.

GUIDELINES:
1. Carefully apply the user's requested additions, edits, or formatting changes.
2. Preserve existing accurate API routes, code snippets, and structural clarity.
3. Keep the tone professional, concise, and developer-friendly.
4. Output STRICTLY a valid JSON object matching the ModuleDoc schema:
{
  "title": "Module Title",
  "summary": "1-2 sentence overview",
  "content_markdown": "# Full updated markdown...",
  "routes": [{"method": "...", "path": "...", "description": "..."}]
}`

	userContent := fmt.Sprintf(
		"File: %s\nLanguage: %s\n\nSource Code:\n```%s\n%s\n```\n\nCurrent Documentation:\n```markdown\n%s\n```\n\nUser Refinement Instruction:\n%s\n\nPlease produce the updated documentation JSON.",
		currentDoc.FilePath,
		currentDoc.Language,
		strings.ToLower(currentDoc.Language),
		codeContent,
		currentDoc.ContentMarkdown,
		instruction,
	)

	resp, err := r.provider.Chat(ctx, llm.ChatRequest{
		Model: r.model,
		Messages: []llm.Message{
			llm.SystemMessage(systemPrompt),
			llm.UserMessage(userContent),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("refiner agent failed: %w", err)
	}

	return ParseModuleDoc(resp.Content, currentDoc.FilePath, currentDoc.Language)
}
