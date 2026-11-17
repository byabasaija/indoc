---
title: Getting started
order: 1
sources:
  - internal/cli/init.go
  - internal/cli/bundle.go
  - internal/config/**
---

# Getting started

## Install

The easiest way is the plugin for your coding agent, which needs nothing else installed; see [Using indoc from a coding agent](agents.md). The agent then runs the steps below for you.

For the command itself, on macOS or Linux:

```sh
curl -fsSL https://github.com/byabasaija/indoc/releases/latest/download/install.sh | sh
```

It installs to `/usr/local/bin` when writable, otherwise `~/.local/bin`, after verifying the download against the release's checksums. Pin a version with `| sh -s -- v0.3.0`, or choose the folder with `INDOC_INSTALL_DIR`. Alternatively, `go install github.com/byabasaija/indoc/cmd/indoc@latest`, or download a binary (including Windows) from the GitHub releases page.

## Initialize

From the repository root:

```sh
indoc init
```

`internal/cli/init.go::runInit` writes `.indoc/config.yaml`. The project name comes from `go.mod`, `pyproject.toml`, or `package.json` (`internal/config/config.go::InferProjectName`). Where pages live depends on the repository:

| Repository | `pages:` |
|---|---|
| Has a Docusaurus config | `docs/**/*.md` |
| Anything else | `documentation/**/*.md` |
| `documentation/` already has Markdown | empty: nothing is published until you choose |

`init` also sets `source_url` from your git remote, so citations in the built site link to the code.

Pages do not go in `docs/` unless the project is a Docusaurus site. In most repositories `docs/` collects plans, review findings, and notes written by coding agents, and none of it should be published by accident. `init` lists the Markdown it did not publish; to publish some of it as well, add it to `pages:`:

```sh
indoc init --force --pages 'documentation/**/*.md,docs/partner-integration.md'
```

The last row of the table is deliberate too: Markdown that is already there was not written for indoc, so you choose what to publish.

## Write pages

Ask your coding agent to document the project. It runs `indoc scan --json`, writes Markdown pages with `sources:` and `path::Symbol` citations, and builds the site. See [Writing pages](writing-pages.md) for the format.

## Build

```sh
indoc bundle
```

`internal/cli/bundle.go::runBundle` refuses to build if any citation or `sources` glob is broken, extracts reference facts from source, writes the site to `output_dir` (default `.indoc/site/`, gitignored), and records new or edited pages in `.indoc/lock.json`.

Preview it with `indoc preview`, which serves on `http://127.0.0.1:4173` and rebuilds when a page changes.

## Commit

Commit `.indoc/config.yaml`, `.indoc/lock.json`, and your pages. The lock is what `indoc check` compares against in CI.
