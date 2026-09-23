#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
cat > "$TMP/build.gradle.kts" <<'EOF'
plugins {
    id("com.android.application")
}
android {
    namespace = "dev.wayshard.app"
    buildTypes {
        getByName("debug") {
            isDebuggable = true
        }
        getByName("release") {
            isMinifyEnabled = false
        }
    }
}
EOF
python3 "$ROOT/scripts/release/android-patch-gradle.py" "$TMP/build.gradle.kts"
grep -q 'keystore.properties' "$TMP/build.gradle.kts"
grep -q 'signingConfig = signingConfigs.getByName("release")' "$TMP/build.gradle.kts"
grep -q 'import java.util.Properties' "$TMP/build.gradle.kts"
grep -q 'enableV3Signing = true' "$TMP/build.gradle.kts"
grep -q 'enableV2Signing = true' "$TMP/build.gradle.kts"
if grep -q 'java.util.Properties()' "$TMP/build.gradle.kts"; then
  echo "must use imported Properties(), not java.util.Properties()" >&2
  exit 1
fi
if grep -q 'java.io.FileInputStream' "$TMP/build.gradle.kts"; then
  echo "must not use java.io.FileInputStream in Gradle Kotlin DSL" >&2
  exit 1
fi
echo "android-patch-gradle test ok"
