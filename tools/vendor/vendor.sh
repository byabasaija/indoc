#!/bin/sh
# Rebuilds the vendored browser libraries in internal/site/assets/vendor from
# pinned npm versions. Run with `make vendor`; the output is committed.
set -eu
PRISM=1.30.0
MERMAID=11.17.2
out="$(cd "$(dirname "$0")/../.." && pwd)/internal/site/assets/vendor"
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
cd "$work"
npm init -y >/dev/null
npm install --ignore-scripts --no-audit --no-fund "prismjs@$PRISM" "mermaid@$MERMAID" >/dev/null

# Prism core and the languages documentation usually shows, in dependency order.
langs="markup css clike javascript typescript jsx tsx bash json yaml python go sql http diff toml docker ini markdown"
{
  echo "/* Prism $PRISM (https://prismjs.com), MIT License, Copyright (c) 2012 Lea Verou. Languages: $langs */"
  echo "window.Prism = window.Prism || {}; window.Prism.manual = true;"
  cat node_modules/prismjs/components/prism-core.min.js
  echo
  for l in $langs; do
    cat "node_modules/prismjs/components/prism-$l.min.js"
    echo
  done
} > "$out/prism.js"

{
  echo "/* Mermaid $MERMAID (https://mermaid.js.org), MIT License, Copyright (c) 2014 - 2022 Knut Sveidqvist */"
  cat node_modules/mermaid/dist/mermaid.min.js
} | gzip -9 -n > "$out/mermaid.min.js.gz"

wc -c "$out/prism.js" "$out/mermaid.min.js.gz"
