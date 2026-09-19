#!/usr/bin/env bash
# Fail closed: PR CI stays secret-free; release.yml uses only maintainer-owned
# signing secrets; no paid/platform signing accounts.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
CI="$ROOT/.github/workflows/ci.yml"
REL="$ROOT/.github/workflows/release.yml"
TAURI="$ROOT/clients/desktop/src-tauri/tauri.conf.json"

fail() { echo "release_policy_test: $*" >&2; exit 1; }

if grep -E 'secrets\.|environment:' "$CI"; then
  fail "ci.yml must not reference secrets or environments"
fi
if grep -E 'TYPESAFE_API_KEY|WAYSHARD_VAULT' "$CI"; then
  fail "ci.yml must not reference runtime vault/Jev secrets"
fi

grep -q 'environment: release' "$REL" || fail "release.yml must use environment: release"
grep -q "github.repository == 'Wayshard/wayshard'" "$REL" || fail "official publish must be gated to Wayshard/wayshard"

required=(
  WAYSHARD_ANDROID_KEYSTORE_BASE64
  WAYSHARD_ANDROID_KEYSTORE_PASSWORD
  WAYSHARD_ANDROID_KEY_ALIAS
  WAYSHARD_ANDROID_KEY_PASSWORD
  WAYSHARD_WINDOWS_PFX_BASE64
  WAYSHARD_WINDOWS_PFX_PASSWORD
  WAYSHARD_RELEASE_MINISIGN_KEY_BASE64
  WAYSHARD_RELEASE_MINISIGN_PASSWORD
)
for n in "${required[@]}"; do
  grep -q "$n" "$REL" || fail "release.yml missing $n"
done

forbidden='APPLE_ID|APP_STORE_CONNECT|APPLE_API_KEY|NOTARIZE|NOTARYTOOL|DEVELOPER_ID_APPLICATION|AZURE_|SM_CLIENT_CERT|DIGICERT|GOOGLE_PLAY|PLAY_STORE|PLAY_CONSOLE|TAURI_SIGNING_PRIVATE_KEY|CSC_LINK|CSC_KEY_PASSWORD|WINDOWS_CERTIFICATE_THUMBPRINT'
if grep -E "$forbidden" "$REL" "$TAURI"; then
  fail "paid/platform signing identifiers must not appear as required workflow config"
fi

grep -q 'APPLE_SIGNING_IDENTITY: "-"' "$REL" || fail "macOS must ad-hoc sign with identity -"
python3 - "$TAURI" <<'PY'
import json, sys
cfg = json.load(open(sys.argv[1], encoding="utf-8"))
ident = cfg.get("bundle", {}).get("macOS", {}).get("signingIdentity")
if ident != "-":
    raise SystemExit(f"tauri macOS signingIdentity must be '-', got {ident!r}")
plugins = cfg.get("plugins") or {}
if "updater" in plugins:
    raise SystemExit("tauri updater plugin must not be configured")
print("tauri ad-hoc signing, no updater")
PY

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
if bash "$ROOT/scripts/release/android-sign.sh" >"$TMP/android.err" 2>&1; then
  fail "android-sign.sh must fail when secrets are missing"
fi
grep -q 'WAYSHARD_ANDROID_KEYSTORE_BASE64' "$TMP/android.err" || fail "android-sign.sh must name the missing secret"

echo "policy" > "$TMP/SHA256SUMS.txt"
if bash "$ROOT/scripts/release/minisign-sign.sh" "$TMP/SHA256SUMS.txt" >"$TMP/minisign.err" 2>&1; then
  fail "minisign-sign.sh must fail when secrets are missing"
fi
grep -q 'WAYSHARD_RELEASE_MINISIGN_KEY_BASE64' "$TMP/minisign.err" || fail "minisign-sign.sh must name the missing secret"

echo "release policy test ok"
