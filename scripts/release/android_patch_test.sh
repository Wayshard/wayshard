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
echo "android-patch-gradle test ok"
