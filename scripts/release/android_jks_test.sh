#!/usr/bin/env bash
set -euo pipefail
if ! command -v keytool >/dev/null 2>&1; then
  echo "keytool missing; skip android jks generation test"
  exit 0
fi
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
keytool -genkeypair \
  -storetype PKCS12 \
  -keystore "$TMP/upload-keystore.jks" \
  -alias upload \
  -keyalg RSA -keysize 2048 -validity 2 \
  -storepass testpass -keypass testpass \
  -dname "CN=Wayshard Test, O=Wayshard, C=US"
test -s "$TMP/upload-keystore.jks"
python3 - "$TMP/upload-keystore.jks" <<'PY'
import base64, sys
data = open(sys.argv[1], "rb").read()
if len(data) < 64:
    raise SystemExit("keystore too small")
b64 = base64.b64encode(data).decode("ascii")
if len(b64) < 80:
    raise SystemExit("base64 too small")
print("android pkcs12/jks generation ok", len(data), "bytes")
PY
