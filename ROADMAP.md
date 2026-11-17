# Roadmap

indoc does one thing: keep documentation correct as code changes. This page says what works today, what's next, and what indoc will not become.

Want something here sooner? [Open an issue](https://github.com/byabasaija/indoc/issues) with a link to a real repository that needs it. That's what decides the order.

## Shipped

| Version | What it added |
|---|---|
| **0.3** | New pages go in `documentation/`, so plans and agent notes in `docs/` are never mixed in or published; a `curl \| sh` installer (0.3.1) |
| **0.2** | A pre-rendered docs site: search, dark mode, highlighting, copy buttons, Mermaid, landing page, edit links |
| **0.1** | `check` and the lock; `path::Symbol` citations; Python, JavaScript/TypeScript, and Go parsers; routes, env vars, and commands from Django, DRF, FastAPI, Express, Next.js, NestJS, Astro, and Cobra; MCP server and plugin for Claude Code and Copilot CLI; GitHub Action |

## Next

- **Pull request comments.** The GitHub Action posts which pages went stale and why, instead of only failing.
- **More frameworks:** Flask blueprints, Rails, Laravel, Spring. Each lands with a test against the framework's own route table, like the others ([how extractors are verified](documentation/accuracy.md#extractors-are-routes-complete)).
- **OpenAPI.** Read a committed OpenAPI schema to add request and response details to route pages.
- **Descriptions for reference entries.** An optional `.indoc/reference.yaml` to describe routes and variables in your own words, with entries that no longer exist reported.
- **Command checks.** Fail when a page tells readers to run `make x` or `npm run x` and that target no longer exists.

## Later, if users ask

- Another language (Rust, Java, C#, PHP, Ruby). Adding one is a pinned grammar, its symbol rules, and a test against the language's own parser, but every grammar adds binary size, so each waits for real demand.
- Versioned docs. Git tags already version the Markdown.

## Not doing

Saying no keeps indoc small enough to be excellent at one thing.

| Not doing | Why |
|---|---|
| Calling an AI model | The agent you already use writes. indoc checks. No API keys, no billing, nothing to leak. |
| Running or importing your code | It would need your dependencies, database, and secrets, and is unsafe in CI. Analysis is static. |
| Page templates or fixed outlines | Templates are why docs sites all read the same. Your agent proposes an outline from your code; you approve it. |
| Plugins, MDX, React components | Every extension point is maintenance forever. `custom.css`, `head.html`, and `allow_html` cover styling and embeds. If you need interactive widgets in docs, Docusaurus is the right tool. |
| Translations | Translated pages go stale twice as fast. Possible later, anchored to code like any other page. |
| Publishing a folder by default | Publishing is opt-in, per glob. Internal notes sit next to user docs in most repositories. |
| Network access during a build | Builds are deterministic and work offline. Live data such as star counts loads in the browser. |
| Replacing OpenAPI or Swagger | drf-spectacular and FastAPI already produce exact schemas. indoc links to them. |
| Hosting | The output is static files. Any host works. |
