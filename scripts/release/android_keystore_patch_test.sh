#!/usr/bin/env bash
# Self-test for android-keystore-patch.sh: the helper is installed beside the
# generated MainActivity with a matching package and Keystore-backed AES-GCM.
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

# A missing generated project must fail closed rather than silently do nothing.
if bash "$ROOT/scripts/release/android-keystore-patch.sh" "$TMP/does-not-exist" >/dev/null 2>&1; then
  echo "patch must fail when the generated Android project is missing" >&2
  exit 1
fi

echo "android-keystore-patch test ok"
