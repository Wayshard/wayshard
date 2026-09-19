#!/usr/bin/env bash
# Set Tauri/Cargo/Android versions from a git tag.
# v0.1.0-rc.1 -> tauri/cargo 0.1.0-rc.1, Android versionCode 10001 (below stable 0.1.0).
set -euo pipefail
TAG="${1:?usage: set-tauri-version.sh <tag>}"
CONF="${2:-clients/desktop/src-tauri/tauri.conf.json}"
CARGO="${3:-clients/desktop/src-tauri/Cargo.toml}"
VER="${TAG#v}"
python3 - "$CONF" "$CARGO" "$VER" <<'PY'
import json, re, sys
from pathlib import Path

conf_path, cargo_path, ver = sys.argv[1], sys.argv[2], sys.argv[3]


def android_version_code(v: str) -> int:
    core, _, pre = v.partition("-")
    parts = (core.split(".") + ["0", "0", "0"])[:3]
    try:
        major, minor, patch = (int(p) for p in parts)
    except ValueError as e:
        raise SystemExit(f"invalid version {v!r}: {e}") from e
    base = major * 10_000_000 + minor * 10_000 + patch * 100
    if not pre:
        return base + 99
    n = 1
    for label in ("rc.", "beta.", "alpha."):
        if pre.startswith(label):
            rest = pre[len(label) :]
            num = rest.split(".")[0].split("+")[0]
            try:
                n = int(num)
            except ValueError:
                n = 1
            break
    if n < 1:
        n = 1
    if n > 98:
        n = 98
    return base + n


code = android_version_code(ver)
path = Path(conf_path)
doc = json.loads(path.read_text(encoding="utf-8"))
doc["version"] = ver
android = doc.setdefault("bundle", {}).setdefault("android", {})
android["versionCode"] = code
path.write_text(json.dumps(doc, indent=2) + "\n", encoding="utf-8")
print(f"set {path} version={ver} android.versionCode={code}")

cargo = Path(cargo_path)
if cargo.is_file():
    text = cargo.read_text(encoding="utf-8")
    new, n = re.subn(
        r'(?m)^version\s*=\s*"[^"]*"',
        f'version = "{ver}"',
        text,
        count=1,
    )
    if n != 1:
        raise SystemExit(f"{cargo}: could not replace package version")
    cargo.write_text(new, encoding="utf-8")
    print(f"set {cargo} version={ver}")
PY
