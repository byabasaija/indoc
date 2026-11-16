# Writing documentation with indoc

Use this guide when creating or updating `.indoc/site.json`.

## Workflow

1. Read `.indoc.yaml` and inspect the project source.
2. Run `indoc scan --json` and use its report as the source of truth for extracted facts and changes.
3. Write or update `.indoc/site.json` to match `.indoc/site.schema.json`.
4. Run `indoc bundle --input .indoc/site.json`. Fix any validation errors and bundle again.
5. The generated static site is in the configured output directory. `indoc serve` is an optional local preview, not required to use or publish the site.

## Writing useful pages

- Set `project_name` to the project name in `.indoc.yaml`.
- Write an overview explaining what the project does and how its important flow works.
- Add a small number of task-oriented guides for work a reader is likely to do. For a CLI, prioritize installation, running commands, and common workflows. For a service or library, cover its important setup and usage paths.
- Use Markdown in each page's `markdown` field. Keep the docs focused; do not make a page for every source file.
- When updating docs, use `previous` in the scan report as context, but verify claims against the current source.

## Reference facts

- Copy the scan report's entire `reference` array into `.indoc/site.json` without changing its entries. Do not invent routes, environment variables, source citations, or other reference rows.
- If the report has no reference entries, use an empty array.
- Review `stale_reference` and the file changes when updating the narrative. The current `reference` array is the authoritative extracted reference for the new bundle.

Write valid JSON only in `.indoc/site.json`; do not include comments or explanatory text outside the JSON document.
