#!/bin/sh
# Installs the indoc command.
#
#   curl -fsSL https://github.com/byabasaija/indoc/releases/latest/download/install.sh | sh
#
# A specific version:   ... | sh -s -- v0.3.0
# Where to install:     INDOC_INSTALL_DIR=/some/bin (default: /usr/local/bin if
#                       writable, otherwise ~/.local/bin)
#
# The archive is verified against the release's checksums.txt before anything
# is installed. macOS and Linux; on Windows, download the zip from the releases
# page.
#
# Everything runs inside main, so a download cut off halfway runs nothing.

set -eu

main() {
  repo="byabasaija/indoc"
  version="${1:-${INDOC_VERSION:-latest}}"

  os=$(uname -s | tr '[:upper:]' '[:lower:]')
  case "$os" in
    darwin | linux) ;;
    mingw* | msys* | cygwin*)
      fail "on Windows, download indoc_windows_amd64.zip from https://github.com/$repo/releases" ;;
    *) fail "unsupported operating system: $os" ;;
  esac
  case "$(uname -m)" in
    x86_64 | amd64) arch=amd64 ;;
    arm64 | aarch64) arch=arm64 ;;
    *) fail "unsupported architecture: $(uname -m)" ;;
  esac
  archive="indoc_${os}_${arch}.tar.gz"

  if [ -n "${INDOC_DOWNLOAD_URL:-}" ]; then
    base="$INDOC_DOWNLOAD_URL" # a mirror, or a local folder for testing
  elif [ "$version" = latest ]; then
    base="https://github.com/$repo/releases/latest/download"
  else
    case "$version" in v*) ;; *) version="v$version" ;; esac
    base="https://github.com/$repo/releases/download/$version"
  fi

  dir="${INDOC_INSTALL_DIR:-}"
  if [ -z "$dir" ]; then
    if [ -d /usr/local/bin ] && [ -w /usr/local/bin ]; then
      dir=/usr/local/bin
    else
      dir="$HOME/.local/bin"
    fi
  fi

  tmp=$(mktemp -d)
  trap 'rm -rf "$tmp"' EXIT

  say "downloading indoc ($version, $os/$arch)"
  fetch "$base/$archive" "$tmp/$archive" || fail "could not download $base/$archive"
  fetch "$base/checksums.txt" "$tmp/checksums.txt" || fail "could not download $base/checksums.txt"

  expected=$(grep " $archive\$" "$tmp/checksums.txt" | cut -d' ' -f1)
  actual=$(sha256 "$tmp/$archive")
  if [ -z "$expected" ] || [ "$expected" != "$actual" ]; then
    fail "checksum mismatch for $archive; nothing was installed"
  fi

  tar -xzf "$tmp/$archive" -C "$tmp" indoc
  mkdir -p "$dir"
  chmod +x "$tmp/indoc"
  mv -f "$tmp/indoc" "$dir/indoc" || fail "could not write to $dir (set INDOC_INSTALL_DIR, or run with sudo)"

  say "installed $("$dir/indoc" version 2>/dev/null | head -n 1) to $dir/indoc"
  case ":$PATH:" in
    *":$dir:"*) ;;
    *) say "$dir is not on your PATH. Add this to your shell profile:"
       say "  export PATH=\"$dir:\$PATH\"" ;;
  esac
  say "next: run 'indoc init' in your repository"
}

fetch() {
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "$1" -o "$2"
  elif command -v wget >/dev/null 2>&1; then
    wget -q "$1" -O "$2"
  else
    fail "curl or wget is required"
  fi
}

sha256() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | cut -d' ' -f1
  else
    shasum -a 256 "$1" | cut -d' ' -f1
  fi
}

say() { printf 'indoc: %s\n' "$*" >&2; }
fail() { say "$*"; exit 1; }

main "$@"
