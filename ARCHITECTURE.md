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
│  1. `indoc scan`:                                               │
│     SHA-256 diff, extracted routes and env vars, and the last   │
│     overview and guides so the agent can patch them.            │
│                                                                 │
│  2. `indoc bundle` / `indoc serve`:                             │
│     Validates overview, guides, and reference, then writes the  │
│     static viewer. `serve` previews it on localhost.            │
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
| **Routes, env vars, and hashing** | `indoc` (`internal/scanner`, `internal/reference`) | Facts come from the source. The agent copies them and does not invent rows. |
| **UI Design & Asset Bundling** | `indoc` (`internal/bundler`) | Go's `//go:embed` packages the viewer into the binary and writes `./static/docs`. |
| **Offline Search & Rendering** | Client Browser (`app.js`) | Markdown is rendered at bundle time. Search and theme run in the browser. |
| **Serving the Docs** | User's Web Application | No secondary servers; documentation lives on the existing web host. |

---

## 4. CLI Contract

The host agent invokes the binary directly. An MCP adapter can wrap these same commands later. The binary does not call a model.

The document is the system, not a page per file:

* **Overview** and **guides** are prose the agent writes.
* **Reference** is facts the scanner extracts: HTTP routes and environment variables. The agent copies that array through unchanged.

### `indoc scan`
* **Purpose:** Hash source files against `.indoc/cache.json`, extract the current reference, and return the last bundled document.
* **Output:** Added, modified, unchanged, and deleted files; `reference`; `stale_reference` for citations whose source file changed; `previous` overview and guides.

### `indoc bundle`
* **Purpose:** Validate a document JSON file and write the static viewer.
* **Inputs:** `project_name`, `overview`, `guides[]`, `reference[]` (`kind` is `routes` or `config`).
* **Output:** `index.html`, `style.css`, `app.js`, and `docs.js` in the output directory. The normalized document is stored in the cache for the next scan.
* **Preview:** `indoc serve` hosts that directory on `127.0.0.1`.

---

## 5. Next Implementation Priorities

1. **MCP adapter** over `scan` and `bundle`, so the host can call them without a shell.
2. **Broader deterministic extraction** (Django/Flask paths, command declarations) with the same rule: the model does not invent reference rows.
3. **Syntax highlighting** in the embedded viewer. Search, theme, sidebar, and table of contents already run from the static files.

