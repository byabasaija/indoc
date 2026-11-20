---
title: Publishing the site
order: 12
description: Serve the docs on GitHub Pages, a static host, or from your own web app.
sources:
  - internal/cli/publish.go
  - action.yml
---

# Publishing the site

`indoc bundle` writes a folder of HTML. `indoc publish` decides where that folder is served and tells you exactly how to get it there (`internal/cli/publish.go::runPublish`). Run it once; the choice is saved under `publish:` in `.indoc/config.yaml`, and running `indoc publish` again repeats the instructions.

| Where | Command |
|---|---|
| GitHub Pages | `indoc publish --target pages` |
| Netlify, Vercel, Cloudflare Pages, S3 | `indoc publish --target host` |
| Your own web app, under a path | `indoc publish --target app --path /guide/ --access public` |
| Nowhere yet | `indoc publish --target none` |

With the plugin, your agent asks you this after the first build and does the steps below.

indoc writes only its own files: the config, and for GitHub Pages the deploy workflow. Code for your app is printed for you or your agent to add, so you review it like any other change.

## GitHub Pages

`indoc publish --target pages` writes `.github/workflows/indoc-pages.yml`, which builds the docs with the indoc GitHub Action and deploys them on every push to the default branch. An existing workflow file is never overwritten. When `source_url` is a GitHub repository, `site_url` is set to the Pages address, which adds canonical links and a sitemap.

One setting on GitHub: **Settings → Pages → Build and deployment → Source: GitHub Actions.**

## Static hosts

Configure the host with the build command and publish directory that `indoc publish --target host` prints. Netlify, Vercel, and Cloudflare Pages serve a folder's `index.html`, so links such as `guides/setup/` work. Amazon S3 website hosting and some CDNs do not; set `urls: files` for those.

## Your own web app

`indoc publish --target app` detects the framework and prints the code that serves the docs under `--path`:

| Framework | How |
|---|---|
| Django | A view in the root URLconf. With `--access login`, only staff users; others are sent to `LOGIN_URL`. |
| FastAPI | `app.mount(..., StaticFiles(..., html=True))` |
| Flask | A route using `send_from_directory` |
| Express | `express.static` on the path, after your own login middleware when access is `login` |
| NestJS | `ServeStaticModule` |
| Next.js, Astro | No code: the site is built into `public/<path>`, which is served as it is |
| Gin, Echo, chi, Fiber, Go | The framework's static handler, or `http.FileServer` |

It also prints the build step to add to wherever the app is built or deployed (next to `collectstatic` in a Django project, as a `prebuild` script in a Next.js one), because the site folder is not committed.

Choosing `app` sets `urls: files`: pages link to `guides/setup/index.html` rather than `guides/setup/`, because many app servers do not serve a folder's `index.html`. Each framework's code above is tested against the framework itself, serving a built site page by page.

Use `--access login` for internal docs such as runbooks: static files are otherwise readable by anyone with the URL. Django's code enforces it; for the other frameworks the comment says where your own sign-in check goes.

## Links and folders

| `urls:` | Links look like | Works on |
|---|---|---|
| `pretty` (default) | `guides/setup/` | Servers that serve a folder's `index.html`: GitHub Pages, Netlify, Vercel, nginx, `indoc preview` |
| `files` | `guides/setup/index.html` | Any server, and opened straight from disk |

Opened from disk, a `pretty` site still works: its links are rewritten to `index.html` in the browser.
