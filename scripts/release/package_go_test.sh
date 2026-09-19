#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
mkdir -p "$TMP/bin" "$TMP/out"
for f in \
  wayshard-server-linux-amd64 wayshard-server-linux-arm64 \
  wayshard-server-darwin-amd64 wayshard-server-darwin-arm64 \
  wayshard-server-windows-amd64.exe \
  wayshard-linux-amd64 wayshard-linux-arm64 \
  wayshard-darwin-amd64 wayshard-darwin-arm64 \
  wayshard-windows-amd64.exe
do
  echo "$f" > "$TMP/bin/$f"
done
bash "$ROOT/scripts/release/package-go.sh" v9.9.9 "$TMP/bin" "$TMP/out"
test -f "$TMP/out/wayshard-server-v9.9.9-linux-amd64"
test -f "$TMP/out/wayshard-v9.9.9-windows-amd64.exe"
python3 "$ROOT/scripts/release/checksums.py" --out "$TMP/out/SHA256SUMS-go.txt" "$TMP/out"/*
# 10 binaries; checksums skip SHA256SUMS itself
rows="$(grep -c . "$TMP/out/SHA256SUMS-go.txt")"
test "$rows" = "10"
echo "package-go test ok"
