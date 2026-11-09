# Architecture Decision Record (ADR) & System Specification

**Status:** Approved  
**Date:** November 2026  
**Subject:** Pivot from Standalone BYO-API CLI to Agent Plugin & MCP-First Static Documentation Compiler  

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
3. **Zero-Server Principle:** `indoc` docs must have **zero external servers**. The generated bundle must be pure static assets (`./static/docs`) directly mounted and served by the developer's existing application (FastAPI, Django, Express, Go, Laravel).

---

## 2. The Decision: Agent Plugin & MCP-First Architecture

We are decoupling the **Intelligence Layer** from the **Compiler & Bundler Layer**:

* **The Host Agent (Claude / Antigravity / Cursor):** Acts as the brain. It reads source code, reasons through dependencies, writes Markdown, generates Mermaid diagrams, and handles conversational refinement using the developer's existing subscription.
* **`indoc` (The Go Engine):** Acts as the static compiler, incremental cache, and asset bundler. It exposes tools via the **Model Context Protocol (MCP)** and compiles a standalone, Docusaurus-styled static SPA directly into the user's project.

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
│  1. `indoc_scan`:                                               │
│     Fast SHA-256 crawler. Tells the agent which files are added,│
│     modified, or unchanged since last run.                      │
│                                                                 │
│  2. `indoc_bundle`:                                             │
│     Takes the agent's synthesized documentation, validates it,  │
│     and injects it into the embedded Docusaurus UI.             │
└────────────────────────────────┬────────────────────────────────┘
                                 │
                                 ▼ (Writes static assets)
┌─────────────────────────────────────────────────────────────────┐
│             TARGET OUTPUT DIRECTORY (`./static/docs`)           │
│                                                                 │
│  ./static/docs/                                                 │
│  ├── index.html   (Single-Page App shell)                       │
│  ├── style.css    (Tailwind/Docusaurus typography)              │
│  ├── app.js       (Offline search engine & Markdown parser)     │
│  └── docs.js      (Self-contained documentation payload)        │
└────────────────────────────────┬────────────────────────────────┘
                                 │
                                 ▼ (Mounted directly by user's app)
┌─────────────────────────────────────────────────────────────────┐
│             USER'S EXISTING BACKEND SERVER                      │
│                                                                 │
│  • FastAPI:   app.mount("/docs", StaticFiles(directory="static/docs"))
│  • Go:        http.Handle("/docs/", http.FileServer(http.Dir("static/docs")))
│  • Django:    django.views.static.serve                         │
│  • Express:   app.use("/docs", express.static("static/docs"))   │
└─────────────────────────────────────────────────────────────────┘
```

---

## 3. Division of Responsibilities

| Responsibility | Handled By | Why |
| :--- | :--- | :--- |
| **Code Reading & Intelligence** | Host Agent (Claude/Antigravity) | Host agents already have native codebase access and superior reasoning capabilities. |
| **User Prompts & Iteration** | Host Agent Chat UI | Eliminates the need to build and maintain terminal REPLs or custom chat UIs. |
| **Incremental SHA-256 Hashing** | `indoc` (`internal/scanner`) | Go streams file hashes with $O(1)$ memory, preventing redundant work. |
| **UI Design & Asset Bundling** | `indoc` (`internal/bundler`) | Go's `//go:embed` packages the entire Docusaurus SPA into a single compiled binary. |
| **Offline Search & Rendering** | Client Browser (`app.js`) | Search runs in the user's browser memory (MiniSearch)—zero network latency, zero servers. |
| **Serving the Docs** | User's Web Application | No secondary servers; documentation lives on the existing web host. |

---

## 4. MCP Tool Specifications

When `indoc` runs as an MCP server (`indoc mcp`), it exposes two primary tools to the host agent:

### Tool 1: `indoc_scan`
* **Purpose:** Inspects source directories, checks `.indoc/cache.json`, and returns diffs.
* **Output:** List of added, modified, unchanged, and deleted source files.
* **Benefit:** The agent immediately knows which files need documentation without reading the entire repository.

### Tool 2: `indoc_bundle`
* **Purpose:** Accepts the agent's documentation payload and writes the complete static viewer.
* **Inputs:**
  * `project_name`: Name of the application.
  * `output_dir`: Target path (e.g., `./static/docs`).
  * `modules`: Array of `{ title, file_path, summary, content_markdown, routes }`.
  * `overview`: High-level system overview Markdown with Mermaid architecture diagrams.
* **Output:** Writes `index.html`, `style.css`, `app.js`, and `docs.js` into `output_dir`.

---

## 5. Next Implementation Priorities

With this decision recorded, our engineering roadmap focuses purely on:
1. **The Docusaurus-Style UI Shell (`ui/`):**
   * Responsive layout (Left sidebar, Center reader, Right Table of Contents).
   * Client-side Markdown rendering and syntax highlighting.
   * Offline full-text search with MiniSearch.
   * Dark/Light mode theme persistence.
2. **The Bundler (`internal/bundler/`):**
   * Embedding `ui/` files into Go via `//go:embed`.
   * Exporting assets and writing `docs.js` into the target directory.
3. **The MCP Server Adapter:**
   * Implementing JSON-RPC stdio handler exposing `indoc_scan` and `indoc_bundle`.
