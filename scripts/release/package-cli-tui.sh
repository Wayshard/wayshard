#!/usr/bin/env bash
# Package one coherent CLI+TUI distribution unit for a single platform.
#
# The archive is what a user downloads and extracts. Inside it the binaries use
# the canonical runtime names the launcher expects ("wayshard" and
# "wayshard-tui", with ".exe" on Windows), so no manual rename and no
# WAYSHARD_TUI override are needed. Notices travel inside the archive.
#
# Usage: package-cli-tui.sh <version> <os> <arch> <srcdir> <destdir>
#   <version> release tag, for example v0.1.0
#   <os>      linux | darwin | windows
#   <arch>    amd64 | arm64
#   <srcdir>  directory containing wayshard[.exe] and wayshard-tui[.exe]
#   <destdir> directory that receives wayshard-<version>-<os>-<arch>.tar.gz|.zip
set -euo pipefail

VERSION="${1:?usage: package-cli-tui.sh <version> <os> <arch> <srcdir> <destdir>}"
OS="${2:?}"
ARCH="${3:?}"
SRC="${4:?}"
DEST="${5:?}"

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
mkdir -p "$DEST"

ext=""
if [[ "$OS" == "windows" ]]; then
  ext=".exe"
fi

cli="$SRC/wayshard${ext}"
tui="$SRC/wayshard-tui${ext}"
notices=("$ROOT/LICENSE" "$ROOT/NOTICE" "$ROOT/THIRD_PARTY_NOTICES.md")

for f in "$cli" "$tui" "${notices[@]}"; do
  if [[ ! -s "$f" ]]; then
    echo "package-cli-tui: missing required file $f" >&2
    exit 1
  fi
done

stage="$(mktemp -d "${TMPDIR:-/tmp}/wayshard-cli-tui.XXXXXX")"
trap 'rm -rf "$stage"' EXIT

cp -a "$cli" "$stage/wayshard${ext}"
cp -a "$tui" "$stage/wayshard-tui${ext}"
cp -a "${notices[@]}" "$stage/"
chmod 0755 "$stage/wayshard${ext}" "$stage/wayshard-tui${ext}"
chmod 0644 "$stage/LICENSE" "$stage/NOTICE" "$stage/THIRD_PARTY_NOTICES.md"

if [[ "$OS" == "windows" ]]; then
  out="$DEST/wayshard-${VERSION}-${OS}-${ARCH}.zip"
  rm -f "$out"
  python3 - "$stage" "$out" <<'PY'
import os
import stat
import sys
import time
import zipfile

stage, out = sys.argv[1], sys.argv[2]
with zipfile.ZipFile(out, "w", zipfile.ZIP_DEFLATED) as z:
    for name in sorted(os.listdir(stage)):
        path = os.path.join(stage, name)
        mode = 0o755 if name.endswith(".exe") else 0o644
        info = zipfile.ZipInfo(name, time.localtime()[:6])
        info.external_attr = (stat.S_IFREG | mode) << 16
        info.compress_type = zipfile.ZIP_DEFLATED
        with open(path, "rb") as fh:
            z.writestr(info, fh.read())
PY
else
  out="$DEST/wayshard-${VERSION}-${OS}-${ARCH}.tar.gz"
  rm -f "$out"
  tar -czf "$out" -C "$stage" \
    "wayshard${ext}" "wayshard-tui${ext}" LICENSE NOTICE THIRD_PARTY_NOTICES.md
fi

echo "packed $(basename "$out")"
