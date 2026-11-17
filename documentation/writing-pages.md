---
title: Writing pages
order: 2
sources:
  - internal/pages/**
  - internal/code/**
---

# Writing pages

A page is a Markdown file with optional YAML frontmatter, parsed by `internal/pages/pages.go::Parse`.

```markdown
---
title: How checkout sessions are routed
order: 2
audience: dev
sources:
  - shop/core/services/checkout/**
---

Every request lands in
`shop/core/services/checkout/dispatcher.py::CheckoutDispatcher.dispatch`.
```

## Frontmatter

| Key | Meaning |
|---|---|
| `title` | Page title. Falls back to the first `# heading`, then the file name. |
| `order` | Sort position. Pages without one sort after, by path. |
| `audience` | Optional reader group, such as `partner` or `dev`. |
| `sources` | Globs of code this page describes. `**` matches any depth. |
| `strict` | `true` makes body changes to cited symbols fail the check. |
| `description` | One line for search results and link previews. |
| `layout` | `landing` on the home page adds a hero and feature cards; see [Customizing the site](site.md). |

Docusaurus keys `sidebar_label` and `sidebar_position` are read too, so an existing Docusaurus docs folder works unchanged.

The top-most `index.md` or `README.md` becomes the overview.

## Citations

Write `path::Symbol` anywhere in the text, usually in backticks: `internal/drift/drift.go::Stamp`. Methods and nested definitions use dots: `internal/drift/drift.go::Finding.Fails`.

A citation does two things:

1. **It is checked.** If the file or symbol does not exist, `indoc check` reports it as broken and `indoc bundle` refuses to build.
2. **It is an anchor.** A signature change makes the page stale. A body change is reported as drift, because most implementation changes do not affect what a page says.

For pages whose readers depend on exact behavior, such as a partner API or a CLI's output, add `strict: true` to the frontmatter. On those pages any change to a cited symbol, including its body, fails the check.

Cite the symbols a page actually explains. Citations inside fenced code blocks are ignored.

3. **It becomes a link.** When `source_url` is set in `.indoc/config.yaml`, `indoc bundle` turns each citation into a link to the symbol's current line (`internal/pages/pages.go::Linkify`). Line numbers are resolved at build time, so links stay right as code moves.

## Sources

`sources:` covers more code than you cite. Under those globs, a page goes stale when a public signature is added, removed, or changed, or a file is added or removed. Body-only changes, changes to private helpers (`_name` in Python, lowercase names in Go), and changes to test files and fixtures (`tests/`, `testdata/`, `*_test.go`, `test_*.py`, `*.spec.ts`, ...), including new ones, are reported as drift without failing CI.

Docstrings, comments, logging calls (`logger.info(...)`), and formatting, including trailing commas, are ignored everywhere.

## Language support

Precise staleness and symbol citations need a parser for the language. `internal/code/code.go::Analyze` currently parses:

| Language | Status |
|---|---|
| JavaScript, TypeScript, JSX, TSX | Supported through tree-sitter. Agrees with the TypeScript compiler on 56,396 symbols across 9,436 files. Exported declarations are public. |
| Python | Supported through tree-sitter. Agrees with CPython's own parser on 454,837 symbols across 34,120 of 34,121 files. `_name` is private. |
| Go | Supported, via the Go standard library parser. Lowercase names are private. |
| Others | Files are still tracked, but any change is reported as drift, and `path::Symbol` citations cannot be verified. |
