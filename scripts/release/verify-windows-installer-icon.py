#!/usr/bin/env python3
"""Release gate: the built Windows NSIS `-setup.exe` must embed the Wayshard icon.

Tauri's NSIS bundler compiles the configured `bundle.windows.nsis.installerIcon`
into the installer's PE icon resources. NSIS copies each ICO frame byte-for-byte
(verified independently against a `makensis` build), so if the installer falls
back to the default Tauri/NSIS icon, none of the Wayshard `wayshard.ico` frame
payloads appear in the executable. This gate fails closed in that case.

Dependency-free (stdlib only) so it runs on the Windows release runner without
extra packages.

Usage:
  verify-windows-installer-icon.py <setup.exe> [--ico assets/branding/wayshard.ico]
"""

from __future__ import annotations

import argparse
import struct
import sys
from pathlib import Path


def ico_frames(path: Path) -> list[tuple[int, int, bytes]]:
    """Return (width, height, image_bytes) for each ICO frame."""
    data = path.read_bytes()
    reserved, icon_type, count = struct.unpack_from("<HHH", data, 0)
    if reserved != 0 or icon_type != 1 or count < 1:
        raise ValueError(f"{path}: not a valid ICO (type={icon_type}, count={count})")
    frames = []
    for i in range(count):
        width, height, _colors, _res, _planes, _bpp, size, offset = struct.unpack_from(
            "<BBBBHHII", data, 6 + 16 * i
        )
        if offset + size > len(data):
            raise ValueError(f"{path}: frame {i} extends past end of file")
        frames.append((width or 256, height or 256, data[offset:offset + size]))
    return frames


def main() -> None:
    ap = argparse.ArgumentParser()
    ap.add_argument("exe")
    ap.add_argument("--ico", default=None)
    args = ap.parse_args()

    exe = Path(args.exe)
    if not exe.is_file():
        raise SystemExit(f"verify-windows-installer-icon: missing {exe}")
    ico = Path(args.ico) if args.ico else Path(__file__).resolve().parents[2] / "assets/branding/wayshard.ico"
    if not ico.is_file():
        raise SystemExit(f"verify-windows-installer-icon: missing {ico}")

    frames = ico_frames(ico)
    payload = exe.read_bytes()

    missing: list[str] = []
    present = 0
    for width, height, blob in frames:
        if blob in payload:
            present += 1
        else:
            missing.append(f"{width}x{height}")

    if missing:
        raise SystemExit(
            "verify-windows-installer-icon: the installer does not embed the Wayshard icon "
            f"(missing frames: {', '.join(missing)} of {len(frames)}). "
            "NSIS fell back to the default Tauri/NSIS installer icon."
        )

    print(
        f"verify-windows-installer-icon: {exe.name} embeds all {present}/{len(frames)} "
        f"Wayshard installer icon frames from {ico.name}"
    )


if __name__ == "__main__":
    sys.exit(main())
