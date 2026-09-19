#!/usr/bin/env bash
# Remove materialized Android signing files from the runner.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
rm -f "$ROOT/clients/desktop/src-tauri/gen/android/keystore.properties"
if [[ -n "${WAYSHARD_ANDROID_KEYSTORE_FILE:-}" ]]; then
  rm -f "$WAYSHARD_ANDROID_KEYSTORE_FILE"
fi
find "${RUNNER_TEMP:-/tmp}" -maxdepth 2 -name 'wayshard-upload-keystore.jks' -delete 2>/dev/null || true
echo "android signing material removed"
