---
title: Customizing the site
order: 11
sources:
  - internal/site/**
  - internal/cli/bundle.go
  - internal/pages/pages.go
---

# Customizing the site

`indoc bundle` writes one pre-rendered HTML file per page (`internal/site/site.go::Build`). Pages read fine without JavaScript; scripts only add search, the theme toggle, copy buttons on code blocks, syntax highlighting, diagrams, and the GitHub repository card in the header (the repository name always shows; its latest release, stars, and forks load from GitHub in the reader's browser). Links are relative, so the folder works from any path, any static host, or straight from disk.

## Structure

The sidebar is built from your folders, the way readers expect from any documentation site: each folder becomes a section that folds open, and a folder inside a folder becomes a sub-section (`internal/site/tree.go::builder.nav`). The sections holding the current page start open (on the home page, the top-level ones), the page itself is highlighted, and a breadcrumb trail above its title links back up.

Group pages by what the reader came to do:

```text
documentation/
  index.md                  /                         the home page, linked from the logo
  get-started/
    quick-start.md          /get-started/quick-start/
    writing-pages.md
  guides/
    index.md                /guides/                  optional: names the section and opens on click
    deploy/                                           a sub-section
      docker.md             /guides/deploy/docker/
  reference/
    configuration.md                                  generated reference pages join this section
  concepts/
    how-it-works.md
```

| Question | Answer |
|---|---|
| What is a section called? | Its `index.md` title, or the folder name humanized: `get-started` becomes "Get started". |
| In what order? | Loose pages first, then sections. Pages sort by `order`, then title. A section sorts by its `index.md`'s `order`, else the lowest `order` inside it, so numbering pages across the whole site (1, 2 in Get started; 10, 11 in Guides) orders the sections too. |
| Where does reference go? | indoc's generated reference pages join the `reference/` section, after your own pages there. |
| Previous and next? | They follow the sidebar from top to bottom. |

Links between Markdown files (`[setup](guides/setup.md#install)`) become links between pages, and local images are copied into the site. Two pages that would land at the same URL, such as `guides.md` next to `guides/index.md`, or a page at `reference/routes.md` next to the generated Routes page, stop the build with both named, instead of one silently replacing the other.

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
      link: get-started/quick-start.md
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

`contributors` loads from GitHub's API in the reader's browser, like the repository card in the header, so `bundle` stays offline and deterministic. Bots are left out. Without JavaScript, or when GitHub's rate limit is reached, the block still links to the contributors page. Mistakes such as an unknown block type or a missing logo image stop the build with the page and section named (`internal/site/site.go::Build`).

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
  font: Inter, system-ui, sans-serif   # load web fonts in theme/head.html
  mono_font: JetBrains Mono, monospace
  radius: 8                      # corner radius in pixels, 0 to 32
  content_width: 820             # reading column in pixels, 560 to 1200
  theme: dark                    # light or dark before the reader chooses; default follows the system
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

## Theme folder

The site folder is rebuilt on every `indoc bundle`, so changes made there do not last. Changes to the look belong in `.indoc/theme/`, which indoc reads and never overwrites (`internal/cli/theme.go::loadTheme`):

| File | Effect |
|---|---|
| `custom.css` | Loaded after the built-in stylesheet. Theme colors are CSS variables on `:root` and `[data-theme="dark"]`, such as `--brand`, `--bg`, and `--fg`. |
| `head.html` | Added to every page's `<head>`: analytics, web fonts. |
| `header.html`, `sidebar.html`, `hero.html`, `features.html`, `footer.html` | Replaces that part of every page. |

To change a part, copy the built-in one and edit the copy:

```sh
indoc theme                  # what is built in and what is yours
indoc theme eject footer     # copies the built-in footer to .indoc/theme/footer.html
indoc theme show footer      # prints the built-in, to compare
```

A copied part starts with a comment recording the indoc version and a fingerprint of the built-in it came from. When a later indoc changes that built-in, `indoc bundle` warns, so your copy can be updated rather than silently falling behind (`internal/site/theme.go::Outdated`). A file in the folder that is not a part, such as a misspelled `foter.html`, stops the build. `custom.css` and `head.html` directly in `.indoc/` are still read.

## Edits to the output

indoc records a fingerprint of every file it writes. If a file in the site folder was changed by hand since the last build, `indoc bundle` stops and lists it instead of overwriting it (`internal/site/site.go::Edited`):

- `indoc bundle --keep-edits` moves the edits into the theme folder: new or changed CSS rules are appended to `theme/custom.css`, and other files are saved under `theme/edited/`, to copy into the theme part they belong in.
- `indoc bundle --discard-edits` overwrites them.

With the plugin, the agent asks you which.

## Page footer

With `source_links: true` and a `source_url` on GitHub or GitLab, each page gets an "Edit this page" link; other hosts get "View source" (`internal/cli/bundle.go::editLink`). The "Last updated" date comes from the last commit that touched the page (`internal/cli/bundle.go::gitLastChange`); the author is shown only with `show_authors: true`.

## Code and diagrams

Fenced code blocks get a copy button and syntax highlighting for common languages, including shell, JavaScript, TypeScript, Python, Go, SQL, YAML, JSON, and Dockerfile. A `mermaid` block becomes a diagram; the Mermaid library is written only when some page uses it, and only those pages load it.

## Publishing

Where the site is served, and how to serve it from your own app, is covered in [Publishing the site](publishing.md).

## Output folder

indoc records the files it wrote in `.indoc-site` inside `output_dir` and removes them when their page goes away. Files it did not write are never touched, so `output_dir` can be a folder your app already serves.
