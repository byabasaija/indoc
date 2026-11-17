# indoc

**Documentation that fails CI when it stops matching the code.**

Docs rot quietly. A function gets a new parameter, a route moves, and the guide that explains them keeps saying the old thing until a user hits it. indoc ties each Markdown page to the code it describes and tells you, in CI, when that code changes and the page does not.

```
$ indoc check
docs/dev/checkout-routing.md
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

```sh
go install github.com/byabasaija/indoc/cmd/indoc@latest
```

## Use it

```sh
indoc init      # create .indoc/config.yaml
                # ask your agent to document the project (see below)
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

### With a coding agent

The skill in [`.github/skills/indoc/SKILL.md`](.github/skills/indoc/SKILL.md) tells an agent how to work with indoc: agree on readers and an outline with you, write pages with citations, and fix stale pages after code changes. Ask it to "document this project" or "update the stale docs".

### Already using Docusaurus or MkDocs?

Keep it. Add `sources:` and citations to your existing pages and run `indoc check` in CI. indoc reads Docusaurus frontmatter (`sidebar_label`, `sidebar_position`). Building the site with indoc is optional.

## Files

```
.indoc/
├── config.yaml   committed
├── lock.json     committed: what each page was verified against
├── .gitignore
└── site/         build output, gitignored
docs/             your pages (location set by `pages:` in config.yaml)
```

To serve the site from an existing app, set `output_dir` to a folder the app serves. The site uses relative links, so it works under any path.

`indoc init` reads your git remote and sets `source_url` (GitHub, GitLab, Bitbucket), so every citation and reference entry in the site links to the exact line in your repository. For another host, set the template yourself:

```yaml
source_url: https://git.example.com/team/app/src/branch/main/{path}#L{line}
```

## Language support

Precise staleness and `path::Symbol` citations need a parser for the language. Every other part of indoc works on any repository.

| Language | Status |
|---|---|
| Python | ✅ Agrees with CPython's own parser on 409,406 symbols across 29,536 files |
| Go | ✅ Go standard library parser |
| Others | Files are tracked, but any change is reported as drift and citations cannot be verified. Planned: TypeScript/JavaScript, Rust, and more through a shared parser runtime. |

## Reference extraction

`indoc bundle` adds reference pages extracted from source, so routes, environment variables, and commands are never hand-copied. Extraction is static: indoc never imports or runs your code.

| Framework | Routes | Configuration | Commands | Status |
|---|---|---|---|---|
| Django + DRF | ✅ URLconf through `include()`, routers, `@action`; HTTP methods from views, viewsets, `@api_view`, `http_method_names` | ✅ django-environ, python-decouple, `os.environ` | ✅ management commands | Supported |
| FastAPI | ✅ decorators, `APIRouter` prefixes, nested `include_router`, `add_api_route`, websockets, mounts, built-in docs routes | ✅ pydantic `BaseSettings` (`env_prefix`, aliases) | | Supported |
| Cobra (Go) | | `os.Getenv` | ✅ command tree, flags, arguments | Supported |
| Python (any) | | ✅ `os.environ`, `os.getenv`, pydantic settings | | Supported |
| Express `app.get`, Go `HandleFunc`/`GET`, Flask-style decorators | best effort | `process.env` | | Best effort |
| Flask blueprints, Next.js, NestJS, Rails, Laravel | | | | Planned |

Extraction is checked against each framework's own route table: Django's URL resolver on six real projects (523 routes), FastAPI's router on a real project and a fixture covering every supported construct, and Cobra's command tree on indoc's own CLI. All match route for route, including HTTP methods. Routes provided by installed packages (admin, allauth) are listed as external includes rather than guessed.

Every extractor reports what it could not resolve, and `indoc scan` shows it. It also warns about things worth fixing: a route that can never be reached because an earlier pattern has the same path, and variables missing from `.env.example`. Default values that may be secrets (names containing `SECRET`, `PASSWORD`, `TOKEN`, `KEY`, ...) and credentials inside URLs are never published.

See [ROADMAP.md](ROADMAP.md) for what is next and what indoc deliberately does not do.

## License

See [LICENSE](LICENSE).
