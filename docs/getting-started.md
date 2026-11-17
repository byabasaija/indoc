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

For the command itself, download a binary from the GitHub releases page, or:

```sh
go install github.com/byabasaija/indoc/cmd/indoc@latest
```

## Initialize

From the repository root:

```sh
indoc init
```

`internal/cli/init.go::runInit` writes `.indoc/config.yaml`. The project name comes from `go.mod`, `pyproject.toml`, or `package.json` (`internal/config/config.go::InferProjectName`). Where pages live depends on the repository:

| Repository | `pages:` |
|---|---|
| Has a Docusaurus config | `docs/**/*.md` |
| No Markdown under `docs/` | `docs/**/*.md` |
| `docs/` already has Markdown | empty: nothing is published until you choose |

`init` also sets `source_url` from your git remote, so citations in the built site link to the code.

The last case in the table is deliberate. A `docs/` folder often holds internal notes, review findings, or plans next to user documentation, and none of it should be published by accident. `init` lists the files it found; pick the ones to publish:

```sh
indoc init --force --pages 'docs/partner-integration.md,docs/guides/**/*.md'
```

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
