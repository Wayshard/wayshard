#!/usr/bin/env bash
# Rewrite a type-2 AppImage's .DirIcon symlink to a relative path inside the
# image.
#
# linuxdeploy/Tauri can leave .DirIcon as an absolute symlink pointing at the
# build machine's AppDir, which dangles once the AppImage is published. This
# repacks the AppImage with a relative .DirIcon while preserving the original
# runtime ELF, so the published icon resolves on the user's machine.
#
# Requires squashfs-tools (unsquashfs/mksquashfs) and python3.
set -euo pipefail

APPIMAGE="${1:?usage: appimage-fix-diricon.sh <appimage>}"
[ -f "$APPIMAGE" ] || { echo "missing $APPIMAGE" >&2; exit 1; }

for tool in unsquashfs mksquashfs python3; do
  command -v "$tool" >/dev/null 2>&1 || { echo "missing required tool: $tool" >&2; exit 1; }
done

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

# Locate the squashfs magic that marks the start of the payload.
offset="$(python3 - "$APPIMAGE" <<'PY'
import sys
with open(sys.argv[1], "rb") as f:
    data = f.read(1 << 20)
print(data.find(b"hsqs"))
PY
)"
if [ "$offset" -le 0 ]; then
  echo "not a type-2 AppImage (no squashfs payload): $APPIMAGE" >&2
  exit 1
fi

head -c "$offset" "$APPIMAGE" > "$work/runtime"
tail -c +"$((offset + 1))" "$APPIMAGE" > "$work/payload.squashfs"
unsquashfs -no-progress -d "$work/appdir" "$work/payload.squashfs" >/dev/null

appdir="$work/appdir"
[ -d "$appdir" ] || { echo "extraction produced no AppDir" >&2; exit 1; }

# Make .DirIcon a relative symlink to the icon that sits beside it.
if [ -L "$appdir/.DirIcon" ]; then
  base="$(basename "$(readlink "$appdir/.DirIcon")")"
  if [ -n "$base" ] && [ -e "$appdir/$base" ]; then
    ln -sfn "$base" "$appdir/.DirIcon"
  fi
fi

mksquashfs "$appdir" "$work/new.squashfs" -root-owned -noappend -no-progress -comp gzip >/dev/null

out="$APPIMAGE.fixed"
cat "$work/runtime" "$work/new.squashfs" > "$out"
chmod +x "$out"
mv "$out" "$APPIMAGE"
echo "fixed .DirIcon in $APPIMAGE"
