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
| A cited symbol changed (signature or body) | **stale**: check fails |
| A signature was added, removed, or changed under `sources` | **stale**: check fails |
| Only implementation changed under `sources` | **drift**: reported, fails only with `--strict` |
| Comments or formatting | ignored |

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
| `indoc scan --json` | Pages, findings, and extracted reference facts, for your agent |
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

## Language support

Precise staleness and `path::Symbol` citations need a parser for the language. Every other part of indoc works on any repository.

| Language | Status |
|---|---|
| Python | ✅ Agrees with CPython's own parser on 409,406 symbols across 29,536 files |
| Go | ✅ Go standard library parser |
| Others | Files are tracked, but any change is reported as drift and citations cannot be verified. Planned: TypeScript/JavaScript, Rust, and more through a shared parser runtime. |

## Reference extraction

`indoc bundle` adds reference pages extracted from source, so routes and environment variables are never hand-copied.

| Source | Routes | Environment variables | Status |
|---|---|---|---|
| Flask/FastAPI-style decorators, Express `app.get`, Go `HandleFunc`/`GET` | ✅ | `os.getenv`, `os.environ`, `process.env`, `os.Getenv` | Best effort |
| Django + DRF | | | Planned next |
| FastAPI routers and settings, Cobra commands | | | Planned |

See [ROADMAP.md](ROADMAP.md) for what is next and what indoc deliberately does not do.

## License

See [LICENSE](LICENSE).
