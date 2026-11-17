#!/bin/sh
# Builds one WASI module per grammar from the pinned sources in grammars.txt.
# Runs inside the Dockerfile's image; use `make grammars` from the repo root.
set -eu
here=$(cd "$(dirname "$0")" && pwd)
out="$here/../wasm"
src=/tmp/indoc-grammars
mkdir -p "$out" "$src"

fetch() { # repo commit dir
  if [ ! -d "$3/.git" ]; then
    git init -q "$3"
    git -C "$3" remote add origin "$1"
  fi
  git -C "$3" fetch -q --depth 1 origin "$2"
  git -C "$3" checkout -q FETCH_HEAD
  test "$(git -C "$3" rev-parse HEAD)" = "$2"
}

grep -v '^#' "$here/grammars.txt" | while read -r name repo commit sub fn; do
  [ -n "$name" ] || continue
  dir="$src/$(basename "$repo")"
  fetch "$repo" "$commit" "$dir"
  if [ "$name" = runtime ]; then
    runtime="$dir/lib"
    echo "$runtime" > "$src/runtime"
    continue
  fi
  runtime=$(cat "$src/runtime")
  gsrc="$dir/$sub/src"
  scanner=""
  [ -f "$gsrc/scanner.c" ] && scanner="$gsrc/scanner.c"
  /opt/wasi-sdk/bin/clang --target=wasm32-wasip1 -Os -mexec-model=reactor \
    -D_POSIX_C_SOURCE=200112L -D_DEFAULT_SOURCE -DLANGUAGE_FN="$fn" \
    -I"$runtime/include" -I"$runtime/src" -I"$gsrc" \
    -Wl,--strip-all -Wl,-z,stack-size=1048576 -Wl,--export=malloc \
    -o "$out/$name.wasm" \
    "$runtime/src/lib.c" "$gsrc/parser.c" $scanner "$here/wrapper.c"
  # Committed compressed (about 7x smaller); -n keeps the output reproducible.
  gzip -9 -n -f "$out/$name.wasm"
  printf '%-11s %8d bytes (gzip)\n' "$name" "$(wc -c < "$out/$name.wasm.gz")"
done
