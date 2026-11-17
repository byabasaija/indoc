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

Pages sort by `order`, then title. A group sorts by its `index.md`'s `order`, or the lowest `order` inside it. Reference pages go last, under "Reference". Links between Markdown files (`[setup](guides/setup.md#install)`) become links between pages, and local images are copied into the site.

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

## Configuration

These keys in `.indoc/config.yaml` shape the site (`internal/cli/bundle.go::siteFor`):

```yaml
appearance:
  logo: design/logo.svg          # path in the repository, or a URL
  colors:
    primary: "#0969da"           # links, active nav, focus
    accent: "#8250df"
links:                           # header links
  - text: Blog
    url: https://acme.dev/blog
footer: "© Acme. [Status](https://status.acme.dev)"   # Markdown
site_url: https://docs.acme.dev  # adds canonical URLs and sitemap.xml
show_authors: true               # "Last updated by Jane on …"
allow_html: false                # raw HTML in pages is escaped unless true
```

Two optional files next to the config change the theme without a fork:

- `.indoc/custom.css` is loaded after the built-in stylesheet. Override the custom properties on `:root` and `[data-theme="dark"]`, such as `--brand`, `--bg`, and `--fg`.
- `.indoc/head.html` is copied into every page's `<head>`, for analytics or fonts.

## Page footer

When `source_url` points at GitHub or GitLab, each page gets an "Edit this page" link; other hosts get "View source" (`internal/cli/bundle.go::editLink`). The "Last updated" date comes from the last commit that touched the page (`internal/cli/bundle.go::gitLastChange`); the author is shown only with `show_authors: true`.

## Code and diagrams

Fenced code blocks get a copy button and syntax highlighting for common languages, including shell, JavaScript, TypeScript, Python, Go, SQL, YAML, JSON, and Dockerfile. A `mermaid` block becomes a diagram; the Mermaid library is written only when some page uses it, and only those pages load it.

## Output folder

indoc records the files it wrote in `.indoc-site` inside `output_dir` and removes them when their page goes away. Files it did not write are never touched, so `output_dir` can be a folder your app already serves.
