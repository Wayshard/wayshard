#!/usr/bin/env bash
# The published APK must carry the versionCode/versionName from tauri.conf.json,
# not the Tauri template defaults. Fail closed when the config is incomplete.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

cat > "$TMP/tauri.conf.json" <<'EOF'
{
  "version": "0.1.0-5",
  "bundle": { "android": { "minSdkVersion": 26, "versionCode": 10005 } }
}
EOF

bash "$ROOT/scripts/release/android-version-properties.sh" \
  "$TMP/tauri.conf.json" "$TMP/app/tauri.properties"

grep -qx 'tauri.android.versionCode=10005' "$TMP/app/tauri.properties"
grep -qx 'tauri.android.versionName=0.1.0-5' "$TMP/app/tauri.properties"
if grep -qE '^versionCode=|^versionName=' "$TMP/app/tauri.properties"; then
  echo "properties must use the tauri.android.* keys the Gradle template reads" >&2
  exit 1
fi
echo "android version properties ok"

# Missing versionCode must fail closed instead of silently shipping 1/1.0.
cat > "$TMP/bad.json" <<'EOF'
{ "version": "0.1.0-5", "bundle": { "android": { "minSdkVersion": 26 } } }
EOF
if bash "$ROOT/scripts/release/android-version-properties.sh" \
  "$TMP/bad.json" "$TMP/bad/tauri.properties" >/dev/null 2>&1; then
  echo "expected failure when bundle.android.versionCode is missing" >&2
  exit 1
fi
echo "android version properties fail-closed ok"
