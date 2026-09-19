#!/usr/bin/env bash
set -euo pipefail
if [[ -n "${WAYSHARD_WINDOWS_PFX_FILE:-}" ]]; then
  rm -f "$WAYSHARD_WINDOWS_PFX_FILE"
fi
find "${RUNNER_TEMP:-/tmp}" -maxdepth 2 -name 'wayshard-windows-codesign.pfx' -delete 2>/dev/null || true
echo "windows signing material removed"
