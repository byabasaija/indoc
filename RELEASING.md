# Releasing

The plugin and the binary ship separately:

- **Plugin files** (skill, MCP config, launcher) are installed from the `main` branch.
- **The binary** is downloaded by the launcher from the GitHub release whose version it pins.

So `main` must never pin a version whose release doesn't exist yet, or every plugin install fails until the release finishes building. **Tag first, push `main` last.**

## Steps

1. Bump the version in all four places (the release workflow refuses a tag that doesn't match the plugin):
   - `plugin/bin/indoc`: `VERSION="X.Y.Z"`
   - `plugin/.claude-plugin/plugin.json`: `"version": "X.Y.Z"`
   - `internal/version/version.go`: `Version = "X.Y.Z-dev"`
   - The `byabasaija/indoc@vX.Y.Z` examples in `README.md`, `documentation/ci.md`, and `plugin/skills/indoc/SKILL.md`
2. Run `indoc bundle`, then `indoc check --strict`, and `go test ./...`. The launcher change drifts `documentation/agents.md`; `indoc ack` it once you've confirmed it's still right.
3. If the release changes behavior users rely on, write `.github/release-notes/vX.Y.Z.md` with an **Upgrading** section; the workflow uses it instead of generated notes.
4. Commit (`chore: release X.Y.Z`) and tag the commit. **Do not push `main` yet.**
   ```sh
   git tag -a vX.Y.Z -m "indoc X.Y.Z: <one line>"
   git push origin vX.Y.Z
   ```
5. Wait for the **Release** workflow to finish and confirm the release has six archives, `install.sh`, and `checksums.txt`.
6. Push `main`. Plugin installs now pin a release that exists.
   ```sh
   git push origin main
   ```
7. Check the launcher against the published release:
   ```sh
   CLAUDE_PLUGIN_DATA=$(mktemp -d) sh plugin/bin/indoc version
   ```

If the release workflow fails, nothing has reached plugin users: fix it, delete the tag (`git push --delete origin vX.Y.Z`), and tag again.

## Versions

Before 1.0: a new feature or changed default bumps the middle number (0.3 → 0.4); a fix bumps the last (0.3.0 → 0.3.1).
