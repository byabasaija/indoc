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
      - uses: actions/setup-go@v5
        with:
          go-version: stable
      - run: go install github.com/byabasaija/indoc/cmd/indoc@latest
      - run: indoc check
```

Use `indoc check --warn` while trying it out: it prints the same report and always exits 0.

## Reading the report

```
docs/dev/checkout-routing.md
  ✗ STALE      shop/core/services/checkout/dispatcher.py::CheckoutDispatcher.dispatch  cited symbol's signature changed
    DRIFT      shop/core/services/checkout/session.py  implementation changed; signatures unchanged
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
- `indoc ack docs/page.md` records that page even if unchanged: "I reviewed it against the new code and it is still correct."

`indoc bundle` performs the first form automatically after building. A page with a broken citation can never be recorded (`internal/drift/drift.go::Stamp`).
