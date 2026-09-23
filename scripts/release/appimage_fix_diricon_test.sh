#!/usr/bin/env bash
# Self-test for appimage-fix-diricon.sh against a synthetic type-2 AppImage.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

for tool in unsquashfs mksquashfs python3; do
  command -v "$tool" >/dev/null 2>&1 || { echo "SKIP: missing $tool"; exit 0; }
done

# Build a synthetic type-2 AppImage: fake runtime bytes + a squashfs payload.
mkdir -p "$TMP/AppDir/usr/bin"
echo "icon" > "$TMP/AppDir/Wayshard.png"
printf '[Desktop Entry]\nName=Wayshard\n' > "$TMP/AppDir/Wayshard.desktop"
echo "binary" > "$TMP/AppDir/usr/bin/wayshard-desktop"
# The broken form: an absolute, dangling .DirIcon symlink to the build machine.
ln -s "/home/runner/work/wayshard/wayshard/Wayshard.AppDir/Wayshard.png" "$TMP/AppDir/.DirIcon"
mksquashfs "$TMP/AppDir" "$TMP/payload.squashfs" -root-owned -noappend -no-progress -comp gzip >/dev/null
printf 'FAKEAPPIMAGE_RUNTIME' > "$TMP/runtime"
cat "$TMP/runtime" "$TMP/payload.squashfs" > "$TMP/test.AppImage"
chmod +x "$TMP/test.AppImage"

before_offset="$(python3 - "$TMP/test.AppImage" <<'PY'
import sys
print(open(sys.argv[1], "rb").read(1 << 20).find(b"hsqs"))
PY
)"

bash "$ROOT/scripts/release/appimage-fix-diricon.sh" "$TMP/test.AppImage"

after_offset="$(python3 - "$TMP/test.AppImage" <<'PY'
import sys
print(open(sys.argv[1], "rb").read(1 << 20).find(b"hsqs"))
PY
)"
if [ "$before_offset" != "$after_offset" ]; then
  echo "runtime offset changed: $before_offset -> $after_offset" >&2
  exit 1
fi

# The runtime prefix must be preserved byte-for-byte.
head -c "$after_offset" "$TMP/test.AppImage" > "$TMP/runtime.after"
cmp -s "$TMP/runtime" "$TMP/runtime.after" || { echo "runtime bytes changed" >&2; exit 1; }

# Extract the patched payload and verify .DirIcon is now relative and resolves.
tail -c +"$((after_offset + 1))" "$TMP/test.AppImage" > "$TMP/patched.squashfs"
unsquashfs -no-progress -d "$TMP/patched" "$TMP/patched.squashfs" >/dev/null
target="$(readlink "$TMP/patched/.DirIcon")"
if [ "$target" != "Wayshard.png" ]; then
  echo ".DirIcon target = $target, want Wayshard.png" >&2
  exit 1
fi
[ -e "$TMP/patched/.DirIcon" ] || { echo ".DirIcon still dangles" >&2; exit 1; }

echo "appimage-fix-diricon test ok"
