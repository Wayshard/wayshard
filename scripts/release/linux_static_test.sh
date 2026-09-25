#!/usr/bin/env bash
# Release gate: the Linux server and CLI must be statically linked.
#
# The release builds cross-compile from Linux with CGO_ENABLED=0, so every Linux
# artifact runs on any distro (musl/Alpine, minimal containers) without glibc
# coupling. The host `linux/amd64` build would otherwise pick up glibc (cgo) and
# diverge from the static `linux/arm64` cross build. This asserts the amd64 host
# build is static too.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"

if [[ "$(uname -s)" != "Linux" ]]; then
  echo "linux static-linkage check skipped on $(uname -s)"
  exit 0
fi

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

for pkg in ./cmd/wayshard-server ./cmd/wayshard; do
  out="$TMP/$(basename "$pkg")"
  ( cd "$ROOT" && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o "$out" "$pkg" )
  info="$(ldd "$out" 2>&1 || true)"
  if grep -qiE "not a dynamic executable|statically linked" <<<"$info"; then
    echo "  $(basename "$pkg") linux/amd64: static"
  else
    echo "$(basename "$pkg") is dynamically linked (should be static):" >&2
    echo "$info" >&2
    exit 1
  fi
done
echo "linux static-linkage test ok"
