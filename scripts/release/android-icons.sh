#!/usr/bin/env bash
# Generate the Android launcher and adaptive-icon resources from the canonical
# Wayshard mark.
#
# `tauri android init` seeds the ephemeral `gen/android` project with Tauri's
# placeholder launcher icons. `clients/desktop/src-tauri/app-icon.json` is a
# Tauri icon *manifest* that regenerates the launcher, adaptive foreground,
# monochrome themed icon and background resources from the canonical branding
# art in `assets/branding`.
#
# Tauri's icon manifest landed in tauri-cli 2.9.0, while this project pins
# tauri-cli 2.5.0 for its mobile build template. Only this icon step runs the
# pinned, manifest-capable generator; the build template and toolchain are
# unchanged. Bump `WAYSHARD_ICON_CLI_VERSION` deliberately, together with
# app-icon.json, when the pinned build CLI is upgraded.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
ICON_CLI_VERSION="${WAYSHARD_ICON_CLI_VERSION:-2.11.5}"
DESKTOP="$ROOT/clients/desktop"
MANIFEST="$DESKTOP/src-tauri/app-icon.json"

[ -f "$MANIFEST" ] || { echo "missing icon manifest $MANIFEST" >&2; exit 1; }
[ -d "$DESKTOP/src-tauri/gen/android/app/src/main/res" ] \
  || { echo "missing generated Android project; run: bunx tauri android init --ci" >&2; exit 1; }

cd "$DESKTOP"
bunx "@tauri-apps/cli@${ICON_CLI_VERSION}" icon src-tauri/app-icon.json

# The generator must have produced the adaptive icon resources, not just legacy
# launcher PNGs.
RES="$DESKTOP/src-tauri/gen/android/app/src/main/res"
[ -f "$RES/mipmap-anydpi-v26/ic_launcher.xml" ] \
  || { echo "adaptive ic_launcher.xml was not generated" >&2; exit 1; }
[ -f "$RES/values/ic_launcher_background.xml" ] \
  || { echo "adaptive background color was not generated" >&2; exit 1; }
for d in mdpi hdpi xhdpi xxhdpi xxxhdpi; do
  [ -f "$RES/mipmap-$d/ic_launcher_foreground.png" ] \
    || { echo "adaptive foreground missing for $d" >&2; exit 1; }
  [ -f "$RES/mipmap-$d/ic_launcher_monochrome.png" ] \
    || { echo "monochrome icon missing for $d" >&2; exit 1; }
done
echo "generated Wayshard Android launcher icons from the canonical mark"
