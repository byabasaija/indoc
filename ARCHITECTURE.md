# Architecture

**Updated:** 2026-11-17. Plans and decisions live in [ROADMAP.md](ROADMAP.md); this file describes what is built.

## Purpose

indoc keeps Markdown documentation tied to the code it describes and fails CI when they drift apart. A coding agent writes the pages; indoc does the parts that must be exact: parsing code, recording what each page was verified against, detecting change, and building a static site.

indoc never calls an AI model.

## Layers

```
cmd/indoc                 entry point
internal/cli              commands: init, scan, check, ack, bundle, preview, version
        │
        ├── internal/drift      lock file, Stamp, Check, Ack: the staleness rules
        │       ├── internal/pages    Markdown pages, frontmatter, path::Symbol citations
        │       ├── internal/code     file listing, per-language dispatch
        │       │       └── internal/lang/{python,jsts,golang}   symbol parsers
        │       │               └── internal/treesitter   WebAssembly grammars via wazero (Python, JS/TS)
        │       └── internal/glob     `**` glob matching
        │
        └── internal/bundler    static site (embedded viewer), via internal/doc + internal/render
                └── internal/reference   extractors: Django/DRF, FastAPI, Cobra, Python env, generic regex
                        └── internal/lang/python (ParseModule)   static module view on tree-sitter
```

Dependencies point down. Language parsers know nothing about pages; pages know nothing about parsers; `drift` joins them.

## Symbols and hashes

`internal/lang` defines `Symbol`: qualified name (`Dispatcher.dispatch`), kind, lines, and two hashes over significant tokens:

- **Surface:** decorators, name, parameters, return annotation, bases. What callers and readers depend on.
- **Body:** the implementation.

Comments, layout, trailing commas, docstrings and other bare string statements, and single logging calls (`logger.info(...)`) are excluded from both, so reformatting and log edits change nothing. A class's body hash covers its interface (attributes and member signatures), not its methods' bodies, which each method hashes itself. Each parser also produces a whole-file hash so module-level code (settings, URL tables) is tracked.

- **Python** (`internal/lang/python`): tree-sitter syntax trees. Symbols are `def`, `async def`, and `class`, qualified by def/class nesting; hashes skip comments, docstrings and other bare strings, single logging calls, and trailing commas, and keep block structure. The same package builds the static module view the framework extractors use (imports, assignments with annotations, classes with bases and methods, decorators, every call and subscript) and converts expressions to a small `Expr` model. An opt-in differential test compares symbols with CPython's `ast`.
- **Go** (`internal/lang/golang`): the standard library's `go/parser`.

- **JavaScript, TypeScript, JSX, TSX** (`internal/lang/jsts`): tree-sitter syntax trees (below). Symbols are declarations as the TypeScript compiler sees them: functions, classes and their methods, constructors, accessors, arrow-function class fields, variables bound to functions or classes, interfaces, type aliases, enums, namespaces. Exported means public, including `export { a }` and CommonJS `module.exports`. Hashes skip comments, `console.*`/logger calls, semicolons, and trailing commas.

Each parser sets `Symbol.Private`; the lock records private names so a removed symbol is classified correctly. Each file's lock entry also records its exact bytes and the hasher that produced its hashes (parser, grammar version, normalization rules), so a newer indoc never misreads an older lock: unchanged files upgrade silently, changed ones are reported as drift.

Files in other languages are hashed as bytes: tracked, but only as drift.

## Tree-sitter

`internal/treesitter` runs tree-sitter grammars compiled to WebAssembly with wazero, a pure-Go runtime, so indoc remains a single static binary without cgo. Each grammar is one gzipped WASI module (runtime plus grammar, about 90-180 KB compressed) embedded in the binary; a module is compiled on first use (about 70 ms) and cached on disk afterwards (a few ms). A small C wrapper (`build/wrapper.c`) parses and returns the whole syntax tree as a flat pre-order array, which Go rebuilds into nodes; deciding what is a symbol happens in Go.

`make grammars` rebuilds the modules in Docker from sources pinned by commit in `build/grammars.txt`, with the toolchain image pinned by digest. Builds are reproducible byte for byte. Embedded grammars: JavaScript, TypeScript, TSX, Python. Adding a language means adding a line to `grammars.txt`, a Go package with its symbol rules, and a differential test against that language's own parser.

## Pages and the lock

Pages are Markdown files selected by `pages:` globs in `.indoc/config.yaml`. Publishing is opt-in. Frontmatter supplies `title`, `order`, `audience`, and `sources` (Docusaurus `sidebar_label`/`sidebar_position` are accepted). Citations are `path::Symbol` anywhere outside fenced code blocks.

`.indoc/lock.json` (committed) holds, per page: the page's content hash, each cited symbol's surface and body hash, and each covered file's whole-file hash plus its symbols' surface hashes.

`drift.Check` compares the current repository with the lock:

| Condition | Kind | Fails `check` |
|---|---|---|
| Cited symbol's surface changed | stale | yes |
| Cited symbol's body changed | drift (stale on `strict: true` pages) | `--strict` (yes on strict pages) |
| Public surface added/removed/changed, or file added/removed, under `sources` | stale | yes |
| Private surface changed under `sources` (`_name` in Python, lowercase in Go) | drift | `--strict` |
| Covered file changed with no surface change (or unparsed file changed) | drift | `--strict` |
| Citation or `sources` glob does not resolve | broken | yes |
| Page edited, new, or deleted since recorded | edited / unstamped / removed | yes |
| No sources and no citations | unanchored | `--strict` |
| Citation into a language without a parser | unverified | `--strict` |

A page is compared only while its content matches the lock. `drift.Ack` with no targets records only new and edited pages; an unchanged stale page is re-recorded only when named explicitly. `drift.Stamp` refuses pages with broken citations. `check` is read-only.

## Reference extraction

`reference.Extract` runs each applicable extractor and returns sections plus a `Coverage` per extractor (evidence, files read, entries, unresolved items, warnings). Nothing imports or runs project code.

- **Module view** (`python.ParseModule`): imports, module- and class-level assignments, module-level call statements, classes with bases and methods, functions with decorators, and every call and subscript in the file, with expressions converted to the forms configuration uses (strings, dotted names, calls, lists, tuples, `+`, `*spread`, `a if b else c`). Everything else is opaque.
- **Django**: finds `ROOT_URLCONF` in settings, follows `urlpatterns` (`=`, `+=`, `append`, `extend`, `insert`, spreads, `if` blocks marked conditional) through `include()` (string, tuple, module object, inline list) and DRF routers (`register`, `@action`, `lookup_field`, empty prefixes, `DefaultRouter` root). HTTP methods come from handler methods on view classes and their project bases, known framework views, `http_method_names`, and `@api_view`. `settings.X` routes resolve to the literal in settings files, noting overrides. Routes shadowed by an earlier identical path are warned about. Verified against Django's resolver by an opt-in differential test.
- **Python env**: token-level reads through django-environ (including `env.db()` and friends), python-decouple, `os.environ`, `os.getenv`, with type and default. Defaults that may be secrets are redacted. `.env.example` is cross-checked; `.env` is never read.
- **FastAPI**: applications assigned from `FastAPI()`; decorator routes, `api_route`, `add_api_route`, websockets; `include_router` with prefixes (recursively, including `APIRouter(prefix=...)`); mounts (FastAPI sub-apps expanded, others listed as `MOUNT`); built-in docs routes per `openapi_url`/`docs_url`/`redoc_url`. Verified against FastAPI's route table by an opt-in test.
- **Pydantic settings**: annotated fields of `BaseSettings` subclasses (also through project base classes) with `env_prefix` from `model_config` and `Field(alias=...)`.
- **Cobra**: `cobra.Command` literals (package-level or local), `AddCommand` tree, `Flags()`/`PersistentFlags()` definitions, via `go/ast`. A test checks it against indoc's runtime command tree.
- **Management commands**: `<app>/management/commands/<name>.py` with `class Command`, its `help`, and `add_argument` names.
- **Generic**: the original regex extractor for decorator-style routes and Go/JS env reads, labeled best effort.

## Site

`indoc bundle` refuses broken citations, runs `internal/reference` over non-test source files, maps pages onto the viewer's document (the top-most `index.md`/`README.md` is the overview), and writes `index.html`, `style.css`, `app.js`, and `docs.js` to `output_dir` (default `.indoc/site/`, gitignored). It then records new and edited pages, as `ack` would.

`indoc preview` builds into a temporary directory, serves it on `127.0.0.1`, and rebuilds when pages or the config change. It never writes the lock or `output_dir`.

The viewer is still the original hash-routed single page; ROADMAP Phase 6 replaces it with pre-rendered pages.

## Files

```
.indoc/config.yaml   committed
.indoc/lock.json     committed
.indoc/.gitignore    ignores site/
.indoc/site/         build output
```
