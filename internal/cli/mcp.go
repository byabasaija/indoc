package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/byabasaija/indoc/internal/config"
	"github.com/byabasaija/indoc/internal/mcp"
	"github.com/byabasaija/indoc/internal/version"
	"github.com/spf13/cobra"
)

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Run the MCP server on stdio (for coding agents)",
	Long: `mcp serves indoc's operations to a coding agent over the Model Context
Protocol on stdin/stdout: indoc_setup, indoc_scan, indoc_check, indoc_ack, and
indoc_bundle. Plugins for Claude Code and GitHub Copilot CLI start it for you.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return mcpServer().Serve(cmd.InOrStdin(), cmd.OutOrStdout())
	},
}

func init() {
	rootCmd.AddCommand(mcpCmd)
}

const mcpInstructions = `indoc keeps Markdown documentation tied to the code it describes.
Start with indoc_scan: it sets the project up on first use and returns the pages, what needs updating, the routes/configuration/commands extracted from source, and a map of the project.
If it reports that existing docs were found and nothing is published, ask the user which files to publish and call indoc_setup with those globs.
Before writing new pages, propose the readers and an outline to the user. Pages are Markdown with frontmatter (title, order, sources globs, strict) and cite code as path::Symbol.
After writing, call indoc_bundle (it refuses broken citations), then indoc_check. After code changes, indoc_check lists stale pages with reasons; update them, or indoc_ack a page only after re-reading it against the code.`

var rootSchema = map[string]any{
	"type":        "string",
	"description": "Repository root. Defaults to the project the agent is working in.",
}

func mcpServer() *mcp.Server {
	obj := func(props map[string]any) map[string]any {
		props["root"] = rootSchema
		return map[string]any{"type": "object", "properties": props, "additionalProperties": false}
	}
	pagesSchema := map[string]any{"type": "array", "items": map[string]any{"type": "string"}}
	return &mcp.Server{
		Name: "indoc", Version: version.Version, Instructions: mcpInstructions,
		Tools: []mcp.Tool{
			{
				Name: "indoc_setup", Title: "Set up indoc",
				Description: "Create .indoc/config.yaml for the project, or set which Markdown files are published (pages globs such as documentation/**/*.md). When the repository already has Markdown, nothing is published until pages are given: ask the user which files to publish.",
				InputSchema: obj(map[string]any{"pages": pagesSchema}),
				Idempotent:  true,
				Handler:     toolSetup,
			},
			{
				Name: "indoc_scan", Title: "Scan the project",
				Description: "Report the documentation pages, every check finding with its reason, reference facts extracted from source (routes, environment variables, commands), extractor coverage, and a map of the project (frameworks, entry points, task runner commands, unpublished docs, most-changed files). Sets the project up on first use.",
				InputSchema: obj(map[string]any{}),
				Handler:     toolScan,
			},
			{
				Name: "indoc_check", Title: "Check docs against code",
				Description: "List pages that no longer match the code (stale, broken, edited, unstamped, removed) and drift, with reasons. failing > 0 means `indoc check` in CI would fail. Read-only.",
				InputSchema: obj(map[string]any{"strict": map[string]any{"type": "boolean", "description": "Also count drift, unanchored pages, and unverified citations as failing."}}),
				ReadOnly:    true,
				Handler:     toolCheck,
			},
			{
				Name: "indoc_ack", Title: "Record pages as verified",
				Description: "Record pages in .indoc/lock.json. With no pages: records new and edited pages. With pages: records those even if unchanged, meaning 'reviewed against the current code and still correct'. Only name a page after re-reading it against the code.",
				InputSchema: obj(map[string]any{"pages": pagesSchema}),
				Idempotent:  true,
				Handler:     toolAck,
			},
			{
				Name: "indoc_bundle", Title: "Build the docs site",
				Description: "Build the static site into output_dir (default .indoc/site/). Refuses to build when a citation or sources glob is broken. Records new and edited pages.",
				InputSchema: obj(map[string]any{}),
				Idempotent:  true,
				Handler:     toolBundle,
			},
		},
	}
}

type toolArgs struct {
	Root   string   `json:"root"`
	Pages  []string `json:"pages"`
	Strict bool     `json:"strict"`
}

func parseArgs(raw json.RawMessage) (toolArgs, string, error) {
	var a toolArgs
	if err := json.Unmarshal(raw, &a); err != nil {
		return a, "", fmt.Errorf("invalid arguments: %w", err)
	}
	root := a.Root
	if root == "" {
		root = os.Getenv("CLAUDE_PROJECT_DIR")
	}
	if root == "" {
		var err error
		if root, err = os.Getwd(); err != nil {
			return a, "", err
		}
	}
	return a, filepath.Join(root, config.DefaultPath), nil
}

func failed(err error) (string, any, bool) { return err.Error(), nil, true }

// setup creates the config, or updates its pages, and reports whether the
// user still has to choose what to publish.
func setup(cfgPath string, pages []string) (string, bool, error) {
	var out bytes.Buffer
	if _, err := os.Stat(cfgPath); err == nil {
		cfg, err := config.Load(cfgPath)
		if err != nil {
			return "", false, err
		}
		if len(pages) > 0 {
			cfg.Pages = pages
			if err := cfg.Save(cfgPath); err != nil {
				return "", false, err
			}
			fmt.Fprintf(&out, "Updated pages in %s: %s\n", cfgPath, strings.Join(pages, ", "))
		} else {
			fmt.Fprintf(&out, "%s already exists; pages: %s\n", cfgPath, strings.Join(cfg.Pages, ", "))
		}
		return out.String(), len(cfg.Pages) == 0, nil
	}
	if err := runInit(cfgPath, &out, pages, "", false); err != nil {
		return "", false, err
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return "", false, err
	}
	return out.String(), len(cfg.Pages) == 0, nil
}

const choosePages = "\nNothing is published yet. Ask the user which of these files to publish (internal notes, reviews, and plans usually should not be), then call indoc_setup with their globs."

func toolSetup(raw json.RawMessage) (string, any, bool) {
	a, cfgPath, err := parseArgs(raw)
	if err != nil {
		return failed(err)
	}
	text, needsChoice, err := setup(cfgPath, a.Pages)
	if err != nil {
		return failed(err)
	}
	if needsChoice {
		text += choosePages
	}
	return text, nil, false
}

func toolScan(raw json.RawMessage) (string, any, bool) {
	_, cfgPath, err := parseArgs(raw)
	if err != nil {
		return failed(err)
	}
	prefix := ""
	if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
		text, needsChoice, err := setup(cfgPath, nil)
		if err != nil {
			return failed(err)
		}
		if needsChoice {
			return "Set up indoc for this project.\n" + text + choosePages, nil, false
		}
		prefix = "Set up indoc for this project.\n" + text + "\n"
	}
	var out bytes.Buffer
	if err := runScan(cfgPath, &out, true); err != nil {
		if strings.Contains(err.Error(), "matches no Markdown files") {
			return prefix + err.Error() + "\nNo pages exist yet: propose readers and an outline to the user, then write the pages.", nil, false
		}
		return failed(err)
	}
	return jsonResult(prefix, out.Bytes())
}

func toolCheck(raw json.RawMessage) (string, any, bool) {
	a, cfgPath, err := parseArgs(raw)
	if err != nil {
		return failed(err)
	}
	var out bytes.Buffer
	if err := runCheck(cfgPath, &out, a.Strict, true, true); err != nil {
		return failed(err)
	}
	return jsonResult("", out.Bytes())
}

func toolAck(raw json.RawMessage) (string, any, bool) {
	a, cfgPath, err := parseArgs(raw)
	if err != nil {
		return failed(err)
	}
	var out bytes.Buffer
	if err := runAck(cfgPath, &out, a.Pages); err != nil {
		return out.String() + err.Error(), nil, true
	}
	return out.String(), nil, false
}

func toolBundle(raw json.RawMessage) (string, any, bool) {
	_, cfgPath, err := parseArgs(raw)
	if err != nil {
		return failed(err)
	}
	var out bytes.Buffer
	if err := runBundle(cfgPath, "", &out); err != nil {
		return out.String() + err.Error(), nil, true
	}
	return strings.Replace(out.String(), "Preview locally with: indoc preview", "The user can preview it with `indoc preview`.", 1), nil, false
}

// jsonResult returns JSON output as both text and structured content.
func jsonResult(prefix string, data []byte) (string, any, bool) {
	var structured map[string]any
	if err := json.Unmarshal(data, &structured); err != nil {
		return prefix + string(data), nil, false
	}
	return prefix + string(data), structured, false
}
