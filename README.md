# indoc 🚀

> **Automated, standalone, Docusaurus-styled documentation for any codebase — zero servers, zero Node.js.**

`indoc` is a single-binary CLI tool written in Go. Just like FastAPI generates interactive Swagger docs at runtime, `indoc` automatically inspects your codebase, uses an LLM (Bring Your Own API Key), and compiles a standalone, Docusaurus-styled documentation bundle directly into your project's static folder.

---

## ✨ Features

- **Zero-Server Static Output:** Generates a clean `./static/docs/index.html` (with bundled CSS/JS) that can be served as a static route in **Go, FastAPI, Django, Express, Rails, or Laravel**, or opened directly in a browser.
- **Docusaurus / Stripe Docs Aesthetics:** Clean sidebar navigation, reading pane, right-side table of contents, and dark/light mode toggle.
- **Offline Full-Text Search:** In-memory, instant client-side search with zero external network calls.
- **Incremental Caching:** Computes SHA-256 hashes of your source files against `.indoc/cache.json`. Only changed files trigger LLM API calls, saving you tokens and money.
- **BYO API Key:** Supports **OpenAI**, **Anthropic**, and **Ollama** (for local, air-gapped codebases).
- **Zero Node.js Dependency:** Shipped as a single compiled Go binary with UI assets embedded via `go:embed`.

---

## 🏗️ Architecture

```
┌─────────────────────────────────────────────────────────────┐
│ 1. SCAN & AST PARSE                                         │
│ Crawls source files (.go, .py, .js, .ts, .sql, etc.)        │
│ Computes SHA-256 hashes against local `.indoc/cache.json`   │
└──────────────────────────────┬──────────────────────────────┘
                               │ (Only modified files)
                               ▼
┌─────────────────────────────────────────────────────────────┐
│ 2. LLM SYNTHESIS (BYO API KEY)                              │
│ Sends code signatures to provider (OpenAI / Claude /        │
│ Ollama) with structured prompt                              │
└──────────────────────────────┬──────────────────────────────┘
                               │ (Returns Markdown / JSON)
                               ▼
┌─────────────────────────────────────────────────────────────┐
│ 3. BUNDLE & INJECT (Zero Server)                            │
│ Injects Markdown & Search Index into pre-built UI           │
│ using Go's `embed` package into target directory            │
└─────────────────────────────────────────────────────────────┘
```

---

## 🚀 Quickstart

### 1. Set your API Key
```bash
export OPENAI_API_KEY="sk-..."
```

### 2. Initialize Configuration
```bash
indoc init
```
This creates a `.indoc.yaml` configuration file in your project root.

### 3. Generate Documentation
```bash
indoc generate --source="./backend" --out="./static/docs"
```

---

## ⚙️ Configuration (`.indoc.yaml`)

```yaml
version: "1"
project_name: "My Awesome Project"
source_dirs:
  - "./backend/controllers"
  - "./backend/models"
output_dir: "./static/docs"
provider: "openai" # Options: openai, anthropic, ollama
model: "gpt-4o-mini"
```

---

## 🗺️ Project Layout

```
indoc/
├── cmd/
│   └── indoc/
│       └── main.go           # CLI Entrypoint (Cobra)
├── internal/
│   ├── config/               # Configuration parsing (.indoc.yaml, flags, env vars)
│   ├── scanner/              # File crawler, hasher, and AST parser
│   ├── llm/                  # BYO API Key LLM provider abstraction
│   ├── generator/            # Markdown & JSON Manifest compiler
│   └── bundler/              # Single-file / Static Asset embedder (go:embed)
├── ui/                       # Docusaurus-style UI Template
├── go.mod
└── go.sum
```

---

## 📄 License

This project is licensed under the [MIT License](LICENSE).

