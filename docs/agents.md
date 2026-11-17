---
title: Using indoc from a coding agent
order: 5
sources:
  - internal/cli/mcp.go
  - internal/mcp/**
  - plugin/**
---

# Using indoc from a coding agent

The indoc plugin for Claude Code and GitHub Copilot CLI gives your agent indoc's operations as MCP tools, plus a skill that says how to use them. Ask it to "document this project" or "update the stale docs".

## Install

Claude Code:

```
/plugin marketplace add byabasaija/indoc
/plugin install indoc@indoc
```

GitHub Copilot CLI:

```
copilot plugin install byabasaija/indoc:plugin
```

On first use the plugin's launcher downloads the indoc release matching the plugin version for your platform, checks it against the release's `checksums.txt`, and caches it in the plugin's data directory. A download that does not match its checksum is never run. To use a local build instead, set `INDOC_BIN` to its path.

## Tools

`indoc mcp` runs the MCP server on stdin and stdout (`internal/cli/mcp.go::mcpServer`); any MCP client can start it directly.

| Tool | Does | Changes files |
|---|---|---|
| `indoc_scan` | Pages, check findings with reasons, extracted reference, extractor coverage, and a project map. Sets the project up on first use. | Only on first use |
| `indoc_setup` | Creates `.indoc/config.yaml`, or sets which Markdown files are published | Yes |
| `indoc_check` | Findings and the number that would fail CI | No |
| `indoc_ack` | Records pages as verified | Yes |
| `indoc_bundle` | Builds the site; refuses broken citations | Yes |

When a repository already has Markdown, `indoc_scan` lists it and publishes nothing (`internal/cli/mcp.go::setup`); the agent asks you which files to publish and passes them to `indoc_setup`. Each tool takes an optional `root`; otherwise it uses `CLAUDE_PROJECT_DIR` or the directory the server started in.

Check failures are results, not tool errors: `indoc_check` returns `failing` so the agent can fix the pages. Tool errors (a broken citation during a build, an invalid config) are returned with `isError` so the agent can act on them.

## Protocol

The server (`internal/mcp/mcp.go::Server.Serve`) speaks both MCP eras: the `initialize` handshake used by protocol versions 2024-11-05 through 2025-11-25, and the 2026-07-28 model, where each request carries its version in `_meta` and clients start with `server/discover`. Messages are one JSON object per line; nothing else is ever written to stdout.
