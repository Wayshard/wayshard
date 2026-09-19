#!/usr/bin/env bash
# Decode the Android upload keystore into $RUNNER_TEMP (or mktemp) and write
# gen/android/keystore.properties for Tauri 2 Gradle. Does not leave the
# keystore in the workspace. Caller must delete the temp file in an always() step.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
GEN="$ROOT/clients/desktop/src-tauri/gen/android"
GRADLE="$GEN/app/build.gradle.kts"

need() {
  local n="$1"
  if [[ -z "${!n:-}" ]]; then
    echo "missing required GitHub Environment secret $n" >&2
    echo "Official Android APKs are signed; unsigned APKs are not published." >&2
    exit 1
  fi
}

need WAYSHARD_ANDROID_KEYSTORE_BASE64
need WAYSHARD_ANDROID_KEYSTORE_PASSWORD
need WAYSHARD_ANDROID_KEY_ALIAS
need WAYSHARD_ANDROID_KEY_PASSWORD

if [[ ! -f "$GRADLE" ]]; then
  echo "missing $GRADLE; run: bunx tauri android init --ci" >&2
  exit 1
fi

TMPDIR_SIGN="${RUNNER_TEMP:-$(mktemp -d /tmp/wayshard-android-sign.XXXXXX)}"
mkdir -p "$TMPDIR_SIGN"
KEYSTORE="$TMPDIR_SIGN/wayshard-upload-keystore.jks"
umask 077

python3 - "$KEYSTORE" <<'PY'
import base64, os, sys
raw = os.environ["WAYSHARD_ANDROID_KEYSTORE_BASE64"]
data = base64.b64decode(raw)
if len(data) < 32:
    raise SystemExit("decoded keystore is too small")
open(sys.argv[1], "wb").write(data)
print(f"wrote {len(data)} byte keystore")
PY

umask 077
cat > "$GEN/keystore.properties" <<EOF
storeFile=$KEYSTORE
storePassword=$WAYSHARD_ANDROID_KEYSTORE_PASSWORD
keyPassword=$WAYSHARD_ANDROID_KEY_PASSWORD
keyAlias=$WAYSHARD_ANDROID_KEY_ALIAS
password=$WAYSHARD_ANDROID_KEYSTORE_PASSWORD
EOF

python3 "$ROOT/scripts/release/android-patch-gradle.py" "$GRADLE"

if [[ -n "${GITHUB_ENV:-}" ]]; then
  echo "WAYSHARD_ANDROID_KEYSTORE_FILE=$KEYSTORE" >> "$GITHUB_ENV"
fi
echo "WAYSHARD_ANDROID_KEYSTORE_FILE=$KEYSTORE"
