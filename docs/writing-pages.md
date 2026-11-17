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

Docusaurus keys `sidebar_label` and `sidebar_position` are read too, so an existing Docusaurus docs folder works unchanged.

The top-most `index.md` or `README.md` becomes the overview.

## Citations

Write `path::Symbol` anywhere in the text, usually in backticks: `internal/drift/drift.go::Stamp`. Methods and nested definitions use dots: `internal/drift/drift.go::Finding.Fails`.

A citation does two things:

1. **It is checked.** If the file or symbol does not exist, `indoc check` reports it as broken and `indoc bundle` refuses to build.
2. **It is an anchor.** Any change to the cited symbol, including its body, makes the page stale.

Cite the symbols a page actually explains. Citations inside fenced code blocks are ignored.

## Sources

`sources:` covers more code than you cite. Under those globs, a page goes stale when a signature is added, removed, or changed, or a file is added or removed. Body-only changes are reported as drift without failing CI.

## Language support

Precise staleness and symbol citations need a parser for the language. `internal/code/code.go::Analyze` currently parses:

| Language | Status |
|---|---|
| Python | Supported. Agrees with CPython's own parser on 409,406 symbols across 29,536 files. |
| Go | Supported, via the Go standard library parser. |
| Others | Files are still tracked, but any change is reported as drift, and `path::Symbol` citations cannot be verified. |
