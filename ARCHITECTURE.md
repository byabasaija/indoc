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

## 4. CLI Contract

The host agent invokes the binary directly. An MCP adapter can wrap these same commands later. The binary does not call a model.

The document is the system, not a page per file:

* **Overview** and **guides** are prose the agent writes.
* **Reference** is facts the scanner extracts: HTTP routes and environment variables. The agent copies that array through unchanged.

### `indoc scan`
* **Purpose:** Hash source files against `.indoc/cache.json`, extract the current reference, and return the last bundled document.
* **Output:** Added, modified, unchanged, and deleted files; `reference`; `stale_reference` for citations whose source file changed; `previous` overview and guides.

### `indoc bundle`
* **Purpose:** Validate a document JSON file and write the portable static viewer.
* **Inputs:** `project_name`, `overview`, `guides[]`, `reference[]` (`kind` is `routes` or `config`).
* **Output:** `index.html`, `style.css`, `app.js`, and `docs.js` in the output directory. The normalized document is stored in the cache for the next scan.
* **Preview:** Optional `indoc serve` previews that directory on `127.0.0.1`. It is not required for deployment, including CLI-only projects.

---

## 5. Delivery Sequence

### Minimum viable tool

First, make the end-to-end core useful and dependable:

1. A developer can initialize a project and scan it for changes and deterministic reference facts.
2. A host agent can use the scan output to create or update a concise overview and task-oriented guides without inventing reference entries.
3. `indoc bundle` validates that content and emits a polished, self-contained static docs site that can be used without a docs backend.
4. The artifact has a clear path for both web projects (optional mount in the existing app) and projects without a web app (publish or ship the files).

### Later improvements

Defer enhancements until that minimum workflow is complete and usable. Candidate follow-up work includes an MCP adapter, broader deterministic extraction (such as Django/Flask routes and command declarations), and additional viewer features such as syntax highlighting. These are not prerequisites for the initial tool.

