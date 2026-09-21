#!/usr/bin/env bash
# Prove the official CLI+TUI archive layout: canonical internal runtime names,
# bundled notices, preserved executable mode, and a runnable pair after
# extraction (no manual rename, no WAYSHARD_TUI override, no PATH lookup).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

fail() { echo "package_cli_tui_test: $*" >&2; exit 1; }

# ---- Unix archive: real Go CLI + stand-in companion -------------------------
mkdir -p "$TMP/stage-unix"
( cd "$ROOT" && go build -o "$TMP/stage-unix/wayshard" ./cmd/wayshard )
cat > "$TMP/stage-unix/wayshard-tui" <<'EOF'
#!/bin/sh
printf 'Wayshard TUI test companion\n'
EOF
chmod 0755 "$TMP/stage-unix/wayshard-tui"

mkdir -p "$TMP/out-unix"
bash "$ROOT/scripts/release/package-cli-tui.sh" v9.9.9 linux amd64 "$TMP/stage-unix" "$TMP/out-unix"
ARCHIVE="$TMP/out-unix/wayshard-v9.9.9-linux-amd64.tar.gz"
[[ -f "$ARCHIVE" ]] || fail "missing unix archive $ARCHIVE"

tar -tzf "$ARCHIVE" | sort > "$TMP/unix-list.txt"
for f in wayshard wayshard-tui LICENSE NOTICE THIRD_PARTY_NOTICES.md; do
  grep -qx "$f" "$TMP/unix-list.txt" || fail "unix archive missing $f"
done
# Executable mode is preserved for the binaries, not the notices.
tar -tzvf "$ARCHIVE" > "$TMP/unix-list-verbose.txt"
grep -Eq '^-rwxr-xr-x[[:space:]].* wayshard$' "$TMP/unix-list-verbose.txt" || fail "wayshard not executable in archive"
grep -Eq '^-rwxr-xr-x[[:space:]].* wayshard-tui$' "$TMP/unix-list-verbose.txt" || fail "wayshard-tui not executable in archive"

mkdir -p "$TMP/extract-unix"
tar -xzf "$ARCHIVE" -C "$TMP/extract-unix"
[[ -x "$TMP/extract-unix/wayshard" ]] || fail "extracted wayshard is not executable"
[[ -x "$TMP/extract-unix/wayshard-tui" ]] || fail "extracted wayshard-tui is not executable"

cd "$TMP/extract-unix"
# The companion is deliberately not on PATH; resolution must be adjacent.
if PATH="$TMP/empty-path" command -v wayshard-tui >/dev/null 2>&1; then
  fail "companion unexpectedly resolvable through PATH"
fi
help_out="$(env -u WAYSHARD_TUI ./wayshard help 2>&1)" || fail "wayshard help failed"
printf '%s\n' "$help_out" | grep -q 'interactive client' || fail "help output missing the interactive-client entry"
if printf '%s\n' "$help_out" | grep -q 'wayshard>'; then fail "retired prompt appeared in help output"; fi
launch_out="$(env -u WAYSHARD_TUI ./wayshard 2>&1)" || fail "no-argument launch failed"
printf '%s\n' "$launch_out" | grep -q 'Wayshard TUI test companion' || fail "no-argument launch did not reach the adjacent companion"

# ---- Windows archive (structural) -------------------------------------------
mkdir -p "$TMP/stage-win" "$TMP/out-win"
printf 'cli' > "$TMP/stage-win/wayshard.exe"
printf 'tui' > "$TMP/stage-win/wayshard-tui.exe"
bash "$ROOT/scripts/release/package-cli-tui.sh" v9.9.9 windows amd64 "$TMP/stage-win" "$TMP/out-win"
ZIP="$TMP/out-win/wayshard-v9.9.9-windows-amd64.zip"
[[ -f "$ZIP" ]] || fail "missing windows archive $ZIP"
python3 - "$ZIP" <<'PY'
import sys, zipfile
zf = zipfile.ZipFile(sys.argv[1])
names = set(zf.namelist())
want = {"wayshard.exe", "wayshard-tui.exe", "LICENSE", "NOTICE", "THIRD_PARTY_NOTICES.md"}
missing = want - names
if missing:
    raise SystemExit(f"windows archive missing {sorted(missing)}")
for exe in ("wayshard.exe", "wayshard-tui.exe"):
    mode = (zf.getinfo(exe).external_attr >> 16) & 0o777
    if mode != 0o755:
        raise SystemExit(f"{exe} stored mode is {oct(mode)}, want 0o755")
print("windows archive layout ok")
PY

echo "package-cli-tui test ok"
