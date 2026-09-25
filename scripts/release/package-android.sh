#!/usr/bin/env bash
# Copy the signed release APK to an unambiguous Wayshard name.
set -euo pipefail
VERSION="${1:?usage: package-android.sh <version> <gen-android-dir> <destdir>}"
GEN="${2:?}"
DEST="${3:?}"
mkdir -p "$DEST"

mapfile -t apks < <(find "$GEN" -type f -name '*.apk' ! -name '*-unsigned.apk' ! -name '*.apk.unsigned' | sort)
if [[ ${#apks[@]} -eq 0 ]]; then
  echo "no APK found under $GEN" >&2
  exit 1
fi

# Prefer a universal or arm64-v8a release APK.
pick=""
for f in "${apks[@]}"; do
  case "$f" in
    *unsigned*) continue ;;
    *universal*) pick="$f"; break ;;
  esac
done
if [[ -z "$pick" ]]; then
  for f in "${apks[@]}"; do
    case "$f" in
      *arm64*) pick="$f"; break ;;
    esac
  done
fi
if [[ -z "$pick" ]]; then
  pick="${apks[0]}"
fi

out="$DEST/wayshard-${VERSION}-android.apk"
cp -a "$pick" "$out"
echo "packed $out from $pick"

# Verify the published APK actually carries the required APK Signature Scheme
# v2 + v3 signatures. v1/JAR is intentionally not enabled (minSdk 26 never uses
# it and apksigner reports it false at that minSdk).
AS="$(command -v apksigner 2>/dev/null || true)"
if [[ -z "$AS" && -n "${ANDROID_SDK_ROOT:-}" ]]; then
  AS="$(ls -d "$ANDROID_SDK_ROOT"/build-tools/*/apksigner 2>/dev/null | sort -V | tail -n1 || true)"
fi

if [[ -n "$AS" && -x "$AS" ]]; then
  verify_out="$("$AS" verify --verbose "$out" 2>&1)"
  echo "$verify_out"
  grep -qiE 'v2 scheme \(APK Signature Scheme v2\): true' <<<"$verify_out" \
    || { echo "APK is not signed with APK Signature Scheme v2" >&2; exit 1; }
  grep -qiE 'v3 scheme \(APK Signature Scheme v3\): true' <<<"$verify_out" \
    || { echo "APK is not signed with APK Signature Scheme v3" >&2; exit 1; }
  echo "verified APK Signature Scheme v2 + v3"
else
  echo "apksigner not available; falling back to a META-INF signature-block check" >&2
  python3 - "$out" <<'PY'
import sys, zipfile
z = zipfile.ZipFile(sys.argv[1])
names = z.namelist()
if not any(n.startswith("META-INF/") and (n.endswith(".RSA") or n.endswith(".DSA") or n.endswith(".EC") or n.endswith(".SF")) for n in names):
    raise SystemExit("APK has no META-INF signing block; refusing to publish as signed")
print("found META-INF signature files")
PY
fi
