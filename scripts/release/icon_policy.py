#!/usr/bin/env python3
"""Verify the canonical Wayshard mark and its platform icon derivatives.

Checks (dependency-free, pure Python):
  * `clients/desktop/src-tauri/app-icon.json` references the canonical mark and
    every referenced asset exists and is a square PNG;
  * the Android adaptive foreground keeps the mark inside the platform safe
    circle (so the launcher mask cannot crop it);
  * the committed desktop/Web icon derivatives exist at their platform sizes;
  * the release workflow generates Android icons through a pinned, icon-manifest
    capable Tauri CLI (>= 2.9.0).

Usage: icon_policy.py [repo-root]
"""

from __future__ import annotations

import json
import re
import struct
import sys
import zlib
from pathlib import Path

# Android adaptive icons are a 108dp layer with a 66dp guaranteed-visible safe
# circle; keep the mark inside it.
SAFE_CIRCLE_RATIO = 0.62

DESKTOP_ICONS = {
    "icons/32x32.png": (32, 32),
    "icons/128x128.png": (128, 128),
    "icons/128x128@2x.png": (256, 256),
    "icons/icon.png": (512, 512),
    "icons/icon.icns": None,
    "icons/icon.ico": None,
}
WEB_ICONS = {
    "app-icon-192.png": (192, 192),
    "app-icon-512.png": (512, 512),
    "app-icon-maskable-192.png": (192, 192),
    "app-icon-maskable-512.png": (512, 512),
    "apple-touch-icon.png": (180, 180),
    "favicon-16x16.png": (16, 16),
    "favicon-32x32.png": (32, 32),
    "favicon-96x96.png": (96, 96),
}


def png_header(path: Path) -> tuple[int, int]:
    """Return (width, height) from the IHDR chunk without decoding pixels."""
    data = path.read_bytes()
    if data[:8] != b"\x89PNG\r\n\x1a\n":
        raise ValueError(f"{path}: not a PNG")
    if data[12:16] != b"IHDR":
        raise ValueError(f"{path}: missing IHDR")
    width, height = struct.unpack_from(">II", data, 16)
    return width, height


def read_png(path: Path) -> tuple[int, int, int, bytes]:
    """Return (width, height, channels, pixels) for an 8-bit non-interlaced PNG."""
    data = path.read_bytes()
    if data[:8] != b"\x89PNG\r\n\x1a\n":
        raise ValueError(f"{path}: not a PNG")
    pos = 8
    idat = bytearray()
    width = height = colortype = None
    while pos + 12 <= len(data):
        length = struct.unpack_from(">I", data, pos)[0]
        ctype = data[pos + 4 : pos + 8]
        chunk = data[pos + 8 : pos + 8 + length]
        pos += 12 + length
        if ctype == b"IHDR":
            width, height, bitdepth, colortype, _comp, _filt, interlace = struct.unpack(
                ">IIBBBBB", chunk
            )
            if bitdepth != 8 or interlace != 0:
                raise ValueError(f"{path}: only 8-bit non-interlaced PNGs are supported")
        elif ctype == b"IDAT":
            idat += chunk
        elif ctype == b"IEND":
            break
    if width is None or height is None:
        raise ValueError(f"{path}: missing IHDR")
    channels = {0: 1, 2: 3, 3: 1, 4: 2, 6: 4}.get(colortype)
    if channels is None:
        raise ValueError(f"{path}: unsupported color type {colortype}")

    raw = zlib.decompress(bytes(idat))
    stride = width * channels
    prev = bytearray(stride)
    out = bytearray(height * stride)
    p = 0
    for y in range(height):
        filt = raw[p]
        p += 1
        line = bytearray(raw[p : p + stride])
        p += stride
        if filt == 1:
            for i in range(channels, stride):
                line[i] = (line[i] + line[i - channels]) & 0xFF
        elif filt == 2:
            for i in range(stride):
                line[i] = (line[i] + prev[i]) & 0xFF
        elif filt == 3:
            for i in range(stride):
                a = line[i - channels] if i >= channels else 0
                line[i] = (line[i] + ((a + prev[i]) >> 1)) & 0xFF
        elif filt == 4:
            for i in range(stride):
                a = line[i - channels] if i >= channels else 0
                b = prev[i]
                c = prev[i - channels] if i >= channels else 0
                pp = a + b - c
                pa, pb, pc = abs(pp - a), abs(pp - b), abs(pp - c)
                pr = a if (pa <= pb and pa <= pc) else (b if pb <= pc else c)
                line[i] = (line[i] + pr) & 0xFF
        elif filt != 0:
            raise ValueError(f"{path}: unknown PNG filter {filt}")
        out[y * stride : (y + 1) * stride] = line
        prev = line
    return width, height, channels, bytes(out)


def alpha_bbox(path: Path) -> tuple[int, int, int, int, int, int]:
    """Return (w, h, x0, y0, x1, y1) for non-transparent content."""
    w, h, channels, px = read_png(path)
    if channels not in (2, 4):
        # RGB: everything is opaque.
        return w, h, 0, 0, w, h
    alpha_off = channels - 1
    x0, y0, x1, y1 = w, h, -1, -1
    for y in range(h):
        row = y * w * channels
        for x in range(w):
            if px[row + x * channels + alpha_off] > 0:
                if x < x0:
                    x0 = x
                if x > x1:
                    x1 = x
                if y < y0:
                    y0 = y
                if y > y1:
                    y1 = y
    if x1 < 0:
        raise ValueError(f"{path}: fully transparent")
    return w, h, x0, y0, x1 + 1, y1 + 1


def check_png_size(path: Path, expected: tuple[int, int]) -> None:
    w, h = png_header(path)
    if (w, h) != expected:
        raise SystemExit(f"{path}: expected {expected[0]}x{expected[1]}, got {w}x{h}")


def check_desktop_icons(root: Path) -> None:
    tauri_conf = root / "clients/desktop/src-tauri/tauri.conf.json"
    conf = json.loads(tauri_conf.read_text(encoding="utf-8"))
    listed = conf.get("bundle", {}).get("icon", [])
    if not listed:
        raise SystemExit(f"{tauri_conf}: bundle.icon must list the packaged app icons")
    base = tauri_conf.parent
    for rel in listed:
        path = (base / rel).resolve()
        if not path.is_file():
            raise SystemExit(f"{tauri_conf}: bundle.icon entry is missing: {rel}")
        expected = DESKTOP_ICONS.get(rel, "skip")
        if expected == "skip":
            raise SystemExit(f"{tauri_conf}: unexpected bundle.icon entry {rel!r}")
        if expected is not None:
            check_png_size(path, expected)


def parse_icon_cli_version(script: str) -> tuple[int, int, int]:
    patterns = (
        r"WAYSHARD_ICON_CLI_VERSION:-(\d+)\.(\d+)\.(\d+)",
        r"ICON_CLI_VERSION:?-?\s*=\s*\"?(\d+)\.(\d+)\.(\d+)",
        r"cli@(\d+)\.(\d+)\.(\d+)",
    )
    for pattern in patterns:
        m = re.search(pattern, script)
        if m:
            return int(m.group(1)), int(m.group(2)), int(m.group(3))
    raise SystemExit("android-icons.sh does not pin a Tauri CLI version")


def main() -> None:
    root = Path(sys.argv[1] if len(sys.argv) > 1 else ".").resolve()

    manifest_path = root / "clients/desktop/src-tauri/app-icon.json"
    if not manifest_path.is_file():
        raise SystemExit(f"missing {manifest_path}")
    manifest = json.loads(manifest_path.read_text(encoding="utf-8"))
    base = manifest_path.parent

    for key in ("default", "android_fg", "android_monochrome", "android_bg"):
        if key not in manifest:
            continue
        target = (base / manifest[key]).resolve()
        if not target.is_file():
            raise SystemExit(f"app-icon.json {key!r} points at a missing file: {target}")
        if target.suffix != ".png":
            raise SystemExit(f"app-icon.json {key!r} must reference a PNG: {target}")

    default = (base / manifest["default"]).resolve()
    canonical = (root / "assets/branding/wayshard.png").resolve()
    if default != canonical:
        raise SystemExit(f"app-icon.json default must be the canonical mark, got {default}")

    fg = (base / manifest["android_fg"]).resolve()
    w, h, x0, y0, x1, y1 = alpha_bbox(fg)
    diagonal = ((x1 - x0) ** 2 + (y1 - y0) ** 2) ** 0.5
    if diagonal > SAFE_CIRCLE_RATIO * w:
        raise SystemExit(
            f"{fg}: mark diagonal {diagonal:.1f}px exceeds the adaptive safe circle "
            f"({SAFE_CIRCLE_RATIO * w:.1f}px of {w}px) and would be cropped by the launcher mask"
        )

    check_desktop_icons(root)
    for rel, size in WEB_ICONS.items():
        check_png_size(root / "clients/web/public" / rel, size)

    script = (root / "scripts/release/android-icons.sh").read_text(encoding="utf-8")
    if "app-icon.json" not in script:
        raise SystemExit("android-icons.sh must generate from app-icon.json")
    version = parse_icon_cli_version(script)
    if version < (2, 9, 0):
        raise SystemExit(
            f"android-icons.sh pins tauri-cli {'.'.join(map(str, version))}, which cannot read an icon manifest (needs >= 2.9.0)"
        )

    workflow = (root / ".github/workflows/release.yml").read_text(encoding="utf-8")
    if "android-icons.sh" not in workflow:
        raise SystemExit("release.yml must generate Android icons with android-icons.sh")
    if "tauri icon src-tauri/app-icon.json" in workflow:
        raise SystemExit("release.yml must not invoke the pinned 2.5.0 CLI on the icon manifest")

    print(
        "icon policy ok: canonical mark, adaptive safe-area foreground, "
        f"desktop/Web derivatives, tauri-cli {'.'.join(map(str, version))} icon generator"
    )


if __name__ == "__main__":
    main()
