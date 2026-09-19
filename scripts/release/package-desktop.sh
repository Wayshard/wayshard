#!/usr/bin/env bash
# Rename Tauri 2 bundles to unambiguous Wayshard release names.
set -euo pipefail
VERSION="${1:?usage: package-desktop.sh <version> <os> <bundle-root> <destdir>}"
OS="${2:?}"
BUNDLE="${3:?}"
DEST="${4:?}"
mkdir -p "$DEST"

arch_linux() {
  case "$(uname -m)" in
    x86_64|amd64) echo "amd64" ;;
    aarch64|arm64) echo "arm64" ;;
    *) uname -m ;;
  esac
}

copy_one() {
  local pattern="$1" destname="$2"
  local matches=()
  while IFS= read -r -d '' f; do
    matches+=("$f")
  done < <(find "$BUNDLE" -type f -name "$pattern" -print0 2>/dev/null || true)
  if [[ ${#matches[@]} -eq 0 ]]; then
    echo "no match for $pattern under $BUNDLE" >&2
    return 1
  fi
  if [[ ${#matches[@]} -gt 1 ]]; then
    echo "multiple matches for $pattern:" "${matches[@]}" >&2
    exit 1
  fi
  cp -a "${matches[0]}" "$DEST/$destname"
  echo "packed $destname"
}

case "$OS" in
  linux)
    ARCH="$(arch_linux)"
    copy_one "*.AppImage" "wayshard-desktop-${VERSION}-linux-${ARCH}.AppImage" || true
    copy_one "*.deb" "wayshard-desktop-${VERSION}-linux-${ARCH}.deb" || true
    if ! ls "$DEST"/wayshard-desktop-"${VERSION}"-linux-* >/dev/null 2>&1; then
      echo "no linux desktop bundles found" >&2
      exit 1
    fi
    ;;
  macos)
    ARCH="$(arch_linux)"
    if [[ "$ARCH" == "amd64" ]]; then ARCH="x86_64"; fi
    if [[ "$ARCH" == "arm64" ]]; then ARCH="aarch64"; fi
    copy_one "*.dmg" "wayshard-desktop-${VERSION}-macos-${ARCH}.dmg" || true
    if ! ls "$DEST"/wayshard-desktop-"${VERSION}"-macos-* >/dev/null 2>&1; then
      echo "no macos desktop bundles found" >&2
      exit 1
    fi
    ;;
  windows)
    copy_one "*.msi" "wayshard-desktop-${VERSION}-windows-x64.msi" || true
    copy_one "*.exe" "wayshard-desktop-${VERSION}-windows-x64-setup.exe" || true
    if ! ls "$DEST"/wayshard-desktop-"${VERSION}"-windows-* >/dev/null 2>&1; then
      echo "no windows desktop bundles found" >&2
      exit 1
    fi
    ;;
  *)
    echo "unknown os $OS" >&2
    exit 1
    ;;
esac
