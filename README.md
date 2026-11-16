# indoc

`indoc` compiles a portable, static documentation site from your codebase. Your coding agent writes the overview and guides; `indoc` extracts source facts such as routes and environment variables, then bundles everything into `./static/docs`.

No API key or docs-specific runtime service. The output works for web apps and CLI-only projects: mount it in your app if you have one, publish it to any static host, or ship it as a release artifact.

## Commands

```bash
go install github.com/byabasaija/indoc/cmd/indoc@latest

indoc init
indoc scan --json
indoc bundle --input .indoc/site.json --out ./static/docs
```

`indoc init` creates `.indoc.yaml` plus a starter document, JSON schema, and agent workflow guide under `.indoc/`. Ask your coding agent to read `.indoc/AGENT_GUIDE.md` and follow it. The guide explains how to use scan output to create or update `.indoc/site.json` and bundle it.

`scan` prints added, modified, and deleted files, a fresh reference (routes and env vars), and the previous overview and guides so the agent can edit them. `bundle` writes `index.html`, `style.css`, `app.js`, and `docs.js`. Optionally run `indoc serve` to preview the bundle locally at `http://127.0.0.1:4173`; it is not needed to build, publish, or use the docs.

## What gets written

The document has three parts:

- **Overview.** What the system does and how a request moves through it.
- **Guides.** Tasks a reader came to finish: run it, authenticate, the one flow that is hard to infer. A few pages, not one page per file.
- **Reference.** Routes and environment variables copied from `indoc scan`. The agent does not invent these.

## Use or publish the bundle

The generated site is static and self-contained. For a web app, serve the output directory from the app you already run. For example, FastAPI:

```python
from fastapi.staticfiles import StaticFiles

app.mount("/docs", StaticFiles(directory="static/docs", html=True), name="docs")
```

Express: `app.use("/docs", express.static("static/docs"))`

Go: `http.Handle("/docs/", http.StripPrefix("/docs/", http.FileServer(http.Dir("static/docs"))))`

For a CLI-only project, no application server is needed. Publish `./static/docs` with a static hosting provider, attach it to a release, or keep it as a local docs artifact. `indoc serve` is only a convenient local preview.

## Configuration

`indoc init` writes `.indoc.yaml`:

```yaml
version: "1"
project_name: "Hospital API"
source_dirs:
  - "."
output_dir: "./static/docs"
exclude:
  - node_modules
  - vendor
  - .git
  - .indoc
  - dist
  - build
```

Hashes and the last bundled document are stored in `.indoc/cache.json`.

## Layout

```
cmd/indoc/            CLI entrypoint
internal/scanner/     SHA-256 incremental scan
internal/reference/   route and env extraction
internal/doc/         overview, guides, reference
internal/bundler/     embedded viewer and site writer
internal/cli/         init, scan, bundle, optional serve preview
```
