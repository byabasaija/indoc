# indoc Roadmap

**Status:** Draft
**Date:** 2026-11-17

---

## 1. Problem

The first real test (a Django codebase, shop) produced docs nobody would keep:

| Symptom | Cause |
| :--- | :--- |
| Reference section empty | Extractor missed 53 Django `path()` routes and ~25 `env("X")` reads. Scan stayed silent about it. |
| Overview ~400 chars, guides ~600 chars | Agent guide asks for "a small number of guides" and sets no quality bar or audience. |
| Existing `docs/*.md` (20 KB of real partner docs) ignored | indoc only knows `.indoc/site.json`. |
| `project_name: Shop API Documentation` | `init` default, never inferred. |

The tool tells the agent to be brief, then hands it no facts. Generic output is the predictable result.

## 2. Position

We do not compete with Docusaurus on rendering (MDX, versioning, i18n, plugins). We will lose that fight and it does not matter.

We compete on the thing no static site generator does: **knowing when the docs are wrong.** indoc hashes the source, knows which code each page describes, and fails CI when that code changes and the page does not.

> **Docs that fail CI when they stop matching the code.**

The static site is the packaging. Drift detection and grounded facts are the product.

Two ways to adopt, and the first needs no migration:

1. **Check-only:** keep Docusaurus, MkDocs, or plain Markdown as the renderer. Add `sources:` frontmatter and `indoc check` to CI. Nothing about the existing site changes.
2. **Full:** indoc also builds the site.

The pitch to an existing Docusaurus team is "add one CI job", not "migrate your docs".

## 3. Three layers

```
┌──────────────────────────────────────────────────────────────┐
│ FRAMEWORK EXTRACTORS: optional enrichment                    │
│  • routes, settings/env vars, CLI commands                   │
│  • Django + DRF, FastAPI, Cobra first; more over time        │
├──────────────────────── built on ────────────────────────────┤
│ LANGUAGE PARSERS: any project in that language               │
│  • symbols with surface/body hashes                          │
│  • path::Symbol citations, precise staleness tiers           │
│  • Python, Go first; JS/TS later                             │
├──────────────────────── built on ────────────────────────────┤
│ CORE: any repo, any file                                     │
│  • Markdown pages, lock, indoc check, bundle, project map    │
│  • file-hash staleness (drift only) where no parser exists   │
└──────────────────────────────────────────────────────────────┘
```

Dependencies point down only. A language parser serves every project in that language regardless of framework (a Flask app or a plain library gets precise staleness from the Python parser). A framework extractor is a thin layer that reads calls like `path("x/", View)` using its language's tokenizer. Adding Flask is extractor work only; adding Express needs the JS/TS parser first.

The README keeps two support tables: **languages** (staleness, citations) and **frameworks** (reference pages). A framework can be marked supported only if its language is.

A repo outside every row still gets the core. **An unsupported framework or language is a smaller feature set, not a broken tool.**

## 4. Data model

### Pages are Markdown files

`.indoc/site.json` (prose escaped inside JSON) is removed. Pages are Markdown files matched by `pages:` globs in `.indoc/config.yaml` (opt-in; see section 11):

```markdown
---
title: Integrate with the partner API
order: 2
audience: partner          # optional; groups pages in the viewer
sources:                   # globs this page describes
  - shop/api/partner/**
  - shop/core/services/payments.py
---

Partners authenticate with ... (`shop/api/partner/auth.py::PartnerAuthentication`)
```

* `index.md` (or the page with `order: 0`) is the overview.
* Pages with no frontmatter still work. The title comes from the first `# H1`. `indoc check` reports them as *unanchored*.
* We read Docusaurus keys (`title`, `sidebar_position`, `sidebar_label`) so an existing Docusaurus `docs/` folder bundles without edits.

### Reference is never authored

`bundle` runs the extractors itself. The agent no longer copies a `reference` array, so it cannot corrupt it. Human-written descriptions for routes or settings go in an optional `.indoc/reference.yaml`, keyed by route or variable name. Keys that no longer match an extracted entry are reported.

### Lock file (committed)

`.indoc/lock.json` records, for each page: the page content hash and the surface and body hashes of each cited symbol and each file its `sources` match. `cache.json` stays local; the lock file is committed so CI can read it.

### Where things live

indoc's own files stay in one folder. Nothing else is added to the repo root.

```
.indoc/
├── config.yaml       committed   (replaces root .indoc.yaml)
├── lock.json         committed
├── reference.yaml    committed   optional route/env descriptions
├── .gitignore                    written by init: cache.json, site/
├── cache.json        local
└── site/             local       default build output
```

**Pages** are user content and live where the user chooses. `init` proposes:

| Repo state | Pages location |
| :--- | :--- |
| Docusaurus config present | Its docs path |
| No `docs/` | `docs/` |
| `docs/` holds other material | Ask: opt files in under `docs/`, or a subfolder |

**Build output** defaults to `.indoc/site/`: gitignored, collides with nothing. Generated files are not committed by default. `init` asks how the site will be published:

* **Static host / CI:** keep the default; CI runs `indoc bundle`.
* **Served by the app:** set `output_dir` to a folder the app already serves (Django: inside a `STATICFILES_DIRS` entry so `collectstatic` picks it up). Commit the output (deterministic, clean diffs) or run `indoc bundle` in the Dockerfile.

**URL path** is the host app's choice, never indoc's. The bundle uses relative links only, so it works under `/docs/`, `/guide/`, or anywhere. Because indoc extracts routes, `init` warns about collisions, e.g. "FastAPI already serves Swagger at `/docs`; mount at `/guide/`."

**OpenAPI is complementary.** drf-spectacular and FastAPI already generate exact request/response schemas. Route reference pages link to the project's Swagger/OpenAPI URLs; reading a committed OpenAPI file to enrich entries is a later extractor feature.

## 5. Commands

| Command | Purpose |
| :--- | :--- |
| `indoc init` | Infer project name (go.mod / pyproject / package.json / dir), detect frameworks, find existing docs, write `.indoc/`. No hand-editing required. |
| `indoc scan --json` | Project map + changed files + **stale and drifting pages with reasons** + extraction coverage. Input for the agent. |
| `indoc check` | Read-only. Exit 1 if a page is stale, a citation is broken, the lock is out of date, or (with `--strict`) a page drifts or is unanchored. `--warn` reports without failing. CI entry point. |
| `indoc ack [page…]` | Update the lock. No args: re-stamp pages whose content changed. With a page: re-stamp it even if unchanged ("reviewed, still correct"). |
| `indoc bundle` | Validate, extract, render the site. Re-stamps edited pages only. |
| `indoc preview` | Bundle to a temp dir, serve on localhost, rebuild and reload on save. Replaces `serve`. |
| `indoc mcp` | MCP server over stdio exposing scan / check / ack / bundle (Phase 5). |
| `indoc version` | Version info. |

**Staleness rule** (tiers in section 11): a page is stale when a cited symbol or covered surface differs from the lock and the page content does not. Editing the page clears it once re-stamped (`bundle` or `ack`). Nothing re-stamps an *unchanged* stale page except an explicit `ack <page>`, so no routine command can silence a real warning.

### Why the CLI stays

The binary is the engine; every other interface is a thin layer over it.

* **CI has no agent.** `indoc check` in GitHub Actions is the CLI.
* **Check-only adopters may not use an agent at all.** Humans run `check` and `ack`.
* **Builds and Dockerfiles** run `indoc bundle`.
* **MCP is `indoc mcp`**, the same binary. The plugin provisions it; users never type commands, but the commands still exist.

The plugin is the primary *onboarding* path. The CLI is the primary *interface for machines and power users*.

### Removed

| Removed | Replaced by |
| :--- | :--- |
| `.indoc/site.json`, `site.schema.json` | Markdown pages with frontmatter |
| `AGENT_GUIDE.md` template written by `init` | The skill / MCP prompt (one copy, shipped with indoc, not per repo) |
| Agent copying the `reference` array | `bundle` runs extractors itself |
| `previous` document in scan output and cache | Pages are files; the agent reads them |
| `bundle --input` | Pages found via `pages:` globs |
| Root `.indoc.yaml` | `.indoc/config.yaml` |
| Default output `./static/docs` | `.indoc/site/` |
| Hash-routed SPA viewer (`app.js`, `docs.js`) | Pre-rendered page per file |
| `indoc serve` | `indoc preview` with rebuild on save |
| Regex extractor as the only extractor | Framework extractors; regex kept as labeled "generic" fallback |

## 6. Extractor contract

Each framework is an extractor in `internal/reference` (built on the static module reader in `internal/lang/python`) that returns entries plus a `Coverage` report. The original sketch:

```go
type Extractor interface {
    Name() string
    Detect(repo Repo) (Detection, bool)         // evidence: "manage.py, django in pyproject.toml"
    Extract(repo Repo) (Result, error)
}

type Result struct {
    Sections []doc.RefSection
    Coverage Coverage                           // files read, entries found, what could not be resolved
}
```

Rules every extractor follows:

1. **Static only.** Never import or run the project's code.
2. **Fail loudly.** If `Detect` succeeds and `Extract` finds 0 entries, scan reports a warning. Unresolved constructs (dynamic `include()`, computed prefixes) are listed, not dropped.
3. **Fixture-tested.** Each extractor ships a `testdata/` mini-project and golden output. A framework is "Supported" only when its fixtures cover the patterns listed in its README row.
4. **No cgo.** indoc stays one static binary on every platform. Parsers are pure Go. Revisit tree-sitter only if a third language makes hand-written parsing the bottleneck.

### Parsers

What staleness and extraction need from a parser is structural, not semantic: where each `def`/`class`/`func` starts and ends, its signature, its decorators, its body with comments and whitespace removed, and the literal arguments of calls like `path("x/", View)` or `env("KEY")`. No type inference, no import resolution beyond following `include("app.urls")` to a file.

| Language | Parser | Why |
| :--- | :--- | :--- |
| Go | Standard library `go/parser` + `go/ast` | Exact, maintained by the Go team, zero dependencies. |
| Python | tree-sitter (originally hand-written; replaced in Phase 4b) | See below; the hand-written parser's CPython differential carried over as the gate. |
| JS/TS (done), then others on demand | One shared path: tree-sitter grammars compiled to WebAssembly, run with a pure-Go runtime (wazero); symbol rules in Go | Hand-writing parsers does not scale past a few languages. With a shared runtime, a new language is a grammar plus a query file. Each language must pass a differential test against its own reference parser (e.g. `syn` for Rust) before it is marked supported. Requires a spike first: binary size (~0.5-3 MB per grammar) and speed. |

**Python, in detail.** The tokenizer is exact and complete: strings (triple-quoted, raw, bytes, f-strings with nested quotes), comments, bracket nesting, line continuations, `INDENT`/`DEDENT`. The parser above it is deliberately partial: it recognises `def`, `async def`, `class`, decorators, and call expressions with literal arguments, and treats every other statement as opaque tokens. Because hashing runs over tokens, comment and whitespace changes are ignored without any extra work.

Alternatives rejected:

* **Shelling out to `python3 -c 'import ast'`.** Exact, but requires Python at runtime (not guaranteed in a Go-based CI image), and an older interpreter fails on newer syntax.
* **Existing pure-Go Python parsers.** The ones available target old grammar versions and fail on modern syntax (f-string nesting, `match`, type parameter syntax).
* **tree-sitter via cgo.** Breaks the single static binary and cross-compilation.

**Correctness is measured, not assumed.** A test-only harness (never a runtime dependency) runs CPython's `ast` module over a corpus (shop, the other Django and FastAPI repos available locally, plus Django and DRF source) and compares, per file: every symbol's name, qualified path, and start/end line. **Target: 100% agreement.** Every disagreement becomes a fixture.

**Result (2026-11-17):** 29,536 files, 409,406 symbols, **100% agreement** with CPython 3.13 `ast` (name, qualified path, start and end line). Corpus: 11 local Django/FastAPI projects including their virtualenvs (Django, DRF, Pillow, pygments, and more) plus the CPython 3.13 standard library. 6 files skipped because CPython itself rejects them. The first run agreed on 84% of files; two tokenizer bugs (indentation measured after a bracket closed mid-line; backslash before `{` in f-strings) accounted for every mismatch and are now regression tests. Reproduce:

```
INDOC_PY_CORPUS=~/projects/shop INDOC_PYTHON=python3.13 \
  go test ./internal/lang/python -run Differential -v
```

The current regex extractor becomes the "generic" fallback (Express-style, `os.getenv`, `process.env`) and is labeled *best-effort*.

### First targets

**Django (+ DRF):** covers 8 of the author's own projects.
* `ROOT_URLCONF` → follow `include("app.urls")` to files, join prefixes.
* `path()`, `re_path()`, `urlpatterns +=`, `name=` → route entries. Detail is the view (Django does not declare methods on routes).
* DRF `DefaultRouter`/`SimpleRouter.register()` → list/detail routes; `@action` extras.
* Settings: `django-environ` (`env()`, `env.str/bool/int/list()`), `python-decouple` `config()`, `os.environ`/`os.getenv`; defaults captured; cross-checked against `.env.example`.
* Management commands (`management/commands/*.py`) → `commands` reference kind.

**FastAPI:** covers 3 projects.
* `@app/router.<method>()` with `APIRouter(prefix=)` and `include_router(prefix=)` resolved.
* Pydantic `BaseSettings` fields → env vars (with `env_prefix`).

**Go / Cobra:** dogfooding; indoc documents itself.
* Commands, `Use`, `Short`, flags → `commands` reference kind.

Later candidates, in order of demand: Express/NestJS, Next.js route handlers, Flask blueprints, Laravel, Rails, Spring. Each is added only with fixtures and a README row.

## 7. Writing quality

Deterministic checks the tool owns:

* **Citations:** `path`, `path:line`, `path#L42` in pages must resolve. Broken → error.
* **Uncited page:** warning. Generic prose shows up as a page with nothing to cite.
* **Command blocks:** `just X`, `make X`, `npm run X`, `uv run X` targets must exist (Phase 3b).

Agent workflow (skill / MCP prompt):

1. Run scan; read the project map and existing docs.
2. **Propose an audience and outline to the user before writing.** Example for shop: *partner integrators* (auth, endpoints, callbacks, errors) and *team developers* (setup, checkout routing, provider adapters).
3. Write pages with `sources:` and citations. Every guide has real commands, real paths, and a real request/response or CLI example where one applies.
4. Run `check` and `bundle`; fix errors.

The "small number of guides" instruction is removed. The bar is "every claim cites code", not a page count.

## 8. Structure and customization

### Structure: no templates

indoc never ships a fixed outline. Templates are how every docs site ends up looking the same.

| Who | Decides |
| :--- | :--- |
| Files | Navigation: folders become sections, `order:` sorts pages. |
| Agent | Proposes audiences and an outline from evidence in the repo. |
| User | Approves the outline before pages are written. |
| indoc | Only the generated Reference section. |

The skill gives the agent questions and evidence to look for, not outlines: who calls this code, is there a public or partner API, library or deployed service, do existing docs already define audiences. Archetypes (library, CLI, service, app, monorepo) are hints, never output templates.

### Presentation: three tiers

1. **Plain Markdown (works today):** badges, contributor images (shields.io, contrib.rocks), screenshots.
2. **Built in, config-driven:** `layout: landing` for `index.md` (hero, tagline, CTA buttons, feature cards); header links with live GitHub star count; footer; "Edit this page"; last-updated date from git; logo and colors.
3. **Escape hatches:** `custom.css`, `head.html` (analytics, fonts, meta), `allow_html: true` for raw HTML in pages (off by default; agents write pages).

Live data (star counts) is fetched in the browser, never at bundle time, so bundles stay deterministic. Failure degrades to a plain link.

No plugin system, no MDX, no React components. Projects that need interactive widgets in docs should use Docusaurus; the README says so.

### Default theme

The user's repo supplies content and structure; user preferences supply branding. indoc supplies the **layout and quality bar**, and that is what makes a default site look credible with zero configuration. The reference design is `design/default-theme.webp` (a GitHub-style docs layout used as inspiration, not copied: no third-party names, mascots, or assets).

| Element in the reference | Filled from |
| :--- | :--- |
| Header: logo, title, search, theme toggle | `appearance.logo`, `project_name` |
| Left nav: pinned links, then categories | `links:` in config; categories = page folders |
| Hero: image, title, tagline, primary/secondary action | `index.md` frontmatter with `layout: landing` |
| Feature cards | `features:` in `index.md` frontmatter |
| Right rail | Table of contents on pages; `featured:` on landing |
| Edit this page, last edited by/on | `repo_url` + git history (author display optional) |

### Bundle output

Every page is pre-rendered to its own HTML file. No JavaScript is required to read the docs; JS only adds search, theme toggle, and the live star count. The current hash-routed single page (`#/guides/x`, blank without JS, not indexable) is replaced.

```
.indoc/site/                     (default; output_dir overrides)
├── index.html                  landing page
├── partner/authentication/index.html
├── dev/checkout-routing/index.html
├── reference/routes/index.html     generated
├── reference/config/index.html     generated
├── 404.html
├── assets/
│   ├── indoc.css               theme; brand colors injected as CSS variables
│   ├── indoc.js                search, theme toggle, star count (small, no framework)
│   ├── search-index.js         built at bundle time (.js, not .json, so search works from file://)
│   ├── mermaid.min.js          only if a page uses Mermaid
│   ├── logo.svg                copied from appearance.logo
│   └── img/…                   images referenced by pages
├── custom.css                  only if provided
└── sitemap.xml                 when site_url is set
```

* Syntax highlighting happens at bundle time (pure-Go highlighter): no highlighting JS shipped.
* No CDN requests. The folder works offline, from `file://`, from any static host, or mounted under a sub-path (`/docs/`) via relative links.
* Deterministic: same inputs give byte-identical output, so the bundle can be committed or diffed.

## 9. Phases

Each phase ends with a concrete check against a real repo, shop unless noted.

### Phase 1: Markdown pages + `check`
* `pages:` globs, frontmatter parsing (incl. Docusaurus keys), overview detection.
* Remove `site.json` path, `site.schema.json`, `AGENT_GUIDE.md` template.
* Go symbol parser (`go/ast`); Python tokenizer + structural parser (signatures, bodies, surface hash per symbol).
* CPython `ast` differential test over the local corpus at 100% symbol agreement.
* `path::Symbol` citations, `lock.json`, tiered `indoc check`, `indoc ack`, stale/drift reasons in scan.
* Move config to `.indoc/config.yaml`; default output `.indoc/site/`; `init` writes `.indoc/.gitignore`.
* `init` infers project name, lists existing docs for opt-in, proposes a pages location, and asks how the site will be published.
* `check` works on any Markdown docs folder (check-only adoption), including Docusaurus.
* **Exit:** shop's `partner-integration.md` bundles with `code-review-gaps-*.md` excluded; replay test meets the section 12 targets.

### Phase 2: Extractor interface + Django/DRF (done 2026-11-17)
* Coverage reporting per extractor, zero-result warnings.
* Django URLconf resolution, DRF routers, settings, management commands.
* **Exit:** shop reference shows all 53 URL patterns (resolved full paths) and every env var, or names exactly which could not be resolved.
* **Result:** a differential test runs Django's own URL resolver in each project's environment and compares route by route. Six private Django projects: **522 routes, 0 missing, 0 extra, 0 wrong HTTP methods.** Third-party URL trees (admin, allauth, wagtail, debug toolbar) are reported as external includes. Env vars on shop: 31 found, a superset of what `grep` finds (25; it misses multi-line calls). Found along the way: an unreachable DRF API root in app-e (now a warning) and secret defaults that would have been published (now redacted).

### Phase 3: Citations + project map (done 2026-11-17)
* Citation validation in `check`/`bundle` (Phase 1); "view source" links via a `source_url` template inferred from the git remote.
* Scan returns the project map: frameworks, entry points, most-imported modules, git hot files, existing docs, task runner targets.
* Rewrite the skill around outline-first authoring.
* **Exit:** regenerating shop docs produces partner and developer pages where every guide cites code; side-by-side review against the first run.
* **Result:** following the skill on shop (in a copy; the repository was not modified):

| | First run (old workflow) | Phase 3 workflow |
| :--- | :--- | :--- |
| Narrative | overview ~70 words, 3 guides ~100 words each | 6 pages, ~2,300 words |
| Readers | none stated | partner integrators (existing guide, kept and corrected) and developers |
| Citations | 0 | 38, all verified, all linked to the exact line |
| Commands | none | from the `justfile` and CI workflow |
| Reference | empty | 67 routes, 31 environment variables, 3 management commands |
| Drift detection | none | changing `verify_withdrawal_otp`'s signature fails `check` on exactly the two pages that describe it |

  The existing partner guide was wrong on three points the code made visible: the API key is required only for partner sessions (not on every call), a wrong or amount-mismatched code returns `400 invalid_code`, and an code is valid only for the amount it was issued for. Partner pages are `strict: true`. Setup is the only page without citations; it cites the `justfile` via `sources:`.

### Phase 4: FastAPI + Cobra (done 2026-11-17)
* **Exit:** lexai (or another FastAPI project) and indoc itself produce complete reference pages.
* **Result:** FastAPI extraction matches FastAPI's own route table (expanded through included routers) on fastapi-app (6/6) and on a fixture covering decorators, `api_route`, `add_api_route`, nested `include_router` with prefixes, websockets, mounted sub-apps, static mounts, custom docs URLs, and duplicate paths (14/14). lexai and another-fastapi-app cannot be imported safely (lexai creates a database at import), so they were checked by reading the source: all routes and environment variables found. Pydantic `BaseSettings` fields are extracted as environment variables. Cobra extraction matches indoc's runtime command tree exactly (8 commands, all flags). The six Django projects still match after the changes.

### Phase 4b: tree-sitter parsing layer (JS/TS done 2026-11-17)
* wazero + tree-sitter WebAssembly; full syntax trees to Go, symbol rules in Go (no `tags.scm`).
* **Exit:** differential agreement on a real corpus, binary size and scan speed measured, go/no-go recorded here.
* **Result: go.**

| Measure | Result | Target |
| :--- | :--- | :--- |
| Correctness | 9,436 JS/TS/TSX files, 56,396 symbols, **0 mismatches** vs the TypeScript compiler (a production web app and a frontend `node_modules`). 57 minified bundles reported, not compared. | ≥ 99.5% |
| Binary size | +4.3 MB (12.9 → 17.2 MB): ~0.5 MB gzipped grammars, the rest wazero | ≤ +6 MB |
| Startup | 70 ms cold (compile), 3-4 ms with the on-disk cache | ≤ 300 ms |
| Speed | ~5 MB/s per core; 4,684 JS files (41 MB) in 3.2 s on 8 workers | 5,000 files < 10 s |
| Reproducibility | `make grammars` output identical byte for byte | — |

  Getting to zero mismatches meant settling what a symbol is, mostly on the reference side: object-literal methods are not symbols (no stable name); `const A = class B {}` is one symbol named `A`; named class expressions count; `declare global` is not a namespace; computed names (`[Symbol.iterator]`) are skipped. Tree-sitter is about 7x slower than the hand-written Python parser per file (4 vs 29 MB/s); in absolute terms shop takes 0.35 s instead of 0.05 s.
* **Python moved to tree-sitter (done 2026-11-17).** The hand-written tokenizer, parser, and expression parser (~1,000 lines) are deleted; symbols, hashes, and the module view used by the Django, FastAPI, and env extractors now come from tree-sitter syntax trees. Gates, all passed: CPython differential 34,120 of 34,121 files (454,837 symbols) (the one miss is CPython's compiler stress test `test_compile.py`, whose dedented continuation inside brackets defeats the grammar's indentation scanner); the six Django differentials (524 routes) and FastAPI differentials unchanged; shop reference identical (67 routes, 31 variables, 3 commands); the 100-commit shop replay produced exactly the same failing findings commit for commit. Per-file parsing is ~7x slower than the hand-written parser, but `check` takes 0.14 s and `scan` 0.63 s on shop, so parallel analysis was not added.
* **Lock compatibility.** Each lock entry records the file's exact bytes and the hasher (parser, grammar commit, normalization rules version). When a newer indoc hashes differently, unchanged files upgrade silently on the next `ack`/`bundle` and changed files are drift, so upgrading indoc never fails a check.
* **JavaScript framework routes (done 2026-11-17).** Express, Next.js, NestJS, and Astro, each checked against the framework itself where it can run: Express 4.22 and 5.3 via a runtime tracer (14/14 routes), Next.js 15.5 via `next build` route manifests (10/10), NestJS 11.2 via the routes it registers (8/8), and Astro on a production web app (70 page files, every endpoint's methods matching its exports). Go stays on `go/ast`.
* **Phase 4b complete.** Next: Phase 5.
* Rust was dropped: no demand yet, and every grammar costs binary size and maintenance. The pipeline adds a language in a day when a user needs one.

### Phase 5: MCP + plugins (built 2026-11-17; release pending)
* `indoc mcp` subcommand (same binary): five tools (`indoc_scan`, `indoc_setup`, `indoc_check`, `indoc_ack`, `indoc_bundle`) sharing the CLI's code paths. The protocol layer is ~250 lines with no dependencies and speaks both MCP eras: the `initialize` handshake (2024-11-05 to 2025-11-25) and 2026-07-28 (`server/discover`, per-request `_meta`, `resultType`). First scan sets the project up; existing Markdown makes the agent ask the user what to publish.
* One plugin directory (`plugin/`) for Claude Code and Copilot CLI: Copilot reads `.claude-plugin/plugin.json`, `.mcp.json`, and `${CLAUDE_PLUGIN_ROOT}`. The server starts as `sh ${CLAUDE_PLUGIN_ROOT}/bin/indoc mcp` because Copilot expands plugin variables in `args` but not `command`. Marketplace at `.claude-plugin/marketplace.json`.
* Launcher `plugin/bin/indoc` downloads the release matching the plugin version, verifies it against `checksums.txt`, and caches it in the plugin data directory. Tested against a locally served release: download, cache, MCP through the launcher, and refusal of a tampered archive.
* `.github/workflows/release.yml`: on a `v*` tag, static builds for macOS, Linux, Windows (amd64, arm64), `checksums.txt`, GitHub release; refuses if the plugin's pinned version differs from the tag.
* `action.yml`: `uses: byabasaija/indoc@vX.Y.Z`, with `mode: default | strict | warn`.
* `.github/workflows/ci.yml`: gofmt, vet, tests, indoc's own `check --strict`, and grammar reproducibility.
* **Remaining:** tag and publish v0.1.0 (license: MIT), then the exit test below on a fresh machine with each host. Cursor and other MCP clients: document running `indoc mcp`.
* **Exit:** fresh machine, one plugin install, "document this project" → bundled site, no indoc commands typed.
### Phase 6: Viewer
* Pre-rendered page-per-file output replacing the hash-routed SPA; default theme per section 8.
* Syntax highlighting, Mermaid, dark mode, folder-based navigation, source links.
* Landing layout, header links with star count, footer, edit-this-page, last-updated.
* `custom.css`, `head.html`, `allow_html`.
* **Exit:** indoc's own docs site uses the landing layout and looks credible next to a default Docusaurus site.

## 10. Not doing

Saying no keeps indoc small enough to be excellent at one thing. Each item lists why, and what would make us reconsider.

| Not doing | Why | Reconsider if |
| :--- | :--- | :--- |
| Calling an AI model from the binary | The agent the developer already pays for does the writing. No API keys, no billing, no prompt maintenance in Go. | Never; this is the architecture. |
| Fixed outlines or page templates | Templates are why docs sites all look the same. The agent proposes from evidence; the user approves. | Never for output. Archetype hints for the agent are fine. |
| Running or importing the user's code | Needs their dependencies, database, and secrets; unsafe in CI. Static analysis only. | A framework whose routes cannot be found statically at all, and only as an opt-in. |
| Plugin system, MDX, React components in pages | This is how Docusaurus became Docusaurus. Every extension point is permanent maintenance. | Repeated requests that `custom.css`, `head.html`, and `allow_html` cannot cover. |
| Versioned docs (v1 / v2 side by side) | Large feature, few projects need it. Git tags already version the Markdown. | Users with real multi-version needs ask, after Phase 6. |
| Translations (i18n) | Large feature, and agent-written translations go stale twice as fast. | After drift detection is proven; translations could be anchored like pages. |
| Bundling `docs/` (or any folder) by default | It published a security-gaps file in our very first test repo. Publishing is opt-in. | Never. |
| Network access during `bundle` | Builds must be deterministic and work offline. Live data (stars) loads in the browser. | Never for build output. |
| Committing build output by default | Generated files cause merge noise. Default output is gitignored; committing is a choice. | Never as a default. |
| cgo / tree-sitter | indoc must stay one static binary on every platform. | A third language makes hand-written parsing the bottleneck. |
| Duplicating OpenAPI / Swagger | drf-spectacular and FastAPI already produce exact schemas. We link to them, and may read them later. | Never as a replacement. |
| Hosting docs for users (an "indoc cloud") | The output is static files; any host works. | Out of scope for the open-source tool. |
| Competing on rendering features | We lose that fight. We compete on keeping docs correct. | Never; this is the positioning. |

## 11. Decisions on the former open questions

### Staleness is symbol-level, tiered

Measured on shop (411 commits, 1,072 non-test source-file changes):

| Change kind | Share |
| :--- | :--- |
| Whitespace/comment only | 3.5% |
| Function bodies only | 67% |
| Signatures, routes, env vars ("surface") | 29% |

87% of commits touch source; 36% change surface. File-hash staleness would flag nearly every commit for any page anchored to a broad directory. Developers would learn to `ack` blindly and the feature would die. Normalizing whitespace removes only 3.5%.

Rules:

| Change | Result |
| :--- | :--- |
| Cited symbol's signature changed, or symbol gone | **Stale** / **Broken**: `check` fails |
| Public surface changed under `sources` globs | **Stale**: `check` fails |
| Cited symbol's body changed | **Drift**; **Stale** on pages with `strict: true` |
| Body-only or private-helper change under `sources` globs | **Drift**: reported by `scan`, fails only with `check --strict` |
| Comments, docstrings, logging calls, formatting (incl. trailing commas) | Ignored |

These rules are the result of the replay measurement in section 12. The first version made every cited body change stale; on real history that was right only about 40% of the time.

* Citations name symbols, not lines: `path::Symbol` or `path::Class.method`. The viewer resolves the current line when linking. Line numbers rot on every edit above them; a symbol only breaks on rename, which is exactly when the doc should be flagged.
* Cited symbols are anchors automatically. `sources:` globs are for broader coverage.
* This needs per-**language** symbol parsers (Python, Go, TS/JS), separate from per-**framework** extractors. Languages without one fall back to normalized file hashes and report drift only.

### Publishing is opt-in

shop's `docs/` contains partner docs next to `code-review-gaps-*.md` and agent planning files. Bundling `docs/` by default would publish a list of security gaps.

* Only files matched by `pages:` globs in `.indoc/config.yaml` are bundled.
* `init` lists existing Markdown; the agent asks the user which to include.
* No existing docs: default `pages: ["docs/**/*.md"]`.
* `docusaurus.config.js` present: use its docs path (already an explicit published set).

## 12. Acceptance bar

Every phase is measured against real repositories before the next starts. Staleness specifically:

* **Replay test:** fixed page set for shop, replay the last 100 commits, run `check` at each.
* **Targets:** at most 5% of commits fail `check` with no real finding (false blocks); at least 80% of 20 hand-reviewed flags are judged real. (The original target, under 15% of commits failing for any reason, was replaced on 2026-11-17: it penalized catching real staleness in actively developed code.)
* Miss either target → tune detection before Phase 2. Results are recorded in this file.

### Result: shop replay (2026-11-17)

Setup: 7 pages written as an agent would (3 partner pages marked `strict: true`), anchored at `54ecd5f` (2025-11-21); replay of the next 100 first-parent commits. In that window the documented directories gained 5,394 lines and lost 3,579, including a provider-abstraction restructure. Each failing commit simulates the developer re-recording the flagged pages. Tool: `go run ./tools/replay`.

| Iteration | Change | Commits failing | Sampled precision |
| :--- | :--- | :--- | :--- |
| 1 | Initial rules | 43% | not sampled |
| 2 | Ignore docstrings and logging calls; class citation = interface | 30% | 10/20 (50%) |
| 3 | Ignore trailing commas; cited body change → drift unless `strict: true` | 20% | **17/20 (85%)**, fresh sample |
| 4 | Private-helper changes → drift | **17%** | ≥ 85% (only false flags removed) |

Judging rule: a flag is real if the change alters behavior or interface that the page describes or that its reader relies on. Real catches included webhook signature verification being removed (the partner page still described it), OTPs becoming amount-bound, new partner endpoints and error responses, and files moved by a restructure.

**Precision target met (85% ≥ 80%). Failure-rate target missed (17% > 15%).** About 3–4 of the 17 failing commits had no real finding, so roughly 3–4% of commits would block CI wrongly; the rest failed because the docs were actually wrong. A raw failure-rate target penalizes catching real staleness in actively developed code, so the target was replaced with false blocks ≤ 5%. **Phase 1 gate: passed** (≈3–4% false blocks, 85% precision).

