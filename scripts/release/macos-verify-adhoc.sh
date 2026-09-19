#!/usr/bin/env bash
# Confirm macOS bundles carry an ad-hoc signature (identity "-"), not unsigned
# and not Developer ID. No Apple account is used.
set -euo pipefail
DEST="${1:?usage: macos-verify-adhoc.sh <dir-or-app>}"

verify_one() {
  local path="$1"
  echo "codesign -dv $path"
  local out
  out="$(codesign -dv --verbose=4 "$path" 2>&1 || true)"
  echo "$out"
  if echo "$out" | grep -qi 'code object is not signed'; then
    echo "$path is unsigned; official macOS artifacts must be ad-hoc signed" >&2
    return 1
  fi
  if echo "$out" | grep -qi 'Authority=Developer ID'; then
    echo "$path has Developer ID; this release policy forbids Apple Developer signing" >&2
    return 1
  fi
  if echo "$out" | grep -Eqi 'Signature=(adhoc|adhoc code)|Identifier=|compiled for'; then
    # ad-hoc typically shows Signature=adhoc
    if echo "$out" | grep -qi 'Signature=adhoc'; then
      echo "ad-hoc signature ok: $path"
      codesign --verify --verbose=4 "$path"
      return 0
    fi
  fi
  # Some tools only print "flags=0x2(adhoc)"
  if echo "$out" | grep -qi adhoc; then
    echo "ad-hoc signature ok: $path"
    codesign --verify --verbose=4 "$path"
    return 0
  fi
  echo "could not confirm ad-hoc signature on $path" >&2
  return 1
}

found_app=0
while IFS= read -r -d '' f; do
  found_app=1
  verify_one "$f"
done < <(find "$DEST" -name '*.app' -print0 2>/dev/null)

if [[ "$found_app" -eq 0 ]]; then
  echo "no .app under $DEST; official macOS artifacts must include an ad-hoc signed application" >&2
  exit 1
fi

# The DMG is a container. Ad-hoc policy applies to the .app; an unsigned DMG is expected.
if find "$DEST" -name '*.dmg' -print -quit | grep -q .; then
  echo "dmg container present (ad-hoc requirement is on the .app)"
fi
