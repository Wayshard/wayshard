#!/usr/bin/env bash
# Self-test for desktop-macos-checksums.sh: both macOS DMGs appear exactly once,
# independent of the order the matrix runners uploaded them, and any missing or
# duplicated architecture fails closed.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
SCRIPT="$ROOT/scripts/release/desktop-macos-checksums.sh"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
VERSION="v9.9.9-rc.1"
A="wayshard-desktop-${VERSION}-macos-aarch64.dmg"
B="wayshard-desktop-${VERSION}-macos-x86_64.dmg"

mkdir -p "$TMP/one" "$TMP/two"
printf 'arm-dmg' > "$TMP/one/$A"
printf 'intel-dmg' > "$TMP/one/$B"
# Unrelated release assets must be ignored.
printf 'linux' > "$TMP/one/wayshard-desktop-${VERSION}-linux-amd64.AppImage"
printf 'other-version' > "$TMP/one/wayshard-desktop-v1.0.0-macos-aarch64.dmg"
# The same pair created in the opposite order must produce identical output.
printf 'intel-dmg' > "$TMP/two/$B"
printf 'arm-dmg' > "$TMP/two/$A"

bash "$SCRIPT" "$VERSION" "$TMP/one" "$TMP/SHA256SUMS-desktop-macos.txt"
bash "$SCRIPT" "$VERSION" "$TMP/two" "$TMP/other.txt"
test "$(grep -c . "$TMP/SHA256SUMS-desktop-macos.txt")" = "2"
test "$(grep -c "  $A$" "$TMP/SHA256SUMS-desktop-macos.txt")" = "1"
test "$(grep -c "  $B$" "$TMP/SHA256SUMS-desktop-macos.txt")" = "1"
diff -u "$TMP/SHA256SUMS-desktop-macos.txt" "$TMP/other.txt" >/dev/null \
  || { echo "output depends on upload order" >&2; exit 1; }

expect_fail() {
  local dir="$1"
  if bash "$SCRIPT" "$VERSION" "$dir" "$TMP/should-not-exist.txt" >/dev/null 2>&1; then
    echo "expected failure for $dir" >&2
    exit 1
  fi
}

# Only one architecture.
mkdir -p "$TMP/missing"
printf 'arm' > "$TMP/missing/$A"
expect_fail "$TMP/missing"

# A duplicated aarch64 alongside x86_64.
mkdir -p "$TMP/dup"
printf 'arm' > "$TMP/dup/$A"
printf 'intel' > "$TMP/dup/$B"
printf 'arm2' > "$TMP/dup/wayshard-desktop-${VERSION}-macos-aarch64-extra.dmg"
expect_fail "$TMP/dup"

# An unexpected architecture.
mkdir -p "$TMP/wrong"
printf 'arm' > "$TMP/wrong/$A"
printf 'ppc' > "$TMP/wrong/wayshard-desktop-${VERSION}-macos-ppc64.dmg"
expect_fail "$TMP/wrong"

# No macOS DMGs at all.
mkdir -p "$TMP/none"
expect_fail "$TMP/none"

echo "desktop-macos-checksums test ok"
