# indoc

**Documentation that fails CI when it stops matching the code.**

Docs rot quietly. A function gets a new parameter, a route moves, and the guide that explains them keeps saying the old thing until a user hits it. indoc ties each Markdown page to the code it describes and tells you, in CI, when that code changes and the page does not.

```
$ indoc check
documentation/dev/checkout-routing.md
  ✗ STALE      shop/core/services/checkout/dispatcher.py::CheckoutDispatcher.dispatch  cited symbol's signature changed
    DRIFT      shop/core/services/checkout/session.py  implementation changed; signatures unchanged

6 pages checked: 1 stale, 1 drift
Error: docs do not match the code
```

Your coding agent writes the pages. indoc never calls an AI model and needs no API key.

## How it works

A page lists the code it covers and cites the symbols it explains:

```markdown
---
title: How checkout sessions are routed
sources:
  - shop/core/services/checkout/**
---

Every request lands in `shop/core/services/checkout/dispatcher.py::CheckoutDispatcher.dispatch`,
which picks a handler from the merchant's payment provider.
```

`indoc ack` records what each page was verified against in `.indoc/lock.json`. `indoc check` compares the current code with that record:

| Change | Result |
|---|---|
| A cited symbol's signature changed, or the symbol is gone | **stale** / **broken**: check fails |
| A public signature was added, removed, or changed under `sources` | **stale**: check fails |
| A cited symbol's body changed | **drift**: reported; fails on `strict: true` pages or with `--strict` |
| Only implementation, private helpers, or tests changed under `sources` | **drift**: reported; fails with `--strict` |
| Comments, docstrings, logging calls, formatting | ignored |

Pages that describe externally visible behavior (a partner API, a CLI's output) can set `strict: true`; then any change to a cited symbol fails the check.

Citations are verified: a renamed function breaks the citation, and the build refuses to publish it.

## Install

### In your coding agent (recommended)

Install the plugin once, then ask your agent to "document this project". It sets indoc up, agrees on readers and an outline with you, writes the pages, and builds the site. Later, "update the stale docs" fixes what changed. The plugin downloads the indoc binary for your platform on first use and verifies its checksum; nothing else needs installing.

**Claude Code**

```
/plugin marketplace add byabasaija/indoc
/plugin install indoc@indoc
```

**GitHub Copilot CLI**

```
copilot plugin install byabasaija/indoc:plugin
```

The plugin provides an MCP server with five tools (`indoc_scan`, `indoc_setup`, `indoc_check`, `indoc_ack`, `indoc_bundle`) and a skill that tells the agent how to use them. Any other MCP client can run `indoc mcp` directly.

### In CI

```yaml
  docs:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: byabasaija/indoc@v0.2.0
        # with:
        #   mode: warn   # report without failing while you try it out
```

### As a command

Download a binary from [Releases](https://github.com/byabasaija/indoc/releases), or:

```sh
go install github.com/byabasaija/indoc/cmd/indoc@latest
```

## Use it

```sh
indoc init      # create .indoc/config.yaml
indoc bundle    # build the site, record pages in the lock
indoc check     # in CI: fail when docs no longer match the code
```

| Command | Does |
|---|---|
| `indoc init` | Create `.indoc/config.yaml`. Publishing is opt-in: existing Markdown is listed, not published. |
| `indoc scan --json` | For your agent: pages, findings, extracted reference facts, and a map of the project (frameworks, entry points, task runner commands, unpublished docs, most-changed files) |
| `indoc check` | Fail when pages no longer match the code. `--warn` to report only, `--strict` to fail on drift too. |
| `indoc ack [page...]` | Record new or edited pages. Name a page to confirm it is still correct after a code change. |
| `indoc bundle` | Build the static site |
| `indoc preview` | Serve the site on localhost and rebuild on save |
| `indoc mcp` | Run the MCP server on stdio |

### Already using Docusaurus or MkDocs?

Keep it. Add `sources:` and citations to your existing pages and run `indoc check` in CI. indoc reads Docusaurus frontmatter (`sidebar_label`, `sidebar_position`). Building the site with indoc is optional.

## Files

```
.indoc/
├── config.yaml   committed
├── lock.json     committed: what each page was verified against
├── .gitignore
└── site/         build output, gitignored
documentation/    your pages (set by `pages:`; not docs/, where plans and notes live)
```

To serve the site from an existing app, set `output_dir` to a folder the app serves. The site uses relative links, so it works under any path.

`indoc init` reads your git remote and sets `source_url` (GitHub, GitLab, Bitbucket), so every citation and reference entry in the site links to the exact line in your repository. For another host, set the template yourself:

```yaml
source_url: https://git.example.com/team/app/src/branch/main/{path}#L{line}
```

The site is one pre-rendered HTML page per Markdown file, with search, dark mode, syntax highlighting, copy buttons, and Mermaid diagrams. The sidebar follows your folders. A landing page, logo, colors, header links, footer, `custom.css`, and `head.html` are set in config and frontmatter; see [documentation/site.md](documentation/site.md).

## Language support

Precise staleness and `path::Symbol` citations need a parser for the language. Every other part of indoc works on any repository.

| Language | Parser | Checked against |
|---|---|---|
| JavaScript, TypeScript, JSX, TSX | tree-sitter | The TypeScript compiler: 9,436 files, 56,396 symbols, all matching |
| Python | tree-sitter | CPython's own parser: 34,120 of 34,121 files, 454,837 symbols |
| Go | Go standard library | — |
| Others | — | Files are tracked, but any change is reported as drift and citations cannot be verified |

In JavaScript and TypeScript, exported declarations are public and everything else is private, so adding an internal helper does not fail `check`. Python uses `_name`, Go uses lowercase names.

## Reference extraction

`indoc bundle` adds reference pages extracted from source, so routes, environment variables, and commands are never hand-copied. Extraction is static: indoc never imports or runs your code.

| Framework | Routes | Configuration | Commands | Status |
|---|---|---|---|---|
| Django + DRF | ✅ URLconf through `include()`, routers, `@action`; HTTP methods from views, viewsets, `@api_view`, `http_method_names` | ✅ django-environ, python-decouple, `os.environ` | ✅ management commands | Supported |
| FastAPI | ✅ decorators, `APIRouter` prefixes, nested `include_router`, `add_api_route`, websockets, mounts, built-in docs routes | ✅ pydantic `BaseSettings` (`env_prefix`, aliases) | | Supported |
| Express | ✅ routes across files, `route()` chains, `use()` mounts of routers and sub-apps | `process.env` | | Supported |
| Cobra (Go) | | `os.Getenv` | ✅ command tree, flags, arguments | Supported |
| Python (any) | | ✅ `os.environ`, `os.getenv`, pydantic settings | | Supported |
| Go `HandleFunc`/`GET`, Flask-style decorators | best effort | | | Best effort |
| Next.js | ✅ App Router pages and route handlers (methods from exports), Pages Router pages and API routes | `process.env` | | Supported |
| NestJS | ✅ `@Controller` + `@Get`/`@Post`/..., global prefix | `process.env` | | Supported |
| Astro | ✅ `src/pages` pages and endpoints (methods from exports) | `import.meta.env` | | Supported |
| Flask blueprints, Rails, Laravel | | | | Planned |

Extraction is checked against each framework's own route table: Django's URL resolver on six real projects (524 routes), FastAPI's router on a real project and a fixture covering every supported construct, Express 4 and 5 by recording registrations at runtime, Next.js against `next build`'s route manifests, NestJS against the routes Nest registers, and Cobra's command tree on indoc's own CLI. All match route for route, including HTTP methods. Routes provided by installed packages (admin, allauth) are listed as external includes rather than guessed.

Every extractor reports what it could not resolve, and `indoc scan` shows it. It also warns about things worth fixing: a route that can never be reached because an earlier pattern has the same path, and variables missing from `.env.example`. Default values that may be secrets (names containing `SECRET`, `PASSWORD`, `TOKEN`, `KEY`, ...) and credentials inside URLs are never published.

See [ROADMAP.md](ROADMAP.md) for what is next and what indoc deliberately does not do.

## License

MIT. See [LICENSE](LICENSE).
