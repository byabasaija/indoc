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
        │       │       └── internal/lang/{python,golang}   symbol parsers
        │       └── internal/glob     `**` glob matching
        │
        └── internal/bundler    static site (embedded viewer), via internal/doc + internal/render
                └── internal/reference   route and env-var extraction
```

Dependencies point down. Language parsers know nothing about pages; pages know nothing about parsers; `drift` joins them.

## Symbols and hashes

`internal/lang` defines `Symbol`: qualified name (`Dispatcher.dispatch`), kind, lines, and two hashes over significant tokens:

- **Surface:** decorators, name, parameters, return annotation, bases. What callers and readers depend on.
- **Body:** the implementation.

Comments and layout are excluded from both, so reformatting changes nothing. Each parser also produces a whole-file hash so module-level code (settings, URL tables) is tracked.

- **Python** (`internal/lang/python`): a complete tokenizer (all string forms including PEP 701 f-strings, indentation, continuations) and a deliberately partial parser that understands only `def`, `async def`, `class`, and decorators. Pure Go; Python is never needed at runtime. An opt-in differential test compares it with CPython's `ast` module; see ROADMAP section 6 for results.
- **Go** (`internal/lang/golang`): the standard library's `go/parser`.

Files in other languages are hashed as bytes: tracked, but only as drift.

## Pages and the lock

Pages are Markdown files selected by `pages:` globs in `.indoc/config.yaml`. Publishing is opt-in. Frontmatter supplies `title`, `order`, `audience`, and `sources` (Docusaurus `sidebar_label`/`sidebar_position` are accepted). Citations are `path::Symbol` anywhere outside fenced code blocks.

`.indoc/lock.json` (committed) holds, per page: the page's content hash, each cited symbol's surface and body hash, and each covered file's whole-file hash plus its symbols' surface hashes.

`drift.Check` compares the current repository with the lock:

| Condition | Kind | Fails `check` |
|---|---|---|
| Cited symbol's surface or body changed | stale | yes |
| Surface added/removed/changed, or file added/removed, under `sources` | stale | yes |
| Covered file changed with no surface change (or unparsed file changed) | drift | `--strict` |
| Citation or `sources` glob does not resolve | broken | yes |
| Page edited, new, or deleted since recorded | edited / unstamped / removed | yes |
| No sources and no citations | unanchored | `--strict` |
| Citation into a language without a parser | unverified | `--strict` |

A page is compared only while its content matches the lock. `drift.Ack` with no targets records only new and edited pages; an unchanged stale page is re-recorded only when named explicitly. `drift.Stamp` refuses pages with broken citations. `check` is read-only.

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
