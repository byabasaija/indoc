# Architecture Decision Record (ADR) & System Specification

**Status:** Approved  
**Date:** November 2026  
**Subject:** Agent-Assisted Static Documentation Compiler

---

## 1. Context & Motivation

### The Original Premise (Spec v2.0)
`indoc` was originally envisioned as a standalone CLI tool that required developers to:
1. Export a raw third-party API key (`export OPENAI_API_KEY="sk-..."`).
2. Run an internal LLM client inside the CLI to document files.
3. Manage API rate limits, billing, and custom terminal chat loops (`indoc refine`).

### The Realities of Modern Developer Workflows
1. **Existing AI Subscriptions:** Modern developers already work inside AI coding assistants (**Claude Code, Google Antigravity, Cursor, Codex**). Asking developers to purchase and manage a separate API key creates unnecessary friction.
2. **Conversational Refinement Belongs in the Agent:** Real-world documentation requires iterative refinement ("add curl examples", "simplify this architecture diagram", "document the Stripe webhook"). Developers naturally do this inside their existing AI chat interface—not in a clunky terminal prompt.
3. **Portable output:** Documentation should not require a separate docs backend or a web application. The generated bundle is a static artifact that works for web applications, CLI tools, libraries, and other projects. If a host application exists, it can serve the bundle; otherwise, the bundle can be published or shipped independently.

---

## 2. The Decision: Agent-Assisted Static Compiler

We are decoupling the **Intelligence Layer** from the **Compiler & Bundler Layer**:

* **The Host Agent (Claude / Antigravity / Cursor):** Acts as the brain. It reads source code, reasons through dependencies, writes Markdown, generates Mermaid diagrams, and handles conversational refinement using the developer's existing subscription.
* **`indoc` (The Go Engine):** Acts as the deterministic scanner, static compiler, and asset bundler. It validates agent-written content, extracts source facts, and compiles a standalone static site into the project.

The bundle is the product; a particular hosting model is not. Web projects may mount the generated directory in their existing server. CLI-only projects can publish the same static files or ship them with releases. Neither path requires a docs-specific production server. `indoc serve` is an optional local preview convenience, not part of the production architecture.

```
┌─────────────────────────────────────────────────────────────────┐
│               DEVELOPER IN CHAT INTERFACE                       │
│    "Generate indoc documentation for this repo"                 │
└────────────────────────────────┬────────────────────────────────┘
                                 │
                                 ▼
┌─────────────────────────────────────────────────────────────────┐
│              HOST AGENT (Claude / Antigravity / Cursor)         │
│  • Employs developer's existing subscription ($0 extra API keys)│
│  • Reads code context and understands application logic         │
│  • Writes structured Markdown, route schemas, & Mermaid flows   │
│  • Iterates interactively with human feedback                   │
└────────────────────────────────┬────────────────────────────────┘
                                 │
                                 │ (MCP JSON-RPC / CLI invocation)
                                 ▼
┌─────────────────────────────────────────────────────────────────┐
│                  indoc ENGINE (Go Binary)                       │
│                                                                 │
│  1. `indoc scan`:                                               │
│     SHA-256 diff, extracted routes and env vars, and the last   │
│     overview and guides so the agent can patch them.            │
│                                                                 │
│  2. `indoc bundle`:                                             │
│     Validates overview, guides, and reference, then writes the  │
│     portable static viewer.                                     │
│  3. Optional `indoc serve`:                                     │
│     Previews the bundle locally; not required for publishing.   │
└────────────────────────────────┬────────────────────────────────┘
                                 │
                                 ▼ (Writes static assets)
┌─────────────────────────────────────────────────────────────────┐
│            PORTABLE OUTPUT DIRECTORY (`./static/docs`)          │
│                                                                 │
│  ./static/docs/                                                 │
│  ├── index.html   (Single-Page App shell)                       │
│  ├── style.css    (Tailwind/Docusaurus typography)              │
│  ├── app.js       (Offline search engine & Markdown parser)     │
│  └── docs.js      (Self-contained documentation payload)        │
└────────────────────────────────┬────────────────────────────────┘
                                 │
                 ┌───────────────┴────────────────┐
                 ▼                                ▼
┌──────────────────────────────┐  ┌──────────────────────────────┐
│ Web application (optional)   │  │ CLI / library / no web app   │
│ Serve from existing app      │  │ Publish or ship static files │
│ server; no new service       │  │ as a release artifact        │
└──────────────────────────────┘  └──────────────────────────────┘
```

---

## 3. Division of Responsibilities

| Responsibility | Handled By | Why |
| :--- | :--- | :--- |
| **Code Reading & Intelligence** | Host Agent (Claude/Antigravity) | Host agents already have native codebase access and superior reasoning capabilities. |
| **User Prompts & Iteration** | Host Agent Chat UI | Eliminates the need to build and maintain terminal REPLs or custom chat UIs. |
| **Routes, env vars, and hashing** | `indoc` (`internal/scanner`, `internal/reference`) | Facts come from the source. The agent copies them and does not invent rows. |
| **UI Design & Asset Bundling** | `indoc` (`internal/bundler`) | Go's `//go:embed` packages the viewer into the binary and writes `./static/docs`. |
| **Offline Search & Rendering** | Client Browser (`app.js`) | Markdown is rendered at bundle time. Search and theme run in the browser. |
| **Using / distributing docs** | Project owner | Mount in an existing web app, publish as a static site, or ship the bundle; no docs-specific production server is required. |
| **Local preview** | Optional `indoc serve` command | Convenience for checking the bundle; not a deployment mechanism or product dependency. |

---

## 4. Agent Integration and Engine Contract

The CLI is the current interface to the Go engine, but it is not the desired end-user workflow. The target experience is one host-native plugin install, followed by a natural-language request such as "create documentation for this project." Users should not need to separately install the CLI, remember `indoc init`, or manually chain commands.

For Copilot CLI, the plugin should provide:

* A **skill** that makes the documentation workflow discoverable and guides the host agent through writing and updating the narrative.
* An **MCP server** that exposes the deterministic engine operations. Project setup should happen automatically when needed; `init` should not be a command the user must learn.
* A bundled or automatically provisioned engine so installing the plugin is sufficient. The plugin must not require a separate global `indoc` installation.

The host agent remains responsible for reading code, asking the user for preferences when needed, and writing the overview and guides. The engine remains responsible for deterministic scanning, validation, and bundling. Keep the CLI as a developer and power-user interface, but make the plugin the primary onboarding path. MCP is therefore part of the one-install product experience, not a feature to defer beyond that goal.

The plugin and MCP server are not implemented yet; the current workflow still depends on the CLI being installed and invoked by the host agent.

The document is the system, not a page per file:

* **Overview** and **guides** are prose the agent writes.
* **Reference** is facts the scanner extracts: HTTP routes and environment variables. The agent copies that array through unchanged.

### Current CLI operations

These are the existing engine operations. The future MCP layer should expose equivalent capabilities without requiring users to invoke the commands themselves.

#### `indoc scan`
* **Purpose:** Hash source files against `.indoc/cache.json`, extract the current reference, and return the last bundled document.
* **Output:** Added, modified, unchanged, and deleted files; `reference`; `stale_reference` for citations whose source file changed; `previous` overview and guides.

#### `indoc bundle`
* **Purpose:** Validate a document JSON file and write the portable static viewer.
* **Inputs:** `project_name`, `overview`, `guides[]`, `reference[]` (`kind` is `routes` or `config`).
* **Output:** `index.html`, `style.css`, `app.js`, and `docs.js` in the output directory. The normalized document is stored in the cache for the next scan.
* **Preview:** Optional `indoc serve` previews that directory on `127.0.0.1`. It is not required for deployment, including CLI-only projects.

---

## 5. Copilot Plugin Distribution

Copilot CLI plugins can be installed directly from a GitHub repository, a repository subdirectory, or a Git URL. A marketplace is a GitHub repository containing a `marketplace.json` catalog; Copilot CLI includes `github/awesome-copilot` by default. The current CLI documentation does not specify acceptance criteria for adding a plugin to that built-in marketplace.

The initial distribution path should therefore be a repository users can install directly, or an indoc-owned marketplace. Treat inclusion in `awesome-copilot` as a separate discovery opportunity, not a launch dependency or a guaranteed approval.

References:

* [Copilot CLI plugins](https://docs.github.com/copilot/concepts/agents/copilot-cli/about-cli-plugins)
* [Copilot CLI plugin marketplaces](https://docs.github.com/copilot/how-tos/copilot-cli/customize-copilot/plugins-marketplace)

---

## 6. Delivery Sequence

### One-install agent workflow

The initial release is not complete until a new user can install the host plugin once and ask their agent to generate useful docs without first learning indoc commands:

1. The user installs the Copilot CLI plugin from a GitHub repository or marketplace.
2. The user asks for documentation in natural language; the skill activates the workflow and gathers any needed preferences.
3. The plugin handles project setup and invokes the engine through MCP. The host agent writes the narrative while preserving scanner-provided reference facts.
4. The engine bundles the result and the agent reports where to preview or publish the static files.

The Go engine and its CLI are implemented. The one-install plugin, MCP server, automatic setup, and end-to-end host validation remain release work. Broader extraction and additional viewer features such as syntax highlighting can remain later improvements.

