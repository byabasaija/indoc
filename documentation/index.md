---
title: Overview
layout: landing
description: Documentation that fails CI when it stops matching the code.
hero:
  title: indoc
  tagline: Documentation that fails CI when it stops matching the code.
  actions:
    - text: Get started
      link: getting-started.md
    - text: Use it from your agent
      link: agents.md
features:
  - title: Docs that can't silently rot
    details: Pages cite code as `path::Symbol`. Change a cited signature and `indoc check` fails in CI, naming the page and what changed.
    link: ci.md
  - title: Reference from the source
    details: Routes, environment variables, and commands are extracted from Django, FastAPI, Express, Next.js, NestJS, Astro, and Cobra. Nobody copies them by hand.
    link: reference.md
  - title: Your agent writes, indoc checks
    details: Install the plugin for Claude Code or Copilot CLI and ask for docs. No API keys; indoc never calls a model.
    link: agents.md
  - title: A static site, anywhere
    details: One HTML file per page, relative links, no server. Mount it in your app, publish it, or open it from disk.
    link: getting-started.md
sections:
  - hero
  - features
  - type: contributors
    title: Built by
  - body
featured:
  - text: Writing pages
    link: writing-pages.md
  - text: Checking docs in CI
    link: ci.md
sources:
  - internal/drift/**
  - cmd/indoc/**
---

## What indoc does

indoc keeps Markdown documentation tied to the code it describes, and fails CI when the two drift apart.

Every page lists the code it covers (`sources:` globs in frontmatter) and cites specific symbols as `path::Symbol`. indoc records what each page was verified against in `.indoc/lock.json`. When a cited function or a covered signature changes and the page does not, `indoc check` exits 1.

indoc never calls an AI model. Your coding agent writes the pages; indoc does the parts that must be exact.

## How a check works

1. `internal/drift/drift.go::Check` loads each page and compares it with its entry in the lock.
2. Each covered file is analyzed by `internal/code/code.go::Analyze`, which uses a language parser where one exists (JavaScript, TypeScript, Python, Go) to hash every symbol's signature and body separately, ignoring comments and formatting.
3. Differences are classified by `internal/drift/drift.go::Finding.Fails`:

| Change | Result |
|---|---|
| A cited symbol's signature changed, or the symbol is gone | **stale** / **broken**: check fails |
| A public signature was added, removed, or changed under `sources` | **stale**: check fails |
| A cited symbol's body changed | **drift**: reported; fails on `strict: true` pages or with `--strict` |
| Only implementation, private helpers, or tests changed under `sources` | **drift**: reported; fails with `--strict` |
| Comments, docstrings, logging calls, formatting | ignored |

A page that was edited must be recorded again with `indoc ack` (or `indoc bundle`). Nothing re-records an unchanged stale page except naming it explicitly, so no routine command can hide a real warning.

## Two ways to use it

- **Check only.** Keep Docusaurus, MkDocs, or plain Markdown. Add `sources:` and citations to your pages and run `indoc check` in CI.
- **Full.** indoc also builds a static site with reference pages (routes, environment variables) extracted from source.

## Commands

| Command | Does |
|---|---|
| `indoc init` | Create `.indoc/config.yaml` |
| `indoc check` | Fail when pages no longer match the code (CI) |
| `indoc ack [page...]` | Record pages as verified |
| `indoc scan --json` | Report pages, findings, and reference facts for an agent |
| `indoc bundle` | Build the static site |
| `indoc preview` | Serve the site locally and rebuild on save |
