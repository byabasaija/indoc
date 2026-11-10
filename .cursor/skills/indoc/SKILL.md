---
name: indoc
description: >-
  Compile project documentation into a static site with indoc. Use when the
  user asks to document a repository, generate docs, update indoc pages, or
  refresh ./static/docs.
---

# Document this project with indoc

The agent writes overview and guides. indoc extracts routes and environment variables. Do not invent reference entries, and do not add an API client.

## Steps

1. From the project root, run `indoc scan --json`. Parse stdout as JSON.
2. Read `previous` when it is present. Edit those pages. Rewrite a guide only when a path in `added`, `modified`, or `deleted` changes what that guide says.
3. Write `.indoc/site.json`:
   - `project_name` from the scan report
   - `overview.markdown` for what the system does and how a request moves through it
   - `guides` for tasks a reader came to finish (run it, auth, one hard flow). A few pages. No page per file.
   - `reference` copied from the scan's `reference` array, unchanged. Ignore `stale_reference` when choosing entries; it only says which old citations moved.
4. Run `indoc bundle --input .indoc/site.json --out <output_dir>` using `output_dir` from the scan (default `./static/docs`).
5. Run `indoc serve` and open the overview, one guide, and the routes table.

Overview and guide markdown start with the prose. The viewer already prints the page title.

## Mount

If the app does not already serve the output directory, show this FastAPI mount and do not edit the app unless asked:

```python
app.mount("/docs", StaticFiles(directory="static/docs", html=True), name="docs")
```
