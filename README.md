# indoc

`indoc` compiles a documentation site into the application you already ship. The editor agent writes the overview and the guides. The binary extracts routes and environment variables from the source, then writes a static site to `./static/docs`.

No API key. No second host. Mount the folder from FastAPI, Express, Django, or Go.

## Commands

```bash
go install github.com/byabasaija/indoc/cmd/indoc@latest

indoc init
indoc scan --json
indoc bundle --input .indoc/site.json --out ./static/docs
indoc serve
```

`scan` prints added, modified, and deleted files, a fresh reference (routes and env vars), and the previous overview and guides so the agent can edit them. `bundle` writes `index.html`, `style.css`, `app.js`, and `docs.js`. `serve` previews that directory at `http://127.0.0.1:4173`.

An example document lives at [`examples/site.json`](examples/site.json).

## What gets written

The document has three parts:

- **Overview.** What the system does and how a request moves through it.
- **Guides.** Tasks a reader came to finish: run it, authenticate, the one flow that is hard to infer. A few pages, not one page per file.
- **Reference.** Routes and environment variables copied from `indoc scan`. The agent does not invent these.

## Mount it

FastAPI:

```python
from fastapi.staticfiles import StaticFiles

app.mount("/docs", StaticFiles(directory="static/docs", html=True), name="docs")
```

Express: `app.use("/docs", express.static("static/docs"))`

Go: `http.Handle("/docs/", http.StripPrefix("/docs/", http.FileServer(http.Dir("static/docs"))))`

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
internal/cli/         init, scan, bundle, serve
```
