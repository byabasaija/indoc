---
title: FAQ
order: 31
description: Short answers to the questions people ask before adopting indoc.
sources:
  - internal/drift/**
  - internal/config/config.go
---

# FAQ

## Does indoc use AI?

No. Your coding agent (or you) writes the pages. indoc parses code, records hashes, and builds HTML. It makes no network calls during a build.

## Will it publish my internal notes?

No. Only files matched by `pages:` in `.indoc/config.yaml` are published. Pages default to `documentation/`, not `docs/`, because `docs/` is where plans and agent notes usually collect.

## Won't it fail CI on every commit?

No. Only public signatures and cited symbols fail the check. Body changes are reported as drift and fail only where you ask for it (`strict: true` or `--strict`). Start with `mode: warn` if you want to see it first.

## My language isn't supported.

indoc still tracks the files a page covers and reports any change as drift. Citations need a parser; open an issue with a real repository you'd like supported.

## Where does the site go?

Wherever you choose with `indoc publish`: GitHub Pages (it writes the deploy workflow), any static host, or your own app under a path like `/guide/`, public or login-only. For an app it prints tested code for Django, FastAPI, Flask, Express, NestJS, Next.js, Astro, or Go. See [Publishing the site](../guides/publishing.md).
