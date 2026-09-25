#!/usr/bin/env bash
# Self-test for verify-windows-installer-icon.py.
#
# Uses makensis to build two throwaway NSIS installers: one told to use the
# Wayshard installer icon and one left to fall back to NSIS's default icon. The
# gate must pass only for the branded installer, so a regression that drops
# `bundle.windows.nsis.installerIcon` is caught here as well as at release time.
#
# makensis is required when WAYSHARD_REQUIRE_NSIS=1 (set in CI), otherwise the
# test skips cleanly.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
VERIFY="$ROOT/scripts/release/verify-windows-installer-icon.py"
ICO="$ROOT/assets/branding/wayshard.ico"

if [[ ! -f "$ICO" ]]; then
  echo "missing $ICO" >&2
  exit 1
fi

if ! command -v makensis >/dev/null 2>&1; then
  if [[ "${WAYSHARD_REQUIRE_NSIS:-0}" == "1" ]]; then
    echo "makensis is required for the Windows installer icon test" >&2
    exit 1
  fi
  echo "makensis unavailable; skipping Windows installer icon self-test"
  exit 0
fi

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

build_installer() { # <outfile> <icon-define>
  cat > "$TMP/build.nsi" <<EOF
Unicode true
!include "MUI2.nsh"
Name "Wayshard Icon Test"
OutFile "$1"
InstallDir "\$LOCALAPPDATA\\Wayshard"
$2
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_LANGUAGE "English"
Section
SectionEnd
EOF
  ( cd "$TMP" && makensis build.nsi >/dev/null )
}

build_installer "$TMP/branded.exe" "!define MUI_ICON \"$ICO\""
build_installer "$TMP/default.exe" ""

python3 "$VERIFY" "$TMP/branded.exe" >/dev/null \
  || { echo "branded installer failed the icon gate" >&2; exit 1; }
if python3 "$VERIFY" "$TMP/default.exe" >/dev/null 2>&1; then
  echo "default-icon installer wrongly passed the icon gate" >&2
  exit 1
fi
echo "windows installer icon test ok"
