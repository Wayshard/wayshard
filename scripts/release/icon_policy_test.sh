#!/usr/bin/env bash
# Self-test for icon_policy.py: the real tree passes, and the safe-area check is
# meaningful -- the raw canonical mark (unpadded) exceeds the Android adaptive
# safe circle, while the committed Android foreground stays inside it.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
python3 "$ROOT/scripts/release/icon_policy.py" "$ROOT"

python3 - "$ROOT" <<'PY'
import importlib.util
import sys
from pathlib import Path

root = Path(sys.argv[1])
spec = importlib.util.spec_from_file_location(
    "icon_policy", root / "scripts/release/icon_policy.py"
)
policy = importlib.util.module_from_spec(spec)
spec.loader.exec_module(policy)


def diagonal_ratio(path):
    w, _h, x0, y0, x1, y1 = policy.alpha_bbox(path)
    return ((x1 - x0) ** 2 + (y1 - y0) ** 2) ** 0.5 / w


canonical = root / "assets/branding/wayshard.png"
foreground = root / "assets/branding/wayshard-android-fg.png"
if diagonal_ratio(canonical) <= policy.SAFE_CIRCLE_RATIO:
    raise SystemExit("fixture invalid: canonical mark unexpectedly fits the safe circle")
if diagonal_ratio(foreground) > policy.SAFE_CIRCLE_RATIO:
    raise SystemExit("Android foreground exceeds the adaptive safe circle")
print("icon policy safe-area check is meaningful")
PY
echo "icon policy test ok"
