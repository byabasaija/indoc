---
title: Reference extraction
order: 4
sources:
  - internal/reference/**
---

# Reference extraction

`indoc bundle` adds reference pages built from source, so routes, environment variables, and commands are never copied by hand. `internal/reference/reference.go::Extract` runs every extractor that applies and returns the sections plus a `internal/reference/reference.go::Coverage` report for each one.

Extraction is static. indoc reads your code; it never imports or runs it.

## Django and Django REST framework

Starting from `ROOT_URLCONF` in your settings, indoc follows `urlpatterns` through `include()` and DRF routers to every view, and works out each route's HTTP methods from:

- the handler methods a view class and its base classes define (`get`, `post`, ...)
- Django and DRF generic views (`ListCreateAPIView`, `TemplateView`, ...)
- viewsets registered on a router, including `@action` routes and `lookup_field`
- `@api_view(["POST"])` on function views
- `http_method_names` restrictions

Routes inside `if` blocks (often `if settings.DEBUG:`) are marked conditional. A route whose path is `settings.ADMIN_URL` resolves to the value in your settings, with a note when another settings file overrides it. Routes provided by installed packages, such as the Django admin or allauth, appear as one external include each rather than as guesses.

The extractor is tested against Django's own URL resolver on real projects; see `internal/reference/django_differential_test.go`.

## Environment variables

Reads through django-environ (`env("X")`, `env.bool("X", default=False)`, `env.db()`), python-decouple (`config("X")`), `os.environ`, and `os.getenv` are listed with their type, default, or "required". If the repository has a `.env.example`, variables missing from it are marked, and variables in it that nothing reads are reported. `.env` itself is never read.

Defaults that may be secrets are never published: values of variables whose names contain `SECRET`, `PASSWORD`, `TOKEN`, `KEY` and similar are hidden, and credentials inside URLs are masked.

## Management commands

Each `<app>/management/commands/<name>.py` that defines `class Command` is listed with its `help` text and the arguments passed to `add_argument`.

## What could not be resolved

`indoc scan` prints each extractor's coverage. Anything it could not resolve is named, never dropped silently, and it warns about problems worth fixing:

```
reference: 67 routes, 31 config, 3 commands
  django: 70 entries from 15 files, 4 unresolved
  python-env: 31 entries from 188 files
    warning: in .env.example but not read by Python code: PAYMENT_ACCOUNT_CODE, WEB_CONCURRENCY
```

A route that can never be reached, because an earlier pattern matches the same path, is reported as a warning too.
