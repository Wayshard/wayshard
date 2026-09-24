#!/usr/bin/env bash
# Self-test for android-keystore-patch.sh: the helper is installed beside the
# generated MainActivity with a matching package, Keystore-backed AES-GCM, and
# the R8/ProGuard keep rules that stop the minified release build stripping it.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

GEN="$TMP/gen/android"
mkdir -p "$GEN/app/src/main/java/dev/wayshard/app"
cat > "$GEN/app/src/main/java/dev/wayshard/app/MainActivity.kt" <<'EOF'
package dev.wayshard.app
class MainActivity
EOF

bash "$ROOT/scripts/release/android-keystore-patch.sh" "$GEN"

DEST="$GEN/app/src/main/java/dev/wayshard/app/WayshardKeystore.kt"
[ -f "$DEST" ] || { echo "helper was not installed" >&2; exit 1; }

check() { grep -q "$1" "$DEST" || { echo "helper missing $1" >&2; exit 1; }; }
check '^package dev.wayshard.app'
check 'AndroidKeyStore'
check 'AES/GCM/NoPadding'
check 'setKeySize(256)'
check 'setRandomizedEncryptionRequired(true)'
check 'GCMParameterSpec'

# The R8/ProGuard keep rules must be installed beside the helper (the release
# build type collects **/*.pro), keep the exact JNI class name and retain all
# members, and must not leave the package placeholder unreplaced.
PRO="$GEN/app/src/main/java/dev/wayshard/app/WayshardKeystore.pro"
[ -f "$PRO" ] || { echo "keep rules were not installed" >&2; exit 1; }
grep -qx -- '-keep class dev.wayshard.app.WayshardKeystore { \*; }' "$PRO" \
  || { echo "keep rules do not retain dev.wayshard.app.WayshardKeystore" >&2; exit 1; }
if grep -q '__PACKAGE__' "$PRO"; then
  echo "keep-rule package placeholder was not replaced" >&2
  exit 1
fi

# The keep rule must also follow a non-default generated package.
ALT="$TMP/alt/gen/android"
mkdir -p "$ALT/app/src/main/java/com/example/app"
printf 'package com.example.app\nclass MainActivity\n' > "$ALT/app/src/main/java/com/example/app/MainActivity.kt"
bash "$ROOT/scripts/release/android-keystore-patch.sh" "$ALT" >/dev/null
grep -qx -- '-keep class com.example.app.WayshardKeystore { \*; }' \
  "$ALT/app/src/main/java/com/example/app/WayshardKeystore.pro" \
  || { echo "keep rules did not follow the derived package" >&2; exit 1; }

# A missing generated project must fail closed rather than silently do nothing.
if bash "$ROOT/scripts/release/android-keystore-patch.sh" "$TMP/does-not-exist" >/dev/null 2>&1; then
  echo "patch must fail when the generated Android project is missing" >&2
  exit 1
fi

echo "android-keystore-patch test ok"
