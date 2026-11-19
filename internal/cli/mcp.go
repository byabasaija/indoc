package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/byabasaija/indoc/internal/config"
	"github.com/byabasaija/indoc/internal/doc"
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
Before writing new pages, ask the user whether the docs' readers can open the repository (if so, set source_links with indoc_setup), which logo to use (map.logos lists candidates; set it with indoc_setup) and propose the readers and an outline, with the home page as a landing page (layout: landing, hero, features), to the user. Pages are Markdown with frontmatter (title, order, sources globs, strict) and cite code as path::Symbol.
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
				Description: "Create .indoc/config.yaml for the project, set which Markdown files are published (pages globs such as documentation/**/*.md), and set the site's logo. When the repository already has Markdown, nothing is published until pages are given: ask the user which files to publish. Set logo only to a file or URL the user chose.",
				InputSchema: obj(map[string]any{
					"pages":        pagesSchema,
					"logo":         map[string]any{"type": "string", "description": "The project's logo: an image path relative to the repository root (SVG or a square PNG works best), or an https URL. Shown in the header, as the favicon, and on a landing page."},
					"source_links": map[string]any{"type": "boolean", "description": "Whether the site links to the code (citations, reference entries, edit links). True only if the docs' readers can open the repository, such as a public repository."},
				}),
				Idempotent: true,
				Handler:    toolSetup,
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
			{
				Name: "indoc_publish", Title: "Choose where the docs are served",
				Description: "Record where the docs site is served and return how to wire it up. target: pages (GitHub Pages; writes .github/workflows/indoc-pages.yml), host (Netlify, Vercel, Cloudflare Pages, S3), app (served by this project's web app at path, with access public or login; returns code for the detected framework for you to add to the app), or none. Ask the user before calling it. Without arguments, returns the instructions for the current target.",
				InputSchema: obj(map[string]any{
					"target": map[string]any{"type": "string", "enum": []string{"pages", "host", "app", "none"}},
					"path":   map[string]any{"type": "string", "description": "app: URL path, such as /guide/. Avoid paths the app already uses."},
					"access": map[string]any{"type": "string", "enum": []string{"public", "login"}, "description": "app: login serves the docs to signed-in users only (staff in Django)."},
				}),
				Handler: toolPublish,
			},
		},
	}
}

type toolArgs struct {
	Root   string   `json:"root"`
	Pages  []string `json:"pages"`
	Logo   string   `json:"logo"`
	Target string   `json:"target"`
	Path   string   `json:"path"`
	Access string   `json:"access"`
	// SourceLinks is a pointer so "not given" differs from false.
	SourceLinks *bool `json:"source_links"`
	Strict      bool  `json:"strict"`
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

const noLogo = "No logo is set. Ask the user which image to use: offer the candidates in map.logos (note any that are tall lockups with small text; a square mark or SVG suits the header and favicon), or ask them for a path or URL. Then call indoc_setup with logo. If they have none, continue without.\n"

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
	if a.Logo != "" {
		if err := setLogo(cfgPath, a.Logo); err != nil {
			return failed(err)
		}
		text += "Logo set: " + a.Logo + "\n"
	}
	if a.SourceLinks != nil {
		cfg, err := config.Load(cfgPath)
		if err != nil {
			return failed(err)
		}
		cfg.SourceLinks = *a.SourceLinks
		if err := cfg.Save(cfgPath); err != nil {
			return failed(err)
		}
		text += fmt.Sprintf("source_links: %t\n", cfg.SourceLinks)
	}
	if needsChoice {
		text += choosePages
	}
	return text, nil, false
}

var logoImageExts = map[string]bool{".svg": true, ".png": true, ".webp": true, ".jpg": true, ".jpeg": true, ".gif": true, ".ico": true}

// setLogo records appearance.logo after checking the file exists and is an
// image, so a typo is caught here rather than as a broken image on the site.
func setLogo(cfgPath, logo string) error {
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return err
	}
	if !strings.HasPrefix(logo, "https://") && !strings.HasPrefix(logo, "http://") {
		logo = filepath.ToSlash(filepath.Clean(logo))
		if filepath.IsAbs(logo) || strings.HasPrefix(logo, "../") {
			return fmt.Errorf("logo %q must be relative to the repository root, or a URL", logo)
		}
		if !logoImageExts[strings.ToLower(filepath.Ext(logo))] {
			return fmt.Errorf("logo %q is not an image (use SVG, PNG, WebP, JPEG, GIF, or ICO)", logo)
		}
		root := config.Resolve(cfgPath).Root
		if info, err := os.Stat(filepath.Join(root, filepath.FromSlash(logo))); err != nil || info.IsDir() {
			return fmt.Errorf("logo %q does not exist in the repository", logo)
		}
	}
	if cfg.Appearance == nil {
		cfg.Appearance = &doc.Appearance{}
	}
	cfg.Appearance.Logo = logo
	return cfg.Save(cfgPath)
}

func toolPublish(raw json.RawMessage) (string, any, bool) {
	a, cfgPath, err := parseArgs(raw)
	if err != nil {
		return failed(err)
	}
	var out bytes.Buffer
	if err := runPublish(cfgPath, &out, a.Target, a.Path, a.Access); err != nil {
		return failed(err)
	}
	return out.String(), nil, false
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
		return failed(err)
	}
	var head struct {
		Pages []json.RawMessage `json:"pages"`
		Logo  string            `json:"logo"`
	}
	json.Unmarshal(out.Bytes(), &head)
	if len(head.Pages) == 0 {
		prefix += "No pages exist yet: propose readers and an outline to the user, including a landing page, then write the pages.\n"
	}
	if head.Logo == "" {
		prefix += noLogo
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
