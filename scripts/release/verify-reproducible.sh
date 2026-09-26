#!/usr/bin/env bash
# Prove published release artifacts are reproducible and internally consistent:
#
#   1. For every platform, the standalone `wayshard` CLI is byte-identical to the
#      `wayshard` inside that platform's CLI+TUI archive (the archive is what a
#      user downloads, so the raw advanced-user binary must not diverge).
#   2. The host CLI rebuilt from the tagged source with the canonical flags and
#      commit-derived date is byte-identical to the published standalone CLI.
#
# Usage: verify-reproducible.sh <tag> <commit> <assets-dir>
set -euo pipefail

TAG="${1:?usage: verify-reproducible.sh <tag> <commit> <assets-dir>}"
COMMIT="${2:?usage: verify-reproducible.sh <tag> <commit> <assets-dir>}"
ASSETS="$(cd "${3:?usage: verify-reproducible.sh <tag> <commit> <assets-dir>}" && pwd)"
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
WORK="$(mktemp -d "${RUNNER_TEMP:-/tmp}/wayshard-repro.XXXXXX")"
trap 'rm -rf "$WORK"' EXIT

sha() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  else
    shasum -a 256 "$1" | awk '{print $1}'
  fi
}

fail=0

# 1) standalone CLI == bundled CLI, per platform.
shopt -s nullglob
archives=("$ASSETS"/wayshard-"$TAG"-*.tar.gz "$ASSETS"/wayshard-"$TAG"-*.zip)
shopt -u nullglob

if [[ ${#archives[@]} -eq 0 ]]; then
  echo "no CLI+TUI archives found for $TAG in $ASSETS" >&2
  exit 1
fi

for archive in "${archives[@]}"; do
  base="$(basename "$archive")"
  plat="${base#wayshard-"$TAG"-}"
  plat="${plat%.tar.gz}"
  plat="${plat%.zip}"
  ext=""
  case "$plat" in windows-*) ext=".exe" ;; esac
  raw="$ASSETS/wayshard-$TAG-$plat$ext"
  dest="$WORK/$plat"
  mkdir -p "$dest"
  case "$archive" in
    *.zip) unzip -q "$archive" -d "$dest" ;;
    *.tar.gz) tar -xzf "$archive" -C "$dest" ;;
  esac
  inner="$dest/wayshard$ext"
  if [[ ! -f "$raw" ]]; then
    echo "MISSING standalone CLI: $raw" >&2
    fail=1
    continue
  fi
  if [[ ! -f "$inner" ]]; then
    echo "MISSING bundled CLI in $(basename "$archive")" >&2
    fail=1
    continue
  fi
  a="$(sha "$raw")"
  b="$(sha "$inner")"
  if [[ "$a" == "$b" ]]; then
    echo "ok: $plat standalone == bundled ($a)"
  else
    echo "MISMATCH $plat standalone=$a bundled=$b" >&2
    fail=1
  fi
done

# 2) fresh rebuild of the host CLI == published standalone host CLI.
hostos="$(go env GOOS)"
hostarch="$(go env GOARCH)"
hostext=""
[[ "$hostos" == "windows" ]] && hostext=".exe"
hostraw="$ASSETS/wayshard-$TAG-$hostos-$hostarch$hostext"
if [[ -f "$hostraw" ]]; then
  (
    cd "$ROOT"
    CGO_ENABLED=0 GOOS="$hostos" GOARCH="$hostarch" \
      make --no-print-directory build-cli VERSION="$TAG" COMMIT="$COMMIT" BINDIR="$WORK/bin" >/dev/null
  )
  rebuilt="$WORK/bin/wayshard$hostext"
  a="$(sha "$hostraw")"
  b="$(sha "$rebuilt")"
  if [[ "$a" == "$b" ]]; then
    echo "ok: rebuilt $hostos-$hostarch == published ($a)"
  else
    echo "MISMATCH rebuilt $hostos-$hostarch=$b published=$a" >&2
    fail=1
  fi
else
  echo "note: no published CLI for host $hostos-$hostarch; skipped rebuild comparison"
fi

if [[ "$fail" -ne 0 ]]; then
  echo "reproducibility verification FAILED" >&2
  exit 1
fi
echo "reproducibility verification ok"
