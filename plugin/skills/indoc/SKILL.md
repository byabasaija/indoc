---
name: indoc
description: Write, update, or check a repository's documentation with indoc. Use when asked to document a project, refresh docs after code changes, fix failing indoc checks, or build the docs site.
---

# Documentation with indoc

indoc keeps Markdown pages tied to code. Pages list the code they cover (`sources:`) and cite symbols as `path::Symbol`; indoc fails the check when that code changes and the page does not. You write the pages. indoc never calls a model.

Use the indoc MCP tools: `indoc_scan`, `indoc_setup`, `indoc_check`, `indoc_ack`, `indoc_bundle`. The same operations exist as the `indoc` command.

## Asking the user

The user decides from what you show them, so make every question easy to read and answer:

- **One decision per question.** The question is one short sentence. Never put lists, outlines, or several questions inside it.
- **The user must see what they approve.** Some agents (Claude Code among them) shorten the text you write just before a question or tool call to a one-line summary, so an outline written there is hidden. Put the material in the question itself: if your question tool has a preview for an option, put the outline, file list, or proposal in the preview of the recommended option. If it has none, end your turn with the material and the question in plain text instead of using a question tool. Saying something was drafted is not showing it.
- **Options are short labels** (one to five words) with a one-line description, the recommended one first. Only offer options that answer that question.
- Questions that are independent and quick to answer (can readers open the repository? which logo?) may be asked together. The outline is always asked on its own, after the user has seen it.
- If your tools have no question form, ask in plain text with the same rules: numbered options, the recommended one first.

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

**Ask whether the docs' readers can open the repository** (a public repository, or docs only for the team that has access). If yes, call `indoc_setup` with `source_links: true` so citations, reference entries, and "Edit this page" link to the code. If not, leave it off: those links would be 404s for readers.

**Settle the logo before writing**, so the first site already carries the project's real identity. If `logo` in the scan is empty, ask the user which image to use:

- Offer the best candidates from `map.logos` by path. Point out a tall lockup with small text (height well above width): it shrinks to an unreadable smudge in the header and browser tab, so suggest a square mark or an SVG if they have one.
- If nothing fits, ask for a path in the repository or an https URL.
- If the project has no logo, offer to design one: a simple, square SVG mark (a letter or a plain symbol from what the project does, one or two colors, no text that must be read small). Save it as `documentation/assets/logo.svg`, describe it in a line (shape and colors) and give the path so the user can open it, and change it until they approve. Never present a logo you made as the project's own without asking.
- Then call `indoc_setup` with `logo`. It checks that the file exists. If the user wants no logo, continue without one.

Then read the code, starting from the entry points and the central modules. Tell the user about `coverage` warnings worth acting on, such as unreachable routes or variables missing from `.env.example`.

## 2. Agree on an outline before writing new docs

When there are no pages yet, or the user asks for new sections, **propose the readers and an outline and wait for approval**. Present it as a message, not inside a question: the readers in one or two lines, then the folder tree with each page's title, its kind, and one line on what the reader gets from it, then the landing page's tagline and card titles. For example:

```text
Readers: developers who run and extend the API; partners who call it.

documentation/
  index.md                 Landing: "Move money with one API call."
  get-started/
    quick-start.md         Tutorial: install, configure, make the first call
  guides/
    how-to-add-a-provider.md   How-to: plug in a new payment provider
  reference/               Generated: routes, configuration (plus error codes, hand-written)
  concepts/
    how-charging-works.md  Explanation: idempotency, retries, ledger entries
```

Ask "Approve this outline?" with options such as "Approve" and "Change something", and put this whole outline in the preview of "Approve", one short line per page and under 25 lines so it fits without scrolling (or, without previews, end your turn with the outline and the question in plain text). The user approves only an outline they can read. Base it on evidence, not a template:

- Who uses this code? A public or partner API, a CLI, a library, a deployed service, the team's own developers?
- Do existing docs already target an audience?
- What would each reader come to do? Install, authenticate, call an endpoint, add a provider, deploy?

Give every page one purpose (the Diátaxis framework), and label each page in the outline with it:

| Kind | Reader wants to | Shape |
|---|---|---|
| **Tutorial** | learn by doing, once | A guided path from nothing to something working, every step runnable |
| **How-to** | finish one task | A title starting "How to", prerequisites, numbered steps, the result |
| **Reference** | look something up | Generated by indoc for routes, configuration, commands; write only what extraction can't, such as error codes or webhook payloads |
| **Explanation** | understand why | Architecture, design decisions, how parts fit; no step lists |

Each audience needs at least one how-to for its most common task, not only explanation. An outline that is all explanation reads well and helps nobody get anything done. Most projects need one tutorial (getting started), not several.

**Lay the pages out in section folders**, never as a flat list: the sidebar shows each folder as a section that folds open, like any documentation site. Use the reader's journey, and only the sections the project needs:

```text
documentation/
  index.md          landing page
  get-started/      the tutorial, installation, first steps
  guides/           how-tos; sub-folders by area once there are more than about six (guides/payments/, guides/deploy/)
  reference/        hand-written reference only; indoc adds its generated pages here
  concepts/         explanation: architecture, design decisions
```

Number `order:` across the whole site (1, 2 in get-started; 10, 11 in guides; 20 in reference; 30 in concepts): a section sorts by the lowest order inside it. Show the folder tree in the outline. A small project with three or four pages may skip folders; ask.

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

- **Cite the code you describe** as `path::Symbol` (methods and nested names use dots: `shop/checkout/service.py::CheckoutService.charge`). Every specific claim about behavior should cite the symbol that implements it. Citations are verified and anchor the page; readers see the symbol name, with the full path in the page's "Checked against the code" box.
- **Put citations in parentheses after the claim**: "Orders are charged once per idempotency key (`shop/checkout/service.py::CheckoutService.charge`)." The sentence must read well without them, because pages with `citations: hidden` drop them.
- Add `citations: hidden` to pages for readers outside the team (partners, customers), who should not see internal names. The page is still checked.
- Set `sources:` to the directories or files the page explains, as narrowly as accurate.
- Add `strict: true` to pages whose readers depend on exact behavior: a partner or public API, webhooks, a CLI's output.
- Use real commands from `map.tasks` and real request/response or CLI examples.
- Do not write route, environment-variable, or command tables. indoc generates them.
- No page per source file. `index.md` at the top of the docs folder is the overview; every other page goes in its section folder from the outline.

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

For an open-source project with a GitHub repository, offer a contributors wall: add `sections: [hero, features, {type: contributors}, body]`. A `logos` section can show who uses the project, but only with logos the user provides. The hero shows the logo unless `hero.image` is set. Links are to other pages' Markdown files. Keep the tagline to what the project does, not adjectives.

## 4. Update docs after code changes

Call `indoc_check`. For each finding:

- **stale**: read the changed code (the finding names the file or symbol and what changed) and update the page. If after reading you are certain the page is still correct, call `indoc_ack` with that page. Never acknowledge a page you have not re-read against the code.
- **broken**: a citation or glob no longer resolves (often a rename). Fix it.
- **drift**: implementation changed under the page's sources. Check whether the prose is affected.
- **edited / unstamped / removed**: recorded by the next `indoc_bundle` or `indoc_ack`.

## 5. Build and verify

1. Call `indoc_bundle`. Fix any broken citations it reports and call it again.
2. Call `indoc_check`; `failing` must be 0.
3. Tell the user where the site was written (default `.indoc/site/`) and that opening its `index.html` in a browser works without a server; with the `indoc` command installed, `indoc preview` serves it and rebuilds on every edit. `.indoc/config.yaml`, `.indoc/lock.json`, and the pages should be committed. Suggest adding `indoc check` to CI with the GitHub Action (`uses: byabasaija/indoc@v0.8.2`).

## 6. Change the look

The output folder is rebuilt every time, so never edit it. When the user asks for a different look:

- Colors, logo, fonts, corner radius, content width, default light or dark: `appearance:` in `.indoc/config.yaml`.
- Anything else in CSS: `.indoc/theme/custom.css`, loaded after the built-in stylesheet. Theme colors are CSS variables such as `--brand`, `--bg`, `--fg`.
- Analytics or web fonts: `.indoc/theme/head.html`.
- The header, sidebar, landing hero, feature cards, or footer: run `indoc theme eject <part>`, then edit `.indoc/theme/<part>.html`.

If `indoc_bundle` reports files in the output that were changed by hand, ask the user whether to keep them (`edits: keep` moves CSS rules into the theme and saves other files under `.indoc/theme/edited/`) or discard them. Never discard without asking. After keeping an HTML edit, move it into the theme part it belongs to.

## 7. Publish

If `publish` in the scan is empty, ask the user where the docs should live, then call `indoc_publish`:

- **GitHub Pages** (`target: pages`): indoc writes the deploy workflow. Tell the user the one setting to change on GitHub.
- **A static host** (`target: host`): Netlify, Vercel, Cloudflare Pages, S3.
- **Served by this app** (`target: app`): ask for the URL path (check the routes in the scan so it does not clash with an existing one, such as an API docs page at `/docs`) and who may read it: `public`, or `login` for internal docs such as runbooks. indoc returns code for the detected framework: add it to the app where it says, and add the build step it gives to the project's build or deploy (a justfile, Makefile, Dockerfile, or package.json script). Show the user what you changed.
- **Nowhere yet** (`target: none`).

indoc only writes its own files. Adding the returned code to the app is your job, so the user can review it like any other change.
