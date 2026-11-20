#!/bin/sh
# Records design/<name>.gif from design/<name>.tape in a throwaway copy of
# the repository, using indoc built from the current source. Needs vhs
# (brew install vhs).
#
#   design/record-demo.sh              # demo-check: indoc check failing
#   design/record-demo.sh <name>       # any other design/<name>.tape
set -eu
name=${1:-demo-check}
repo=$(git rev-parse --show-toplevel)
tmp=$(mktemp -d)
work="$tmp/indoc"
git -C "$repo" worktree add -q --detach "$work" HEAD
trap 'git -C "$repo" worktree remove --force "$work"; rm -rf "$tmp"' EXIT
(cd "$repo" && go build -o "$tmp/bin/indoc" ./cmd/indoc)
(cd "$work" && PATH="$tmp/bin:$PATH" vhs "$repo/design/$name.tape")
mv "$work/$name.gif" "$repo/design/$name.gif"
echo "wrote design/$name.gif ($(du -h "$repo/design/$name.gif" | cut -f1))"
