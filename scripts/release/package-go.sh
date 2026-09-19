#!/usr/bin/env bash
# Copy cross-compiled Go binaries to versioned release names.
set -euo pipefail
VERSION="${1:?usage: package-go.sh <version> <srcdir> <destdir>}"
SRC="${2:?}"
DEST="${3:?}"
mkdir -p "$DEST"

copy() {
  local from="$1" to="$2"
  if [[ -f "$from" ]]; then
    cp -a "$from" "$DEST/$to"
    echo "packed $to"
  else
    echo "missing $from" >&2
    exit 1
  fi
}

copy "$SRC/wayshard-server-linux-amd64"     "wayshard-server-${VERSION}-linux-amd64"
copy "$SRC/wayshard-server-linux-arm64"     "wayshard-server-${VERSION}-linux-arm64"
copy "$SRC/wayshard-server-darwin-amd64"    "wayshard-server-${VERSION}-darwin-amd64"
copy "$SRC/wayshard-server-darwin-arm64"    "wayshard-server-${VERSION}-darwin-arm64"
copy "$SRC/wayshard-server-windows-amd64.exe" "wayshard-server-${VERSION}-windows-amd64.exe"
copy "$SRC/wayshard-linux-amd64"            "wayshard-${VERSION}-linux-amd64"
copy "$SRC/wayshard-linux-arm64"            "wayshard-${VERSION}-linux-arm64"
copy "$SRC/wayshard-darwin-amd64"           "wayshard-${VERSION}-darwin-amd64"
copy "$SRC/wayshard-darwin-arm64"           "wayshard-${VERSION}-darwin-arm64"
copy "$SRC/wayshard-windows-amd64.exe"      "wayshard-${VERSION}-windows-amd64.exe"
