---
title: Checking docs in CI
order: 3
sources:
  - internal/cli/check.go
  - internal/cli/ack.go
  - internal/drift/**
---

# Checking docs in CI

`indoc check` compares every page with `.indoc/lock.json` and exits 1 when the docs no longer match the code. It never writes anything (`internal/cli/check.go::runCheck`).

## GitHub Actions

```yaml
  docs:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: byabasaija/indoc@v0.5.0
```

The action downloads the matching indoc release, verifies its checksum, and runs `indoc check` in the repository root. Inputs:

| Input | Default | Meaning |
|---|---|---|
| `mode` | `default` | `strict` also fails on drift; `warn` prints the same report and always passes |
| `version` | the action's tag | indoc version to run |
| `working-directory` | `.` | Directory containing `.indoc/` |

Use `mode: warn` while trying it out. Without the action, install indoc any way you like and run `indoc check`.

## Reading the report

```
documentation/dev/checkout.md
  ✗ STALE      shop/checkout/service.py::CheckoutService.charge  cited symbol's signature changed
    DRIFT      shop/checkout/session.py  implementation changed; signatures unchanged
```

Lines marked `✗` fail the check. With `--strict`, drift, unanchored pages, and unverifiable citations fail too.

| Kind | Meaning | Fix |
|---|---|---|
| stale | A cited signature, or a public signature under `sources`, changed (or any cited change on a `strict` page) | Update the page, or `indoc ack <page>` if it is still correct |
| broken | A citation or `sources` glob does not resolve | Fix the citation |
| edited | The page changed since it was recorded | `indoc ack` |
| unstamped | A new page | `indoc ack` |
| removed | A recorded page was deleted | `indoc ack` |
| drift | Implementation, private helpers, or tests changed; public signatures did not | Review; fails only with `--strict` |
| unanchored | No `sources` and no citations | Add them; fails only with `--strict` |

## Recording pages

`internal/drift/drift.go::Ack` updates the lock:

- `indoc ack` records new and edited pages and forgets deleted ones. It **never** re-records an unchanged page, so it cannot silence a stale warning.
- `indoc ack documentation/page.md` records that page even if unchanged: "I reviewed it against the new code and it is still correct."

`indoc bundle` performs the first form automatically after building. A page with a broken citation can never be recorded (`internal/drift/drift.go::Stamp`).

## Upgrading indoc

A new indoc release can parse code differently, for example after a grammar update, which changes the hashes it computes. The lock records which version wrote each entry and the exact bytes of each file (`internal/drift/drift.go::FileLock`):

- **File unchanged since it was recorded:** nothing to report. The next `indoc ack` or `indoc bundle` re-records it with the new hashes.
- **File changed:** indoc cannot compare signatures across versions, so the change is reported as drift, which does not fail the check. Review the page and run `indoc ack <page>`.

Upgrading indoc never turns a passing check into a failing one.
