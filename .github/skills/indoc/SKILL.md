---
name: indoc
description: Generate or update a repository's user documentation with indoc. Use when asked to document a project, refresh its docs after code changes, or build its indoc site.
---

# Generate documentation with indoc

Use the indoc CLI to scan project facts and bundle an agent-authored documentation site. Do not invent reference facts or create a page for every source file.

## Workflow

1. Check that `indoc` is available. If it is not installed, tell the user how to install it; do not install software without their approval.
2. If `.indoc.yaml` is missing, run `indoc init`. Do not use `--force` to replace an existing configuration.
3. Read `.indoc.yaml`, `.indoc/AGENT_GUIDE.md`, and `.indoc/site.schema.json`.
4. Run `indoc scan --json`. Use the report and inspect relevant source files to understand the project and any changes.
5. Create or update `.indoc/site.json`:
   - Set `project_name` from `.indoc.yaml`.
   - Write a concise overview and a small set of task-oriented guides appropriate to the project. For a CLI, cover installation, common commands, and workflows; for a service or library, cover its important setup and usage paths.
   - Use `previous` as context when updating prose, and verify claims against the current source.
   - Copy the report's complete `reference` array without changing or inventing entries. If it is empty, write `[]`.
   - Review `stale_reference`; update narrative claims affected by changed or deleted sources.
   - Keep the file valid JSON matching `.indoc/site.schema.json`.
6. Run `indoc bundle --input .indoc/site.json`. Fix any validation errors and rerun the bundle.
7. Report the configured output directory. Mention `indoc serve` as an optional local preview; do not start a long-running preview server unless the user asks.

If `.indoc/site.json` is ignored by Git, tell the user so the source document can be committed; do not silently change ignore rules.
