#!/bin/sh
# Records design/demo.gif from design/demo.tape in a throwaway copy of the
# repository, using indoc built from the current source. Needs vhs
# (brew install vhs).
set -eu
repo=$(git rev-parse --show-toplevel)
tmp=$(mktemp -d)
work="$tmp/indoc"
git -C "$repo" worktree add -q --detach "$work" HEAD
trap 'git -C "$repo" worktree remove --force "$work"; rm -rf "$tmp"' EXIT
(cd "$repo" && go build -o "$tmp/bin/indoc" ./cmd/indoc)
(cd "$work" && PATH="$tmp/bin:$PATH" vhs "$repo/design/demo.tape")
mv "$work/demo.gif" "$repo/design/demo.gif"
echo "wrote design/demo.gif ($(du -h "$repo/design/demo.gif" | cut -f1))"
