#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
cat > "$TMP/tauri.conf.json" <<'EOF'
{
  "productName": "Wayshard",
  "version": "0.0.0",
  "bundle": { "android": { "minSdkVersion": 26 } }
}
EOF
cat > "$TMP/Cargo.toml" <<'EOF'
[package]
name = "wayshard-desktop"
version = "0.0.0"
EOF
bash "$ROOT/scripts/release/set-tauri-version.sh" "v0.1.0-rc.1" "$TMP/tauri.conf.json" "$TMP/Cargo.toml"
python3 - "$TMP/tauri.conf.json" "$TMP/Cargo.toml" <<'PY'
import json, sys
from pathlib import Path
conf = json.loads(Path(sys.argv[1]).read_text(encoding="utf-8"))
assert conf["version"] == "0.1.0-rc.1", conf["version"]
assert conf["bundle"]["android"]["versionCode"] == 10001, conf["bundle"]["android"]["versionCode"]
cargo = Path(sys.argv[2]).read_text(encoding="utf-8")
assert 'version = "0.1.0-rc.1"' in cargo
print("rc version mapping ok")
PY
bash "$ROOT/scripts/release/set-tauri-version.sh" "v0.1.0" "$TMP/tauri.conf.json" "$TMP/Cargo.toml"
python3 - "$TMP/tauri.conf.json" <<'PY'
import json, sys
from pathlib import Path
conf = json.loads(Path(sys.argv[1]).read_text(encoding="utf-8"))
assert conf["version"] == "0.1.0"
assert conf["bundle"]["android"]["versionCode"] == 10099
print("stable version mapping ok")
PY
echo "set-tauri-version test ok"
