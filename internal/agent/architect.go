package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/byabasaija/indoc/internal/llm"
)

// SystemOverviewDoc represents the global architecture documentation for the project.
type SystemOverviewDoc struct {
	Title           string `json:"title"`
	Summary         string `json:"summary"`
	ContentMarkdown string `json:"content_markdown"`
}

// ArchitectAgent synthesizes high-level system architecture and diagrams across all modules.
type ArchitectAgent struct {
	provider    llm.Provider
	model       string
	projectName string
	verbose     bool
}

// NewArchitectAgent creates an ArchitectAgent.
func NewArchitectAgent(provider llm.Provider, model, projectName string, verbose bool) *ArchitectAgent {
	return &ArchitectAgent{
		provider:    provider,
		model:       model,
		projectName: projectName,
		verbose:     verbose,
	}
}

// SynthesizeOverview reads all module summaries and generates a global architecture overview.
func (a *ArchitectAgent) SynthesizeOverview(ctx context.Context, modules []*ModuleDoc) (*SystemOverviewDoc, error) {
	if len(modules) == 0 {
		return &SystemOverviewDoc{
			Title:           a.projectName,
			Summary:         "No source modules discovered.",
			ContentMarkdown: fmt.Sprintf("# %s\n\nNo source modules were found to document.", a.projectName),
		}, nil
	}

	// 1. Prepare module digest
	var moduleSummaries strings.Builder
	for _, m := range modules {
		moduleSummaries.WriteString(fmt.Sprintf("- **%s** (`%s`): %s\n", m.Title, m.FilePath, m.Summary))
		if len(m.Routes) > 0 {
			for _, r := range m.Routes {
				moduleSummaries.WriteString(fmt.Sprintf("  - Route: `%s %s` - %s\n", r.Method, r.Path, r.Description))
			}
		}
	}

	systemPrompt := `You are indoc's Principal System Architect.
Your task is to analyze all discovered modules in a project and generate the master architecture overview.

GUIDELINES:
1. Write a compelling, high-level technical overview of the project.
2. Generate an accurate Mermaid Architecture Diagram (flowchart TD or flowchart LR) illustrating how controllers, services, database models, and APIs interact.
3. Generate a Mermaid Sequence Diagram illustrating a typical core user journey or request lifecycle.
4. Highlight key capabilities, data flows, and security/design principles.
5. Format strictly as a valid JSON object matching this schema:
{
  "title": "System Architecture & Overview",
  "summary": "Executive summary of the entire application architecture",
  "content_markdown": "# System Overview\n\n..."
}`

	userContent := fmt.Sprintf(
		"Project Name: %s\nTotal Modules: %d\n\nDiscovered Modules:\n%s\n\nSynthesize the master architecture overview and diagrams.",
		a.projectName,
		len(modules),
		moduleSummaries.String(),
	)

	resp, err := a.provider.Chat(ctx, llm.ChatRequest{
		Model: a.model,
		Messages: []llm.Message{
			llm.SystemMessage(systemPrompt),
			llm.UserMessage(userContent),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("architect agent synthesis failed: %w", err)
	}

	clean := strings.TrimSpace(resp.Content)
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

	var doc SystemOverviewDoc
	if err := json.Unmarshal([]byte(clean), &doc); err != nil {
		// Fallback: If JSON unmarshaling fails, use the raw response content
		return &SystemOverviewDoc{
			Title:           fmt.Sprintf("%s - Architecture Overview", a.projectName),
			Summary:         "High-level system architecture and module relationships",
			ContentMarkdown: resp.Content,
		}, nil
	}

	return &doc, nil
}

