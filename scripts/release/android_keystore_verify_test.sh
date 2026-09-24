#!/usr/bin/env bash
# Prove the Android Keystore JNI helper survives the real minified release
# toolchain (R8 in --release mode) and that the verify gate catches it when the
# keep rule is absent.
#
# The synthetic-DEX self-test always runs. The real-R8 half runs when a JDK,
# R8 (build-tools d8.jar) and android.jar are available; set
# WAYSHARD_REQUIRE_R8=1 to fail instead of skipping when they are not.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
VERIFY="$ROOT/scripts/release/android-keystore-verify.py"
KEEP_SRC="$ROOT/clients/desktop/android/WayshardKeystore.pro"
PKG="dev.wayshard.app"

python3 "$VERIFY" --self-test

sdk="${ANDROID_HOME:-${ANDROID_SDK_ROOT:-$HOME/Android/Sdk}}"

find_first() { find "$1" -maxdepth "$2" -name "$3" 2>/dev/null | sort -V | tail -n1; }

D8_JAR=""
if [[ -d "$sdk/build-tools" ]]; then
  D8_JAR="$(find_first "$sdk/build-tools" 3 'd8.jar')"
fi
ANDROID_JAR=""
if [[ -d "$sdk/platforms" ]]; then
  ANDROID_JAR="$(find_first "$sdk/platforms" 2 'android.jar')"
fi

if [[ -z "$D8_JAR" || -z "$ANDROID_JAR" || ! -f "$D8_JAR" || ! -f "$ANDROID_JAR" ]] || ! command -v javac >/dev/null 2>&1; then
  if [[ "${WAYSHARD_REQUIRE_R8:-0}" == "1" ]]; then
    echo "real R8 toolchain unavailable (d8.jar=$D8_JAR android.jar=$ANDROID_JAR javac=$(command -v javac || echo none))" >&2
    exit 1
  fi
  echo "real R8 toolchain unavailable; ran synthetic-DEX self-test only"
  exit 0
fi

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

mkdir -p "$TMP/src/dev/wayshard/app" "$TMP/classes"
cat > "$TMP/src/dev/wayshard/app/WayshardKeystore.java" <<'EOF'
package dev.wayshard.app;
public final class WayshardKeystore {
  public static String encrypt(String value) { return value; }
  public static String decrypt(String token) { return token; }
  public static String deleteKey(String ignored) { return "ok"; }
}
EOF
javac -source 8 -target 8 -d "$TMP/classes" "$TMP/src/dev/wayshard/app/WayshardKeystore.java" >/dev/null 2>&1
CLASSES=("$TMP/classes/dev/wayshard/app/WayshardKeystore.class")

# The keep rule installed by android-keystore-patch.sh.
sed "s/__PACKAGE__/${PKG}/g" "$KEEP_SRC" > "$TMP/keep.pro"
grep -q "^-keep class ${PKG}.WayshardKeystore { \*; }$" "$TMP/keep.pro"

run_r8() { # <outdir> [extra args...]
  local out="$1"; shift
  rm -rf "$out"; mkdir -p "$out"
  java -cp "$D8_JAR" com.android.tools.r8.R8 --release --min-api 26 \
    --lib "$ANDROID_JAR" --output "$out" "$@" "${CLASSES[@]}" >/dev/null 2>&1 || true
}

# Without the keep rule R8 tree-shakes the unreferenced helper away, and the
# gate must fail closed.
run_r8 "$TMP/stripped"
if [[ -f "$TMP/stripped/classes.dex" ]] && python3 "$VERIFY" "$TMP/stripped/classes.dex" >/dev/null 2>&1; then
  echo "R8 stripped the helper without keep rules but the gate accepted it" >&2
  exit 1
fi

# With the keep rule the helper and all JNI method names survive.
run_r8 "$TMP/kept" --pg-conf "$TMP/keep.pro"
python3 "$VERIFY" "$TMP/kept/classes.dex" >/dev/null
echo "real R8 keep-rule test ok (stripped without rules, preserved with rules)"
