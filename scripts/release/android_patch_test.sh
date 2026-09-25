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
            isMinifyEnabled = false
        }
        getByName("release") {
            // The real Tauri 2.5.0 release build type: R8 minification is ON
            // and every **/*.pro under the app module is collected. The patch
            // must not weaken this, or the JNI helper would not be shrunk the
            // way the release APK actually is.
            isMinifyEnabled = true
            proguardFiles(
                *fileTree(".") { include("**/*.pro") }
                    .plus(getDefaultProguardFile("proguard-android-optimize.txt"))
                    .toList().toTypedArray()
            )
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
# minSdk 26 -> the v1/JAR scheme is never used; apksigner reports it `false` at
# the APK's own minSdk, so the config must not enable or claim it.
grep -q 'enableV1Signing = false' "$TMP/build.gradle.kts"
if grep -q 'enableV1Signing = true' "$TMP/build.gradle.kts"; then
  echo "v1 signing must not be enabled (minSdk 26 never uses it)" >&2
  exit 1
fi
# The release build must stay minified with the .pro glob so the keep rules in
# WayshardKeystore.pro are applied to the real release configuration.
grep -q 'isMinifyEnabled = true' "$TMP/build.gradle.kts"
grep -q 'include("\*\*/\*.pro")' "$TMP/build.gradle.kts"
if grep -q 'java.util.Properties()' "$TMP/build.gradle.kts"; then
  echo "must use imported Properties(), not java.util.Properties()" >&2
  exit 1
fi
if grep -q 'java.io.FileInputStream' "$TMP/build.gradle.kts"; then
  echo "must not use java.io.FileInputStream in Gradle Kotlin DSL" >&2
  exit 1
fi
echo "android-patch-gradle test ok"
