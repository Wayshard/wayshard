#!/usr/bin/env bash
# Regression self-test for appimage-fix-diricon.sh: the fix must use the
# runtime's --appimage-offset, not magic-byte scanning. The synthetic AppImage
# contains a decoy "hsqs" inside the runtime before the real SquashFS payload,
# so a naive scan would slice at the decoy and fail.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

for tool in unsquashfs mksquashfs python3 stat; do
  command -v "$tool" >/dev/null 2>&1 || { echo "SKIP: missing $tool"; exit 0; }
done

# Build a real SquashFS payload with the broken absolute .DirIcon.
mkdir -p "$TMP/AppDir/usr/bin"
echo "icon" > "$TMP/AppDir/Wayshard.png"
printf '[Desktop Entry]\nName=Wayshard\n' > "$TMP/AppDir/Wayshard.desktop"
echo "binary" > "$TMP/AppDir/usr/bin/wayshard-desktop"
ln -s "/home/runner/work/wayshard/wayshard/Wayshard.AppDir/Wayshard.png" "$TMP/AppDir/.DirIcon"
mksquashfs "$TMP/AppDir" "$TMP/payload.squashfs" -root-owned -noappend -no-progress -comp gzip >/dev/null

# Synthetic type-2 AppImage: an executable runtime that prints the real payload
# offset, padded to a fixed length with a decoy "hsqs" before the payload.
RUNTIME_LEN=4096
python3 - "$TMP/test.AppImage" "$RUNTIME_LEN" "$TMP/payload.squashfs" <<'PY'
import sys

out, runtime_len, payload = sys.argv[1], int(sys.argv[2]), sys.argv[3]
script = "#!/bin/sh\n# synthetic AppImage runtime for the regression test\necho %d\n" % runtime_len
blob = bytearray(script.encode())
if len(blob) > runtime_len:
    raise SystemExit("runtime script too long")
blob += b"#" * (runtime_len - len(blob))  # shell-comment padding
decoy = b"hsqsDECOY"
start = runtime_len - len(decoy) - 1
blob[start : start + len(decoy)] = decoy
assert len(blob) == runtime_len
with open(out, "wb") as f:
    f.write(bytes(blob))
    with open(payload, "rb") as p:
        f.write(p.read())
print("built AppImage: decoy hsqs at %d, payload at %d" % (start, runtime_len))
PY
chmod +x "$TMP/test.AppImage"
head -c "$RUNTIME_LEN" "$TMP/test.AppImage" > "$TMP/runtime.before"

bash "$ROOT/scripts/release/appimage-fix-diricon.sh" "$TMP/test.AppImage"

# The runtime ELF prefix is preserved byte-for-byte (including the decoy).
head -c "$RUNTIME_LEN" "$TMP/test.AppImage" > "$TMP/runtime.after"
cmp -s "$TMP/runtime.before" "$TMP/runtime.after" || { echo "runtime prefix changed" >&2; exit 1; }
grep -aq "hsqsDECOY" "$TMP/runtime.after" || { echo "decoy marker lost" >&2; exit 1; }

# The repacked payload extracts and .DirIcon is relative and resolves.
tail -c +"$((RUNTIME_LEN + 1))" "$TMP/test.AppImage" > "$TMP/patched.squashfs"
unsquashfs -no-progress -d "$TMP/patched" "$TMP/patched.squashfs" >/dev/null
target="$(readlink "$TMP/patched/.DirIcon")"
[ "$target" = "Wayshard.png" ] || { echo ".DirIcon target = $target, want Wayshard.png" >&2; exit 1; }
[ -e "$TMP/patched/.DirIcon" ] || { echo ".DirIcon still dangles" >&2; exit 1; }

# A runtime that does not report a numeric offset must fail closed.
cat > "$TMP/bad.AppImage" <<'EOF'
#!/bin/sh
echo "not-a-number"
EOF
chmod +x "$TMP/bad.AppImage"
if bash "$ROOT/scripts/release/appimage-fix-diricon.sh" "$TMP/bad.AppImage" >/dev/null 2>&1; then
  echo "fix must fail closed when --appimage-offset is not numeric" >&2
  exit 1
fi

echo "appimage-fix-diricon test ok (decoy hsqs ignored)"
