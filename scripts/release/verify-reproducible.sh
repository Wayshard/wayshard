#!/usr/bin/env bash
# Reproducible-release verification.
#
# Two modes share one comparison path so normal CI runs exactly what tagged
# releases run:
#
#   verify-reproducible.sh --assets <version> <commit> <assets-dir>
#       Compare an already-published release asset set (used by the release
#       workflow's `reproducible` job after `gh release download`).
#
#   verify-reproducible.sh --build <version> <commit> <workdir>
#       Build the same artifact layout locally from source with the canonical
#       Makefile targets and packaging scripts, then run the identical
#       comparison. Used by normal CI, requires no tag and publishes nothing.
#
# The comparison asserts:
#   1. For every platform, the standalone `wayshard` CLI is byte-identical to the
#      `wayshard` inside that platform's CLI+TUI archive.
#   2. The host CLI rebuilt from the commit with the canonical flags is
#      byte-identical to the published standalone host CLI.
set -euo pipefail

MODE="${1:?usage: verify-reproducible.sh --assets|--build <version> <commit> <dir>}"
VERSION="${2:?usage: verify-reproducible.sh --assets|--build <version> <commit> <dir>}"
COMMIT="${3:?usage: verify-reproducible.sh --assets|--build <version> <commit> <dir>}"
TARGET="${4:?usage: verify-reproducible.sh --assets|--build <version> <commit> <dir>}"
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
WORK="$(mktemp -d "${TMPDIR:-/tmp}/wayshard-repro.XXXXXX")"
trap 'rm -rf "$WORK"' EXIT

PLATFORMS="linux-amd64 linux-arm64 darwin-amd64 darwin-arm64 windows-amd64"

sha() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  else
    shasum -a 256 "$1" | awk '{print $1}'
  fi
}

ext() { [ "$1" = "windows-amd64" ] && echo ".exe" || echo ""; }

# verify_asset_dir <assets-dir>: the single shared comparison path.
verify_asset_dir() {
  local assets="$1" fail=0 archive base plat e raw inner dest a b
  assets="$(cd "$assets" && pwd)"

  shopt -s nullglob
  archives=("$assets"/wayshard-"$VERSION"-*.tar.gz "$assets"/wayshard-"$VERSION"-*.zip)
  shopt -u nullglob
  if [[ ${#archives[@]} -eq 0 ]]; then
    echo "no CLI+TUI archives found for $VERSION in $assets" >&2
    return 1
  fi

  # 1) standalone CLI == bundled CLI, per platform.
  for archive in "${archives[@]}"; do
    base="$(basename "$archive")"
    plat="${base#wayshard-"$VERSION"-}"
    plat="${plat%.tar.gz}"
    plat="${plat%.zip}"
    e="$(ext "$plat")"
    raw="$assets/wayshard-$VERSION-$plat$e"
    dest="$WORK/verify-$plat"
    mkdir -p "$dest"
    case "$archive" in
      *.zip) unzip -q "$archive" -d "$dest" ;;
      *.tar.gz) tar -xzf "$archive" -C "$dest" ;;
    esac
    inner="$dest/wayshard$e"
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
  local hostos hostarch hostext hostraw rebuilt
  hostos="$(go env GOOS)"
  hostarch="$(go env GOARCH)"
  hostext=""
  [[ "$hostos" == "windows" ]] && hostext=".exe"
  hostraw="$assets/wayshard-$VERSION-$hostos-$hostarch$hostext"
  if [[ -f "$hostraw" ]]; then
    (
      cd "$ROOT"
      CGO_ENABLED=0 GOOS="$hostos" GOARCH="$hostarch" \
        make --no-print-directory build-cli VERSION="$VERSION" COMMIT="$COMMIT" BINDIR="$WORK/bin" >/dev/null
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

  [[ "$fail" -eq 0 ]]
}

# build_asset_dir <builddir>: build the release layout with the same Makefile
# targets and packaging scripts the release workflow uses.
build_asset_dir() {
  local builddir="$1" plat os arch e stage
  mkdir -p "$builddir/bin" "$builddir/assets"

  echo "building canonical server+CLI for all platforms ..."
  (
    cd "$ROOT"
    make --no-print-directory build-cross VERSION="$VERSION" COMMIT="$COMMIT" BINDIR="$builddir/bin" >/dev/null
  )
  bash "$ROOT/scripts/release/package-go.sh" "$VERSION" "$builddir/bin" "$builddir/assets" >/dev/null

  for plat in $PLATFORMS; do
    os="${plat%-*}"
    arch="${plat#*-}"
    e="$(ext "$plat")"
    stage="$builddir/stage-$plat"
    mkdir -p "$stage"
    cp -a "$builddir/assets/wayshard-$VERSION-$plat$e" "$stage/wayshard$e"
    chmod 0755 "$stage/wayshard$e"
    printf '#!/bin/sh\necho "wayshard-tui stub"\n' >"$stage/wayshard-tui$e"
    chmod 0755 "$stage/wayshard-tui$e"
    cp "$ROOT/LICENSE" "$ROOT/NOTICE" "$ROOT/THIRD_PARTY_NOTICES.md" "$stage/"
    bash "$ROOT/scripts/release/package-cli-tui.sh" "$VERSION" "$os" "$arch" "$stage" "$builddir/assets" >/dev/null
  done
}

case "$MODE" in
  --assets)
    echo "verifying published asset set for $VERSION in $TARGET"
    verify_asset_dir "$TARGET"
    ;;
  --build)
    echo "building a local $VERSION artifact set in $TARGET and verifying it"
    build_asset_dir "$TARGET"
    verify_asset_dir "$TARGET/assets"
    ;;
  *)
    echo "unknown mode: $MODE (expected --assets or --build)" >&2
    exit 2
    ;;
esac

echo "reproducibility verification ok"
