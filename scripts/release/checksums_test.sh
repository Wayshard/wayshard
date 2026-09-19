#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
python3 "$ROOT/scripts/release/checksums.py" --self-test

# Overlapping glob must not duplicate server rows.
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
echo server > "$TMP/wayshard-server-v0.0.0-linux-amd64"
echo cli > "$TMP/wayshard-v0.0.0-linux-amd64"
python3 "$ROOT/scripts/release/checksums.py" --out "$TMP/SHA256SUMS.txt" \
  "$TMP"/wayshard-server-* "$TMP"/wayshard-*
rows="$(grep -c . "$TMP/SHA256SUMS.txt")"
test "$rows" = "2"
grep -c 'wayshard-server-v0.0.0-linux-amd64' "$TMP/SHA256SUMS.txt" | grep -qx 1
echo "checksum overlapping-glob test ok"
