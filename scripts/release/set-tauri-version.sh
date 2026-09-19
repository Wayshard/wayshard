#!/usr/bin/env bash
# Set clients/desktop/src-tauri/tauri.conf.json version from a git tag (v1.2.3 -> 1.2.3).
set -euo pipefail
TAG="${1:?usage: set-tauri-version.sh <tag>}"
CONF="${2:-clients/desktop/src-tauri/tauri.conf.json}"
VER="${TAG#v}"
python3 - "$CONF" "$VER" <<'PY'
import json, sys
path, ver = sys.argv[1], sys.argv[2]
with open(path, encoding="utf-8") as f:
    doc = json.load(f)
doc["version"] = ver
with open(path, "w", encoding="utf-8") as f:
    json.dump(doc, f, indent=2)
    f.write("\n")
print(f"set {path} version to {ver}")
PY
