#!/usr/bin/env bash
# Inject the Android Keystore credential helper into the generated Tauri Android
# project. Run after `tauri android init` (the gen/ project is not committed).
#
# The helper is placed beside the generated MainActivity.kt so its package always
# matches the app identifier, and only the package declaration is rewritten.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
GEN="${1:-$ROOT/clients/desktop/src-tauri/gen/android}"
SRC="$ROOT/clients/desktop/android/WayshardKeystore.kt"

[ -f "$SRC" ] || { echo "missing $SRC" >&2; exit 1; }
JAVA_ROOT="$GEN/app/src/main/java"
[ -d "$JAVA_ROOT" ] || { echo "missing generated Android project at $GEN; run: bunx tauri android init --ci" >&2; exit 1; }

MAIN="$(find "$JAVA_ROOT" -name MainActivity.kt -print -quit)"
[ -n "$MAIN" ] || { echo "MainActivity.kt not found under $JAVA_ROOT; run: bunx tauri android init --ci" >&2; exit 1; }
DIR="$(dirname "$MAIN")"

PKG="$(python3 - "$DIR" "$JAVA_ROOT" <<'PY'
import os, sys
rel = os.path.relpath(sys.argv[1], sys.argv[2])
print(rel.replace(os.sep, "."))
PY
)"
[ -n "$PKG" ] || { echo "could not derive the Android package from $DIR" >&2; exit 1; }

DEST="$DIR/WayshardKeystore.kt"
python3 - "$SRC" "$DEST" "$PKG" <<'PY'
import re, sys
src, dest, pkg = sys.argv[1], sys.argv[2], sys.argv[3]
text = open(src, encoding="utf-8").read()
text = re.sub(r"(?m)^package\s+.*$", f"package {pkg}", text, count=1)
open(dest, "w", encoding="utf-8").write(text)
PY

grep -q "AndroidKeyStore" "$DEST" || { echo "injected helper does not use AndroidKeyStore" >&2; exit 1; }
grep -q "AES/GCM/NoPadding" "$DEST" || { echo "injected helper does not use AES/GCM/NoPadding" >&2; exit 1; }
echo "installed WayshardKeystore.kt (package $PKG) at $DEST"
