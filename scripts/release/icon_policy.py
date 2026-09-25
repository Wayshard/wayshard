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


# --- Windows installer icon ------------------------------------------------- #

BRANDING_ICO = "assets/branding/wayshard.ico"
REQUIRED_ICO_FRAMES = {16, 32, 48, 256}
# The 16x16 frame is too coarse for a tight pixel comparison (each 8x8 grid cell
# covers only 2x2 pixels), so it gets a looser bound than the larger frames.
ICO_FRAME_DIFF = 10.0
ICO_SMALL_FRAME_DIFF = 22.0


def _decode_bmp_dib(data: bytes) -> tuple[int, int, bytes]:
    """Decode a 32-bit BI_RGB BMP DIB (an ICO frame) into RGBA bytes."""
    bi_size = struct.unpack_from("<I", data, 0)[0]
    width = struct.unpack_from("<i", data, 4)[0]
    height = struct.unpack_from("<i", data, 8)[0]
    bitcount = struct.unpack_from("<H", data, 14)[0]
    compression = struct.unpack_from("<I", data, 16)[0]
    if bitcount != 32 or compression != 0 or width <= 0 or height <= 0:
        raise ValueError(f"unsupported ICO frame (bpp={bitcount}, compression={compression})")
    h = height // 2  # the DIB height doubles the visible height (XOR + AND bitmaps)
    stride = width * 4
    off = bi_size
    out = bytearray(width * h * 4)
    for y in range(h):
        src = off + (h - 1 - y) * stride  # BMP rows are bottom-up
        for x in range(width):
            b, g, r, a = data[src + x * 4 : src + x * 4 + 4]
            o = (y * width + x) * 4
            out[o : o + 4] = bytes((r, g, b, a))
    return width, h, bytes(out)


def ico_frames(path: Path) -> list[tuple[int, int, bytes]]:
    """Return (width, height, rgba_bytes) for every frame of a BMP-based ICO."""
    data = path.read_bytes()
    reserved, icon_type, count = struct.unpack_from("<HHH", data, 0)
    if reserved != 0 or icon_type != 1 or count < 1:
        raise ValueError(f"{path}: not a valid ICO")
    frames = []
    for i in range(count):
        _w, _h, _c, _r, _p, _bpp, size, offset = struct.unpack_from(
            "<BBBBHHII", data, 6 + 16 * i
        )
        blob = data[offset : offset + size]
        if blob[:8] == b"\x89PNG\r\n\x1a\n":
            raise ValueError(f"{path}: PNG-compressed ICO frames are not supported here")
        fw, fh, rgba = _decode_bmp_dib(blob)
        frames.append((fw, fh, rgba))
    return frames


def _box_grid(px: bytes, w: int, h: int, n: int = 8) -> list[float]:
    """Box-average an RGBA image into an n*n grid (resampling-robust fingerprint)."""
    out: list[float] = []
    for gy in range(n):
        y0, y1 = gy * h // n, (gy + 1) * h // n
        for gx in range(n):
            x0, x1 = gx * w // n, (gx + 1) * w // n
            acc = [0, 0, 0, 0]
            count = 0
            for y in range(y0, y1):
                row = y * w * 4
                for x in range(x0, x1):
                    o = row + x * 4
                    acc[0] += px[o]
                    acc[1] += px[o + 1]
                    acc[2] += px[o + 2]
                    acc[3] += px[o + 3]
                    count += 1
            out.extend(v / count for v in acc)
    return out


def check_branding_ico(root: Path) -> None:
    """The Windows installer icon must be the canonical mark at Windows sizes."""
    ico = root / BRANDING_ICO
    if not ico.is_file():
        raise SystemExit(f"missing Windows installer icon {ico}")
    frames = ico_frames(ico)
    sizes = {w for w, _h, _px in frames}
    missing = REQUIRED_ICO_FRAMES - sizes
    if missing:
        raise SystemExit(f"{ico}: missing required Windows frames {sorted(missing)}")
    for w, _h, rgba in frames:
        if all(rgba[i] == 255 for i in range(3, len(rgba), 4)):
            raise SystemExit(f"{ico}: {w}x{w} frame has no transparency")

    cw, ch, cchan, cpx = read_png(root / "assets/branding/wayshard.png")
    if cchan != 4:
        raise SystemExit("canonical mark must be 8-bit RGBA")
    canonical = _box_grid(cpx, cw, ch)
    for w, h, rgba in frames:
        grid = _box_grid(rgba, w, h)
        diff = sum(abs(a - b) for a, b in zip(canonical, grid)) / len(canonical)
        bound = ICO_SMALL_FRAME_DIFF if w <= 16 else ICO_FRAME_DIFF
        if diff > bound:
            raise SystemExit(
                f"{ico}: {w}x{h} frame is not derived from the canonical mark (diff {diff:.1f} > {bound})"
            )


def check_tauri_nsis_icon(root: Path) -> None:
    """The Tauri NSIS config must name the Wayshard installer icon explicitly."""
    conf_path = root / "clients/desktop/src-tauri/tauri.conf.json"
    conf = json.loads(conf_path.read_text(encoding="utf-8"))
    nsis = (((conf.get("bundle") or {}).get("windows") or {}).get("nsis")) or {}
    icon = nsis.get("installerIcon")
    if not icon:
        raise SystemExit(
            f"{conf_path}: bundle.windows.nsis.installerIcon must be set "
            "(NSIS would otherwise fall back to the default Tauri/NSIS installer icon)"
        )
    resolved = (conf_path.parent / icon).resolve()
    expected = (root / BRANDING_ICO).resolve()
    if resolved != expected:
        raise SystemExit(
            f"{conf_path}: installerIcon must point at {BRANDING_ICO}, got {icon!r}"
        )
    if not resolved.is_file():
        raise SystemExit(f"{conf_path}: installerIcon does not exist: {icon}")


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

    check_branding_ico(root)
    check_tauri_nsis_icon(root)

    print(
        "icon policy ok: canonical mark, adaptive safe-area foreground, "
        f"desktop/Web derivatives, Windows installer icon, "
        f"tauri-cli {'.'.join(map(str, version))} icon generator"
    )


if __name__ == "__main__":
    main()
