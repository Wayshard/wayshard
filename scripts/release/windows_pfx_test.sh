#!/usr/bin/env bash
set -euo pipefail
if ! command -v openssl >/dev/null 2>&1; then
  echo "openssl missing" >&2
  exit 1
fi
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
openssl req -x509 -newkey rsa:2048 -sha256 -days 2 -nodes \
  -subj "/CN=Wayshard/" \
  -addext "extendedKeyUsage=codeSigning" \
  -keyout "$TMP/k.pem" -out "$TMP/c.crt" >/dev/null 2>&1
openssl pkcs12 -export -inkey "$TMP/k.pem" -in "$TMP/c.crt" \
  -name Wayshard -passout pass:testpass -out "$TMP/c.pfx"
python3 - "$TMP/c.pfx" <<'PY'
import sys
p = sys.argv[1]
data = open(p, "rb").read()
if len(data) < 64:
    raise SystemExit("pfx too small")
# PKCS12 magic-ish: starts with 0x30 (SEQUENCE)
if data[0] != 0x30:
    raise SystemExit("pfx does not look like DER")
print("windows pfx generation ok", len(data), "bytes")
PY
