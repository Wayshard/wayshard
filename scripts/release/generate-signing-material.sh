#!/usr/bin/env bash
# Generate maintainer-owned signing material locally. Never run this in CI.
# Does not require Apple, Microsoft, Google, or any paid account.
set -euo pipefail
OUT="${1:?usage: generate-signing-material.sh <secure-signing-directory>}"
mkdir -p "$OUT"
umask 077
cd "$OUT"
echo "writing irreplaceable keys under $OUT"
echo "back this directory up offline (encrypted disk / printed split)."

# Android JKS
if command -v keytool >/dev/null 2>&1; then
  echo "=== Android JKS ==="
  echo "You will be prompted for store/key passwords and DN fields."
  keytool -genkeypair -v \
    -storetype PKCS12 \
    -keystore upload-keystore.jks \
    -keyalg RSA -keysize 2048 -validity 10000 \
    -alias upload
  if base64 -w0 /dev/null >/dev/null 2>&1; then
    base64 -w0 upload-keystore.jks > upload-keystore.jks.b64
  else
    base64 -i upload-keystore.jks > upload-keystore.jks.b64
  fi
  echo "GitHub secret WAYSHARD_ANDROID_KEYSTORE_BASE64 <= upload-keystore.jks.b64"
else
  echo "keytool not found; skip Android JKS (install a JDK)"
fi

# Windows self-signed PFX
if command -v openssl >/dev/null 2>&1; then
  echo "=== Windows self-signed code-signing PFX ==="
  openssl req -x509 -newkey rsa:4096 -sha256 -days 3650 -nodes \
    -subj "/CN=Wayshard/" \
    -addext "extendedKeyUsage=codeSigning" \
    -keyout windows-codesign.key \
    -out windows-codesign.crt
  echo "Enter PFX export password when prompted (this is WAYSHARD_WINDOWS_PFX_PASSWORD)."
  openssl pkcs12 -export -inkey windows-codesign.key -in windows-codesign.crt \
    -name Wayshard \
    -out windows-codesign.pfx
  if base64 -w0 /dev/null >/dev/null 2>&1; then
    base64 -w0 windows-codesign.pfx > windows-codesign.pfx.b64
  else
    base64 -i windows-codesign.pfx > windows-codesign.pfx.b64
  fi
  echo "GitHub secret WAYSHARD_WINDOWS_PFX_BASE64 <= windows-codesign.pfx.b64"
  echo "Keep windows-codesign.key offline; the PFX is what GitHub stores."
else
  echo "openssl not found; skip Windows PFX"
fi

# minisign
if command -v minisign >/dev/null 2>&1; then
  echo "=== minisign release key ==="
  minisign -G -p wayshard-release.minisign.pub -s wayshard-release.minisign.sec
  if base64 -w0 /dev/null >/dev/null 2>&1; then
    base64 -w0 wayshard-release.minisign.sec > wayshard-release.minisign.sec.b64
  else
    base64 -i wayshard-release.minisign.sec > wayshard-release.minisign.sec.b64
  fi
  echo "Commit wayshard-release.minisign.pub as keys/wayshard-release.minisign.pub"
  echo "GitHub secret WAYSHARD_RELEASE_MINISIGN_KEY_BASE64 <= wayshard-release.minisign.sec.b64"
  echo "GitHub secret WAYSHARD_RELEASE_MINISIGN_PASSWORD is the passphrase you just set"
else
  echo "minisign not found; install from https://jedisct1.github.io/minisign/ then re-run"
fi

echo "done. Do not copy private keys into the git repository."
