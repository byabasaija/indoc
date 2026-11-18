---
name: indoc
description: Write, update, or check a repository's documentation with indoc. Use when asked to document a project, refresh docs after code changes, fix failing indoc checks, or build the docs site.
---

# Documentation with indoc

indoc keeps Markdown pages tied to code. Pages list the code they cover (`sources:`) and cite symbols as `path::Symbol`; indoc fails the check when that code changes and the page does not. You write the pages. indoc never calls a model.

Use the indoc MCP tools: `indoc_scan`, `indoc_setup`, `indoc_check`, `indoc_ack`, `indoc_bundle`. The same operations exist as the `indoc` command.

## 1. Scan

Call `indoc_scan`. On first use it sets the project up.

- New pages go where the `pages:` globs point: `documentation/` by default, `docs/` in a Docusaurus project. Do not put them in `docs/` otherwise; that folder usually holds plans, reviews, and agent notes, which must not be published.
- If it says existing Markdown was found and nothing is published, **ask the user which files to publish**. Internal notes, code reviews, and plans must not be published. Then call `indoc_setup` with their globs (for example `["documentation/**/*.md", "docs/partner-integration.md"]`).
- Otherwise it returns `pages`, `findings`, `reference` (routes, environment variables, commands extracted from source), `coverage` (what each extractor detected and could not resolve), and `map`:
  - `frameworks`, `entry_points`, `languages` (`parsed` languages support `path::Symbol` citations)
  - `tasks`: the project's real commands (just, make, npm scripts, Python entry points). Use these in guides instead of inventing commands.
  - `docs`: Markdown not yet published. Read it; it often says who the readers are.
  - `hot_files`: what changed most recently. `central`: modules most of the code imports.
  - `logos`: image files that look like the project's logo, best first, with pixel sizes.

**Settle the logo before writing**, so the first site already carries the project's real identity. If `logo` in the scan is empty, ask the user which image to use:

- Offer the best candidates from `map.logos` by path. Point out a tall lockup with small text (height well above width): it shrinks to an unreadable smudge in the header and browser tab, so suggest a square mark or an SVG if they have one.
- If nothing fits, ask for a path in the repository or an https URL.
- Then call `indoc_setup` with `logo`. It checks that the file exists. If the user has no logo, continue without one; never invent or draw one.

Then read the code, starting from the entry points and the central modules. Tell the user about `coverage` warnings worth acting on, such as unreachable routes or variables missing from `.env.example`.

## 2. Agree on an outline before writing new docs

When there are no pages yet, or the user asks for new sections, **propose the readers and an outline and wait for approval**. Base it on evidence, not a template:

- Who uses this code? A public or partner API, a CLI, a library, a deployed service, the team's own developers?
- Do existing docs already target an audience?
- What would each reader come to do? Install, authenticate, call an endpoint, add a provider, deploy?

Include the **home page as a landing page** in the outline: a one-line tagline, two buttons (where to start, and the most important next page), and three or four feature cards, each a real capability you found in the code and linking to the page that explains it. Show the user the tagline and card titles with the outline. A project whose readers only want reference material can skip it; ask.

## 3. Write pages

Markdown files under the configured `pages:` globs (by default `documentation/`):

```markdown
---
title: How checkout charges a card
order: 2
audience: dev
sources:
  - shop/checkout/**
---
```

- **Cite the code you describe** as `path::Symbol` (methods and nested names use dots: `shop/checkout/service.py::CheckoutService.charge`). Every specific claim about behavior should cite the symbol that implements it. Citations are verified, anchor the page, and become links to the code.
- Set `sources:` to the directories or files the page explains, as narrowly as accurate.
- Add `strict: true` to pages whose readers depend on exact behavior: a partner or public API, webhooks, a CLI's output.
- Use real commands from `map.tasks` and real request/response or CLI examples.
- Do not write route, environment-variable, or command tables. indoc generates them.
- No page per source file. `index.md` at the top of the docs folder is the overview.

The landing page is the overview's frontmatter; the Markdown body follows the cards:

```markdown
---
title: Overview
order: 0
layout: landing
description: One sentence for search results and link previews.
hero:
  title: Shop
  tagline: Take payments in any currency with one API call.
  actions:
    - text: Get started
      link: getting-started.md
    - text: API reference
      link: api.md
features:
  - title: Idempotent by default
    details: Every write takes an idempotency key, so retries never charge twice.
    link: guides/idempotency.md
featured:
  - text: Webhooks
    link: guides/webhooks.md
---
```

The hero shows the logo unless `hero.image` is set. Links are to other pages' Markdown files. Keep the tagline to what the project does, not adjectives.

## 4. Update docs after code changes

Call `indoc_check`. For each finding:

- **stale**: read the changed code (the finding names the file or symbol and what changed) and update the page. If after reading you are certain the page is still correct, call `indoc_ack` with that page. Never acknowledge a page you have not re-read against the code.
- **broken**: a citation or glob no longer resolves (often a rename). Fix it.
- **drift**: implementation changed under the page's sources. Check whether the prose is affected.
- **edited / unstamped / removed**: recorded by the next `indoc_bundle` or `indoc_ack`.

## 5. Build and verify

1. Call `indoc_bundle`. Fix any broken citations it reports and call it again.
2. Call `indoc_check`; `failing` must be 0.
3. Tell the user where the site was written (default `.indoc/site/`) and that opening its `index.html` in a browser works without a server; with the `indoc` command installed, `indoc preview` serves it and rebuilds on every edit. `.indoc/config.yaml`, `.indoc/lock.json`, and the pages should be committed. Suggest adding `indoc check` to CI with the GitHub Action (`uses: byabasaija/indoc@v0.3.1`).
