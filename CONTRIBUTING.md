# Contributing to indoc

Thanks for helping. indoc has one job: keep documentation correct as code changes. Contributions that make that more accurate, or bring it to more projects, are the most welcome.

## Reporting

- **A bug:** use the [bug report](https://github.com/byabasaija/indoc/issues/new?template=bug_report.yml) form, with the output of `indoc version` and the full command output.
- **`indoc check` got it wrong:** use [wrong check result](https://github.com/byabasaija/indoc/issues/new?template=wrong_check.yml), with the code diff and the page. These reports are how the staleness rules get tuned.
- **A framework or language:** use [framework or language request](https://github.com/byabasaija/indoc/issues/new?template=framework_request.yml), with a link to a real public repository that uses it. That link decides the order.

Questions, ideas, and "here's what I built with indoc" go in [Discussions](https://github.com/byabasaija/indoc/discussions). For anything larger than a fix, open an issue first so we can agree on the approach before you write it. [ROADMAP.md](ROADMAP.md) lists what's planned and what indoc deliberately won't do.

## Setting up

You need Go (the version in `go.mod`) and nothing else. The tree-sitter grammars and browser libraries are committed.

```sh
git clone https://github.com/byabasaija/indoc
cd indoc
go test ./...                       # all tests
go run ./cmd/indoc check --strict   # indoc's own docs must match its code
go run ./cmd/indoc preview          # its docs site on http://127.0.0.1:4173
```

[ARCHITECTURE.md](ARCHITECTURE.md) maps the packages and explains the decisions behind them. Read its "Decisions" section before changing how staleness works.

## Making a change

1. Keep the change focused: one fix or feature per pull request.
2. Add or update tests. Every rule in `internal/drift` and every extractor in `internal/reference` has tests that show what it does; a change without one is hard to accept.
3. Run `go test ./...` and `gofmt -l .` (it should print nothing).
4. **Keep the docs true.** indoc's own documentation in `documentation/` cites its code, so `go run ./cmd/indoc check --strict` fails when you change code a page describes. Update the page, or run `go run ./cmd/indoc ack <page>` if it is still correct. Describe new user-facing behavior there too.
5. Open the pull request and say what changes for users, and why.

## Adding a framework

Route extractors live in `internal/reference`, one file per framework (`django.go`, `fastapi.go`, `express.go`, `nest.go`, `fsroutes.go` for file-based routing). An extractor reads source statically; indoc never imports or runs the user's code.

A new extractor needs two kinds of tests:

- **A fixture test** (`express_test.go` is a good model): a small in-memory project that uses every construct the extractor supports, and the exact routes expected, including HTTP methods and handlers.
- **A differential test** against the framework itself (`django_differential_test.go`): a script under `testdata/` asks the framework for its real route table, and the test compares it with indoc's, route for route. It is opt-in through environment variables, so CI does not need the framework installed. Run it on at least one real project and put the result in the pull request.

Routes the extractor cannot resolve statically are reported as coverage warnings, never guessed. [How accurate is indoc?](documentation/concepts/accuracy.md) shows the bar the existing extractors meet.

## Adding a language

Symbol parsers live in `internal/lang`. A parser must find every function, class, and method with its exact start and end lines, and is compared symbol by symbol with the language's own compiler over a large corpus (`internal/lang/python/differential_test.go`). Python and JavaScript/TypeScript use tree-sitter grammars compiled to WebAssembly; `make grammars` rebuilds them with Docker when a grammar is added or updated.

## Style

- Go: `gofmt`, standard library first, small packages with one job. Comments say why, not what.
- Docs: short sentences, real commands, real examples. Write for the reader who has never seen indoc.
- No private code in tests, fixtures, or docs: use small invented examples (`shop/checkout/service.py::CheckoutService.charge`).

## Releases

Maintainers cut releases; [RELEASING.md](RELEASING.md) has the steps.

## License

By contributing, you agree that your contributions are licensed under the [MIT License](LICENSE).
