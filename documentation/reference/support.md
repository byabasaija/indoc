---
title: Supported languages and frameworks
order: 19
description: Which languages indoc parses for citations, and which frameworks it extracts routes, configuration, and commands from.
sources:
  - internal/lang/**
  - internal/reference/**
---

# Supported languages and frameworks

## Languages

Precise staleness and `path::Symbol` citations need a parser. Everything else works on any repository.

| Language | Parser | Checked against |
|---|---|---|
| JavaScript, TypeScript, JSX, TSX | tree-sitter | The TypeScript compiler: 9,436 files, 56,396 symbols, all matching |
| Python | tree-sitter | CPython's own parser: 34,120 of 34,121 files, 454,837 symbols |
| Go | Go standard library | — |
| Others | — | Files are tracked; any change is reported as drift |

Exported JavaScript/TypeScript declarations are public; Python uses `_name` and Go lowercase names for private. Adding a private helper never fails `check`.

## Frameworks

| Framework | Routes | Configuration | Commands |
|---|---|---|---|
| Django + DRF | ✅ URLconf through `include()`, routers, `@action`; methods from views, viewsets, `@api_view` | ✅ django-environ, python-decouple, `os.environ` | ✅ management commands |
| FastAPI | ✅ decorators, `APIRouter` prefixes, nested `include_router`, websockets, mounts | ✅ pydantic `BaseSettings` | |
| Express | ✅ routes across files, `route()` chains, mounted routers and sub-apps | `process.env` | |
| Next.js | ✅ App Router pages and route handlers, Pages Router pages and API routes | `process.env` | |
| NestJS | ✅ `@Controller` + `@Get`/`@Post`/..., global prefix | `process.env` | |
| Astro | ✅ `src/pages` pages and endpoints | `import.meta.env` | |
| Cobra (Go) | | `os.Getenv` | ✅ command tree, flags, arguments |
| Flask, Go `net/http` | best effort | | |
| Rails, Laravel | planned | | |

Every extractor is tested against the framework's own route table: Django's URL resolver on six real projects (524 routes), FastAPI's router, Express 4 and 5 at runtime, `next build`'s route manifests, the routes Nest registers, and Cobra's command tree. All match route for route, including HTTP methods. Extraction is static: indoc never imports or runs your code.

`indoc scan` also warns about routes that can never be reached and variables missing from `.env.example`.

How each framework is read is described in [How reference is extracted](extraction.md).
