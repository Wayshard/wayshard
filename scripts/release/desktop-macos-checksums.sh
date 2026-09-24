#!/usr/bin/env bash
# Deterministically write SHA256SUMS-desktop-macos.txt for BOTH macOS DMGs.
#
# The desktop matrix builds Apple silicon (aarch64) and Intel (x86_64) on
# separate runners. Generating a per-leg SHA256SUMS-desktop-macos.txt made the
# two runners race to upload the same asset name, so the published file could
# end up listing only one architecture. This runs once, after the whole matrix,
# and fails closed unless exactly the expected aarch64 and x86_64 DMGs are
# present. checksums.py also rejects a duplicate basename.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
VERSION="${1:?usage: desktop-macos-checksums.sh <version> <assets-dir> <outfile>}"
ASSETS="${2:?}"
OUT="${3:?}"

expected=(
  "wayshard-desktop-${VERSION}-macos-aarch64.dmg"
  "wayshard-desktop-${VERSION}-macos-x86_64.dmg"
)

mapfile -t dmgs < <(find "$ASSETS" -maxdepth 1 -type f \
  -name "wayshard-desktop-${VERSION}-macos-*.dmg" | sort)

if [[ ${#dmgs[@]} -ne ${#expected[@]} ]]; then
  echo "expected exactly ${#expected[@]} macOS DMGs, found ${#dmgs[@]}:" >&2
  printf '  %s\n' "${dmgs[@]:-<none>}" >&2
  exit 1
fi

for name in "${expected[@]}"; do
  matches=0
  for f in "${dmgs[@]}"; do
    [[ "$(basename "$f")" == "$name" ]] && matches=$((matches + 1))
  done
  if [[ "$matches" -ne 1 ]]; then
    echo "expected exactly one $name, found $matches" >&2
    exit 1
  fi
done

python3 "$ROOT/scripts/release/checksums.py" --out "$OUT" "${dmgs[@]}"

rows="$(grep -c . "$OUT")"
if [[ "$rows" -ne ${#expected[@]} ]]; then
  echo "SHA256SUMS-desktop-macos.txt must have exactly ${#expected[@]} rows, got $rows" >&2
  exit 1
fi
for name in "${expected[@]}"; do
  count="$(grep -c "  ${name}$" "$OUT" || true)"
  if [[ "$count" -ne 1 ]]; then
    echo "checksum row for $name appears $count times (must be exactly once)" >&2
    exit 1
  fi
done

echo "wrote $OUT with both macOS architectures exactly once"
