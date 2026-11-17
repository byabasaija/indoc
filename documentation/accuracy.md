---
title: How accurate is indoc?
order: 7
description: How indoc's staleness rules, parsers, and extractors were measured against real code and the tools they imitate.
sources:
  - tools/replay/**
  - internal/lang/python/differential_test.go
  - internal/lang/jsts/differential_test.go
  - internal/reference/django_differential_test.go
---

# How accurate is indoc?

A check that cries wolf gets switched off, and a reference page with a missing route is worse than none. So every part of indoc that makes a claim about code is measured against real repositories and, where one exists, against the tool it imitates. Every test below can be rerun.

## Staleness: does `check` fail for real reasons?

### What changes in real code

On a private production Django API (411 commits, 1,072 changes to non-test source files):

| Change kind | Share |
|---|---|
| Whitespace or comments only | 3.5% |
| Function bodies only | 67% |
| Signatures, routes, environment variables | 29% |

87% of commits touch source code. If any change to a page's files made it stale, nearly every commit would fail CI, developers would learn to acknowledge warnings blindly, and the check would be worthless. That is why indoc compares symbols, not files, and separates *stale* (fails) from *drift* (reported).

### Replaying history

Seven pages were written for the same codebase as an agent would write them, three of them `strict: true`. Then 100 consecutive commits were replayed, running `check` at each one and re-recording flagged pages as a developer would (`tools/replay/main.go::main`). Two targets:

- **False blocks:** at most 5% of commits fail with nothing wrong in the docs.
- **Precision:** at least 80% of 20 randomly sampled flags are real problems, meaning the change altered behavior or an interface that the page describes or its reader relies on.

| Iteration | Rule change | Commits failing | Sampled precision |
|---|---|---|---|
| 1 | Initial rules | 43% | not sampled |
| 2 | Ignore docstrings and logging calls; a class citation covers its interface | 30% | 10/20 (50%) |
| 3 | Ignore trailing commas; a cited body change is drift unless `strict: true` | 20% | **17/20 (85%)** |
| 4 | Private-helper changes are drift | **17%** | ≥ 85% |

About 3 to 4 of the 17 failing commits had no real finding, so **roughly 3–4% of commits would have blocked CI wrongly**. The rest failed because the docs really were wrong: a security check removed while a page still described it, a changed validation rule, new endpoints and error responses, files moved by a restructure.

## Parsers: are symbols found exactly?

Citations and staleness depend on knowing where every function and class starts and ends. Each parser is compared with the language's own compiler, symbol by symbol (name, qualified path, start and end line):

| Language | Compared with | Corpus | Result |
|---|---|---|---|
| Python | CPython 3.13 `ast` | 34,121 files, including Django, DRF, and the standard library | 34,120 files agree (454,837 symbols) |
| JavaScript, TypeScript, JSX, TSX | The TypeScript compiler | 9,436 files, an app plus a frontend `node_modules` | all 56,396 symbols agree |
| Go | — | uses Go's own `go/parser` | exact by construction |

The one Python miss is CPython's compiler stress test, `test_compile.py`. Minified bundles are reported and skipped rather than compared.

```sh
INDOC_PY_CORPUS=~/code/app INDOC_PYTHON=python3.13 \
  go test ./internal/lang/python -run Differential -v
INDOC_TS_CORPUS=~/code/app INDOC_TS_MODULE=/path/to/node_modules/typescript \
  go test ./internal/lang/jsts -run Differential -v
```

## Extractors: are routes complete?

Each framework extractor is compared with the framework's own route table, route for route, including HTTP methods. indoc itself never imports or runs your code; only these tests do.

| Framework | Compared with | Result |
|---|---|---|
| Django + DRF | Django's URL resolver, in each project's own environment | 6 projects, 524 routes: 0 missing, 0 extra, 0 wrong methods |
| FastAPI | FastAPI's route table, expanded through included routers | a real project (6/6) and a fixture of every supported construct (14/14) |
| Express 4.22 and 5.3 | Routes recorded at runtime by a tracer | 14/14 |
| Next.js 15.5 | `next build` route manifests | 10/10 |
| NestJS 11.2 | Routes Nest registers at startup | 8/8 |
| Astro | Page files and each endpoint's exported methods | 70 page files |
| Cobra | indoc's own runtime command tree | 8 commands, all flags |

Routes that come from installed packages (Django admin, allauth) are listed as external includes rather than guessed. Building these tests found real bugs in the projects too: an API root that could never be reached, now a warning, and secret default values that would have been published, now redacted.

```sh
INDOC_DJANGO_REPO=~/code/app INDOC_DJANGO_PYTHON=~/code/app/.venv/bin/python \
  go test ./internal/reference -run DjangoDifferential -v
```

## Speed and size

| | Result |
|---|---|
| `check` on a mid-size Django API | 0.14 s |
| `scan` on the same | 0.63 s |
| Parsing JavaScript | about 5 MB/s per core; 4,684 files (41 MB) in 3.2 s on 8 cores |
| Startup | 70 ms the first time, 3–4 ms after (compiled grammars are cached) |
| Binary | one static file, no runtime dependencies |
