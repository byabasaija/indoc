---
title: Customizing the site
order: 5
sources:
  - internal/site/**
  - internal/cli/bundle.go
  - internal/pages/pages.go
---

# Customizing the site

`indoc bundle` writes one pre-rendered HTML file per page (`internal/site/site.go::Build`). Pages read fine without JavaScript; scripts only add search, the theme toggle, copy buttons on code blocks, syntax highlighting, diagrams, and the GitHub star count. Links are relative, so the folder works from any path, any static host, or straight from disk.

## Structure

The sidebar mirrors the folders your pages live in:

| File | URL | In the sidebar |
|---|---|---|
| `documentation/index.md` | `/` | The home page, linked from the logo |
| `documentation/faq.md` | `/faq/` | Top level |
| `documentation/guides/index.md` | `/guides/` | Names the "Guides" group and comes first in it |
| `documentation/guides/setup.md` | `/guides/setup/` | In the "Guides" group |

Pages sort by `order`, then title. A group sorts by its `index.md`'s `order`, or the lowest `order` inside it. Reference pages go last, under "Reference". Links between Markdown files (`[setup](guides/setup.md#install)`) become links between pages, and local images are copied into the site. Two pages that would land at the same URL, such as `guides.md` next to `guides/index.md`, or a page at `reference/routes.md` next to the generated Routes page, stop the build with both named, instead of one silently replacing the other.

## Landing page

Give the home page `layout: landing` to get a hero, call-to-action buttons, and feature cards (`internal/pages/pages.go::Hero`):

```yaml
---
layout: landing
description: One line for search engines and link previews.
hero:
  title: Acme Payments
  tagline: Move money with one API call.
  image: images/hero.svg        # defaults to the logo
  actions:
    - text: Get started          # the first action is primary
      link: getting-started.md
    - text: API reference
      link: reference/api.md
features:
  - title: Idempotent by default
    details: Every write takes an idempotency key.
    link: guides/idempotency.md
    link_text: How it works       # defaults to "Learn more"
featured:                         # the right-hand column
  - text: Webhooks
    link: guides/webhooks.md
---
```

The Markdown body follows the cards.

### Sections

To change the order, or add blocks, list them under `sections:` (`internal/pages/pages.go::Section`). Blocks appear in the order listed; without the list the order is hero, features, body.

```yaml
sections:
  - hero
  - type: logos
    title: Used by
    items:
      - image: images/acme.svg      # relative to the page, a repository path, or a URL
        name: Acme
        link: https://acme.dev
  - features
  - type: markdown
    title: Why teams switch
    body: |
      Any Markdown, full width: lists, tables, images, diagrams.
  - type: contributors              # avatars from GitHub
    title: Built by
    limit: 48                       # up to 100
  - body                            # the page's own Markdown
```

| Block | Shows |
|---|---|
| `hero` | Image, title, tagline, and buttons from `hero:` |
| `features` | The cards from `features:` |
| `body` | The page's Markdown. Added at the end when the list leaves it out, so content is never dropped. |
| `markdown` | A titled, full-width block of Markdown from `body:`. Links and images resolve like the page's own. |
| `contributors` | The repository's contributors as avatars, linking to their profiles. `repo: owner/repo` defaults to the GitHub repository in `source_url`. |
| `logos` | A row of logos, such as companies using the project, muted until hovered. |

`contributors` loads from GitHub's API in the reader's browser, like the star count, so `bundle` stays offline and deterministic. Bots are left out. Without JavaScript, or when GitHub's rate limit is reached, the block still links to the contributors page. Mistakes such as an unknown block type or a missing logo image stop the build with the page and section named (`internal/site/site.go::Build`).

### Recipes

Images from other services work in any `markdown` block or in the body:

```markdown
[![Contributors](https://contrib.rocks/image?repo=owner/repo)](https://github.com/owner/repo/graphs/contributors)

[![Star history](https://api.star-history.com/svg?repos=owner/repo&type=Date)](https://star-history.com/#owner/repo)

![License](https://img.shields.io/github/license/owner/repo) ![Release](https://img.shields.io/github/v/release/owner/repo) ![Downloads](https://img.shields.io/github/downloads/owner/repo/total)
```

For layouts Markdown cannot express, set `allow_html: true` and write HTML in a `markdown` block; the theme's colors are CSS variables such as `var(--brand)` and `var(--fg-muted)`.

## Configuration

These keys in `.indoc/config.yaml` shape the site (`internal/cli/bundle.go::siteFor`):

```yaml
appearance:
  logo: design/logo.svg          # path in the repository, or a URL; also the favicon
  colors:
    primary: "#0969da"           # links, active nav, focus
    accent: "#8250df"
links:                           # header links
  - text: Blog
    url: https://acme.dev/blog
footer: "© Acme. [Status](https://status.acme.dev)"   # Markdown
site_url: https://docs.acme.dev  # adds canonical URLs and sitemap.xml
source_links: true               # link to the code; only if readers can open the repository
citations: short                 # short (default), full, or hidden
urls: pretty                     # pretty (guides/setup/) or files (guides/setup/index.html)
show_authors: true               # "Last updated by Jane on …"
allow_html: false                # raw HTML in pages is escaped unless true
```

Two optional files next to the config change the theme without a fork:

- `.indoc/custom.css` is loaded after the built-in stylesheet. Override the custom properties on `:root` and `[data-theme="dark"]`, such as `--brand`, `--bg`, and `--fg`.
- `.indoc/head.html` is copied into every page's `<head>`, for analytics or fonts.

## Page footer

With `source_links: true` and a `source_url` on GitHub or GitLab, each page gets an "Edit this page" link; other hosts get "View source" (`internal/cli/bundle.go::editLink`). The "Last updated" date comes from the last commit that touched the page (`internal/cli/bundle.go::gitLastChange`); the author is shown only with `show_authors: true`.

## Code and diagrams

Fenced code blocks get a copy button and syntax highlighting for common languages, including shell, JavaScript, TypeScript, Python, Go, SQL, YAML, JSON, and Dockerfile. A `mermaid` block becomes a diagram; the Mermaid library is written only when some page uses it, and only those pages load it.

## Publishing

Where the site is served, and how to serve it from your own app, is covered in [Publishing the site](publishing.md).

## Output folder

indoc records the files it wrote in `.indoc-site` inside `output_dir` and removes them when their page goes away. Files it did not write are never touched, so `output_dir` can be a folder your app already serves.
