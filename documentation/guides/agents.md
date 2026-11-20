---
title: Using indoc from a coding agent
order: 13
sources:
  - internal/cli/mcp.go
  - internal/mcp/**
  - plugin/**
---

# Using indoc from a coding agent

The indoc plugin for Claude Code gives your agent indoc's operations as MCP tools, plus a skill that says how to use them. Ask it to "document this project" or "update the stale docs".

## Install

Claude Code, in a terminal session (`claude`; the editor extensions do not run `/plugin`):

```
/plugin marketplace add byabasaija/indoc
/plugin install indoc@indoc
```

On first use the plugin's launcher downloads the indoc release matching the plugin version for your platform, checks it against the release's `checksums.txt`, and caches it in the plugin's data directory. A download that does not match its checksum is never run. To use a local build instead, set `INDOC_BIN` to its path.

## Tools

`indoc mcp` runs the MCP server on stdin and stdout (`internal/cli/mcp.go::mcpServer`); any MCP client can start it directly. Plugins for other agents (GitHub Copilot CLI, Cursor, OpenAI Codex, Gemini CLI) are planned; see the [roadmap](https://github.com/byabasaija/indoc/blob/main/ROADMAP.md#next).

| Tool | Does | Changes files |
|---|---|---|
| `indoc_scan` | Pages, check findings with reasons, extracted reference, extractor coverage, and a project map. Sets the project up on first use. | Only on first use |
| `indoc_setup` | Creates `.indoc/config.yaml`, sets which Markdown files are published, the logo, and whether the site links to the code (`source_links`) | Yes |
| `indoc_check` | Findings and the number that would fail CI | No |
| `indoc_ack` | Records pages as verified | Yes |
| `indoc_bundle` | Builds the site; refuses broken citations, and hand edits in the output until you choose to keep or discard them | Yes |
| `indoc_publish` | Saves where the docs are served and returns the steps, including code for your app | Yes |

## What the agent does on a first run

1. Scans the project: frameworks, entry points, real commands, existing Markdown, and images that look like a logo.
2. Asks which existing Markdown to publish, if there is any.
3. Asks whether the docs' readers can open the repository, and only then turns on links to the code.
4. Asks which logo to use, offering the candidates it found (and warning when one is a tall lockup that will be unreadable in the header), or asks you for one. If the project has none, it offers to design a simple square SVG mark and shows it to you before using it. `indoc_setup` checks the file exists before writing it to the config (`internal/cli/mcp.go::setLogo`).
5. Proposes the readers and an outline, each page labeled as a tutorial, how-to, reference, or explanation and placed in a section folder (get started, guides, reference, concepts), with a landing page, and waits for your approval. Each question is one decision; the outline is shown inside the approval question, so you always see what you approve.
6. Writes the pages with citations, builds the site, and runs the check.

When a repository already has Markdown, `indoc_scan` lists it and publishes nothing (`internal/cli/mcp.go::setup`); the agent asks you which files to publish and passes them to `indoc_setup`. Each tool takes an optional `root`; otherwise it uses `CLAUDE_PROJECT_DIR` or the directory the server started in.

Check failures are results, not tool errors: `indoc_check` returns `failing` so the agent can fix the pages. Tool errors (a broken citation during a build, an invalid config) are returned with `isError` so the agent can act on them.

## Protocol

The server (`internal/mcp/mcp.go::Server.Serve`) speaks both MCP eras: the `initialize` handshake used by protocol versions 2024-11-05 through 2025-11-25, and the 2026-07-28 model, where each request carries its version in `_meta` and clients start with `server/discover`. Messages are one JSON object per line; nothing else is ever written to stdout.
