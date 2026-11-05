package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/byabasaija/indoc/internal/llm"
)

// ExplorerAgent analyzes an individual source code file using an agentic ReAct loop.
type ExplorerAgent struct {
	provider           llm.Provider
	model              string
	registry           *ToolRegistry
	projectName        string
	customInstructions string
	maxTurns           int
	verbose            bool
}

// NewExplorerAgent creates an ExplorerAgent.
func NewExplorerAgent(
	provider llm.Provider,
	model string,
	registry *ToolRegistry,
	projectName string,
	customInstructions string,
	verbose bool,
) *ExplorerAgent {
	return &ExplorerAgent{
		provider:           provider,
		model:              model,
		registry:           registry,
		projectName:        projectName,
		customInstructions: customInstructions,
		maxTurns:           5, // Cap to 5 iterations for speed and token efficiency
		verbose:            verbose,
	}
}

// DocumentFile runs the ReAct loop to document a source file.
func (a *ExplorerAgent) DocumentFile(ctx context.Context, filePath, language string) (*ModuleDoc, error) {
	// 1. Read initial target file content
	codeContent, err := a.registry.readSourceFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read target file '%s': %w", filePath, err)
	}

	// 2. Build system and initial user prompt
	systemPrompt := a.buildSystemPrompt()
	userContent := fmt.Sprintf(
		"Project: %s\nFile: %s\nLanguage: %s\n\n```%s\n%s\n```\n\nAnalyze this file and produce comprehensive documentation. If you need details on referenced models, structs, or functions, call read_source_file or search_code.",
		a.projectName,
		filePath,
		language,
		strings.ToLower(language),
		codeContent,
	)

	messages := []llm.Message{
		llm.SystemMessage(systemPrompt),
		llm.UserMessage(userContent),
	}

	tools := a.registry.GetDefinitions()

	// 3. The ReAct Agent Loop
	for turn := 0; turn < a.maxTurns; turn++ {
		req := llm.ChatRequest{
			Model:    a.model,
			Messages: messages,
			Tools:    tools,
		}

		resp, err := a.provider.Chat(ctx, req)
		if err != nil {
			return nil, fmt.Errorf("LLM error during file analysis: %w", err)
		}

		// If no tool calls were requested, the agent has finished its synthesis!
		if len(resp.ToolCalls) == 0 {
			if a.verbose {
				fmt.Printf("      ✓ Completed synthesis for %s (Turn %d)\n", filePath, turn+1)
			}
			return ParseModuleDoc(resp.Content, filePath, language)
		}

		// Agent requested one or more tool calls
		if a.verbose {
			for _, call := range resp.ToolCalls {
				fmt.Printf("      ↳ [Tool Call] %s(%s)\n", call.Function.Name, call.Function.Arguments)
			}
		}

		messages = append(messages, llm.AssistantToolCallsMessage(resp.ToolCalls))

		// Execute tools and append observation results
		for _, call := range resp.ToolCalls {
			result, err := a.registry.Execute(call.Function.Name, call.Function.Arguments)
			if err != nil {
				result = fmt.Sprintf("Error executing tool: %v", err)
			}
			messages = append(messages, llm.ToolResultMessage(call.ID, result))
		}
	}

	// Safety fallback: if turns exhausted, request final JSON output
	messages = append(messages, llm.UserMessage("Please finalize your analysis now and output the JSON documentation."))
	fallbackResp, err := a.provider.Chat(ctx, llm.ChatRequest{
		Model:    a.model,
		Messages: messages,
	})
	if err != nil {
		return nil, fmt.Errorf("failed during fallback generation: %w", err)
	}

	return ParseModuleDoc(fallbackResp.Content, filePath, language)
}

// buildSystemPrompt constructs instructions for the explorer agent.
func (a *ExplorerAgent) buildSystemPrompt() string {
	var sb strings.Builder

	sb.WriteString("You are indoc's Explorer Agent, a world-class technical documentation engineer.\n")
	sb.WriteString("Your goal is to inspect source code and produce clear, beautiful, and deeply accurate developer documentation.\n\n")

	sb.WriteString("GUIDELINES:\n")
	sb.WriteString("1. Explain the module's core responsibility, exported types, structs, interfaces, and public functions.\n")
	sb.WriteString("2. Extract any HTTP routes or API endpoints (method, path, description).\n")
	sb.WriteString("3. Use Mermaid diagrams (```mermaid ... ```) for complex flows, lifecycles, or state transitions.\n")
	sb.WriteString("4. Format content using GitHub-flavored Markdown with tables and syntax-highlighted code blocks.\n")
	sb.WriteString("5. AGENTIC RULE: If you encounter structs, interfaces, database models, or functions imported/referenced from other files, USE your tools (read_source_file, search_code) to inspect them so your documentation is 100% accurate!\n")

	if a.customInstructions != "" {
		sb.WriteString(fmt.Sprintf("\nPROJECT INSTRUCTIONS:\n%s\n", a.customInstructions))
	}

	sb.WriteString("\nOUTPUT FORMAT:\n")
	sb.WriteString("When you have completed your investigation, respond STRICTLY with a valid JSON object matching this schema:\n")
	sb.WriteString("{\n")
	sb.WriteString("  \"title\": \"Human-friendly Module Title\",\n")
	sb.WriteString("  \"summary\": \"1-2 sentence executive overview of what this file does\",\n")
	sb.WriteString("  \"content_markdown\": \"# Full documentation in Markdown...\",\n")
	sb.WriteString("  \"routes\": [\n")
	sb.WriteString("    {\"method\": \"POST\", \"path\": \"/api/v1/auth/login\", \"description\": \"Authenticate user and issue JWT\"}\n")
	sb.WriteString("  ]\n")
	sb.WriteString("}\n")

	return sb.String()
}
