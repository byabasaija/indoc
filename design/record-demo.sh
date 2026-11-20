#!/bin/sh
# Records design/demo-check.gif from design/demo-check.tape in a throwaway copy of the
# repository, using indoc built from the current source. Needs vhs
# (brew install vhs).
set -eu
repo=$(git rev-parse --show-toplevel)
tmp=$(mktemp -d)
work="$tmp/indoc"
git -C "$repo" worktree add -q --detach "$work" HEAD
trap 'git -C "$repo" worktree remove --force "$work"; rm -rf "$tmp"' EXIT
(cd "$repo" && go build -o "$tmp/bin/indoc" ./cmd/indoc)
(cd "$work" && PATH="$tmp/bin:$PATH" vhs "$repo/design/demo-check.tape")
mv "$work/demo-check.gif" "$repo/design/demo-check.gif"
echo "wrote design/demo-check.gif ($(du -h "$repo/design/demo-check.gif" | cut -f1))"
