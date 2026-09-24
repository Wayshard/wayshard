#!/usr/bin/env bash
# Rewrite a type-2 AppImage's .DirIcon symlink to a relative path inside the
# image.
#
# linuxdeploy/Tauri can leave .DirIcon as an absolute symlink pointing at the
# build machine's AppDir, which dangles once the AppImage is published. The
# payload offset is taken from the AppImage runtime itself
# (`--appimage-offset`), never from magic-byte scanning: a naive `hsqs` search
# matches a decoy inside the runtime before the real SquashFS superblock. The
# original runtime ELF is preserved byte-for-byte.
#
# Requires squashfs-tools (unsquashfs/mksquashfs), python3 and stat.
set -euo pipefail

APPIMAGE="${1:?usage: appimage-fix-diricon.sh <appimage>}"
# Normalize to an absolute path so a bare relative name is not PATH-searched.
case "$APPIMAGE" in
  /*) : ;;
  *) APPIMAGE="$PWD/$APPIMAGE" ;;
esac
[ -f "$APPIMAGE" ] || { echo "missing $APPIMAGE" >&2; exit 1; }
[ -x "$APPIMAGE" ] || chmod +x "$APPIMAGE"

for tool in unsquashfs mksquashfs python3 stat; do
  command -v "$tool" >/dev/null 2>&1 || { echo "missing required tool: $tool" >&2; exit 1; }
done

# The runtime reports the numeric payload offset; reject anything non-numeric.
offset="$("$APPIMAGE" --appimage-offset 2>/dev/null || true)"
case "$offset" in
  '' | *[!0-9]*)
    echo "AppImage runtime did not report a numeric --appimage-offset (got '$offset')" >&2
    exit 1
    ;;
esac
[ "$offset" -gt 0 ] || { echo "invalid AppImage payload offset: $offset" >&2; exit 1; }

size="$(stat -c%s "$APPIMAGE")"
[ "$offset" -lt "$size" ] || {
  echo "AppImage payload offset $offset is not before the file size $size" >&2
  exit 1
}

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

head -c "$offset" "$APPIMAGE" > "$work/runtime"
tail -c +"$((offset + 1))" "$APPIMAGE" > "$work/payload.squashfs"

# The payload at the reported offset must actually be a SquashFS superblock.
magic="$(head -c 4 "$work/payload.squashfs" || true)"
[ "$magic" = "hsqs" ] || {
  echo "AppImage payload does not start with a SquashFS superblock (got '$magic')" >&2
  exit 1
}

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

# Validate the repacked payload before replacing the original.
newmagic="$(head -c 4 "$work/new.squashfs" || true)"
[ "$newmagic" = "hsqs" ] || { echo "repacked payload is not a SquashFS image" >&2; exit 1; }
unsquashfs -no-progress -ll "$work/new.squashfs" >/dev/null

out="$APPIMAGE.fixed"
cat "$work/runtime" "$work/new.squashfs" > "$out"
chmod +x "$out"
mv "$out" "$APPIMAGE"
echo "fixed .DirIcon in $APPIMAGE (payload offset $offset)"
