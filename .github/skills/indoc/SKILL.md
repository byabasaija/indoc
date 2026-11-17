---
name: indoc
description: Write, update, or check a repository's documentation with indoc. Use when asked to document a project, refresh docs after code changes, fix failing `indoc check` results, or build the docs site.
---

# Documentation with indoc

indoc keeps Markdown pages tied to code. Pages list the code they cover (`sources:`) and cite symbols as `path::Symbol`; `indoc check` fails when that code changes and the page does not. You write the pages. indoc never calls a model.

## 1. Set up

1. Check that `indoc` is available. If not, tell the user to run `go install github.com/byabasaija/indoc/cmd/indoc@latest`; do not install software without approval.
2. If `.indoc/config.yaml` is missing, run `indoc init`.
   - If init lists existing Markdown files and publishes nothing, **ask the user which files to publish** before continuing. Internal notes, reviews, and plans must not be published. Then run `indoc init --force --pages '<globs>'`.
   - Never pass `--force` when a config already exists unless the user asked to replace it.

## 2. Understand the project

Run `indoc scan --json`. It returns the pages, every check finding with its reason, and reference facts (routes, environment variables) extracted from source. Then read the code: entry points, the main request or command flow, configuration, and any existing docs.

## 3. Agree on an outline before writing new docs

When there are no pages yet, or the user asks for new sections, **propose the readers and an outline and wait for approval.** Base it on evidence, not a template:

- Who uses this code? A public or partner API, a CLI, a library, a deployed service, internal developers?
- Do existing docs already target an audience (for example a partner integration guide)?
- What would each reader come to do? Install, authenticate, call an endpoint, add a provider, deploy?

Example: "Two readers. Partner integrators: authentication, endpoints, webhooks. Your developers: setup, checkout routing, adding a banking provider. OK?"

## 4. Write pages

Each page is Markdown under the configured `pages:` globs:

```markdown
---
title: How checkout sessions are routed
order: 2
audience: dev
sources:
  - shop/core/services/checkout/**
---
```

- **Cite the code you describe** as `path::Symbol` (`app/checkout/dispatcher.py::Dispatcher.dispatch`; methods and nested names use dots). Every specific claim about behavior should cite the symbol that implements it. Citations are verified and anchor the page.
- Set `sources:` to the directories or files the page explains, as narrowly as accurate.
- Use real commands from the project's task runner (`justfile`, `Makefile`, `package.json` scripts) and real request/response or CLI examples.
- Do not write route or environment-variable tables. indoc generates them from source.
- No page per source file. Write the pages a reader needs.
- `index.md` at the top of the docs folder is the overview.

## 5. Update docs after code changes

For each finding from `indoc scan --json` or `indoc check`:

- **stale**: read the changed code (the finding names the file or symbol and what changed), then update the page. If after reading you are certain the page is still correct, run `indoc ack <page>`. Never ack a page you have not re-read against the code.
- **broken**: a citation or glob no longer resolves (often a rename). Fix it.
- **drift**: implementation changed under the page's sources. Review whether the prose is affected.
- **edited / unstamped / removed**: run `indoc ack` (no arguments) after reviewing.

## 6. Build and verify

1. Run `indoc bundle`. Fix any broken citations it reports and rerun.
2. Run `indoc check`. It must pass.
3. Tell the user where the site was written (`output_dir`, default `.indoc/site/`), that `indoc preview` serves it locally, and that `.indoc/config.yaml`, `.indoc/lock.json`, and the pages should be committed. Do not start `indoc preview` unless asked; it runs until stopped.
