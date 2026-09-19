#!/usr/bin/env python3
"""Write SHA-256 checksums for each file exactly once.

Usage:
  checksums.py --out SHA256SUMS.txt FILE [FILE ...]
  checksums.py --self-test

Skips checksum files themselves (SHA256SUMS*). Hashes by resolved path so
overlapping globs cannot emit duplicate rows for the same file. Fails if two
different files would share a checksum-file basename (ambiguous download).
"""

from __future__ import annotations

import argparse
import hashlib
import os
import sys
from pathlib import Path


def is_sums_name(name: str) -> bool:
    n = name.upper()
    return n == "SHA256SUMS.TXT" or n.startswith("SHA256SUMS")


def sha256_file(path: Path) -> str:
    h = hashlib.sha256()
    with path.open("rb") as f:
        for chunk in iter(lambda: f.read(1024 * 1024), b""):
            h.update(chunk)
    return h.hexdigest()


def collect(paths: list[str]) -> list[Path]:
    seen: set[Path] = set()
    out: list[Path] = []
    for raw in paths:
        p = Path(raw)
        if not p.exists() or not p.is_file():
            continue
        if is_sums_name(p.name) or p.name.endswith(".minisig"):
            continue
        key = p.resolve()
        if key in seen:
            continue
        seen.add(key)
        out.append(p)
    out.sort(key=lambda p: p.name)
    return out


def write_sums(files: list[Path], dest: Path) -> None:
    names: dict[str, Path] = {}
    lines: list[str] = []
    for p in files:
        if p.name in names:
            raise SystemExit(
                f"ambiguous checksum basename {p.name!r}: {names[p.name]} and {p}"
            )
        names[p.name] = p
        lines.append(f"{sha256_file(p)}  {p.name}\n")
    dest.parent.mkdir(parents=True, exist_ok=True)
    dest.write_text("".join(lines), encoding="utf-8")


def self_test() -> None:
    import tempfile

    with tempfile.TemporaryDirectory() as td:
        d = Path(td)
        a = d / "wayshard-server-v0.0.0-linux-amd64"
        b = d / "wayshard-v0.0.0-linux-amd64"
        a.write_bytes(b"server")
        b.write_bytes(b"cli")
        (d / "SHA256SUMS.txt").write_text("junk\n", encoding="utf-8")
        (d / "SHA256SUMS.txt.minisig").write_text("sig\n", encoding="utf-8")
        files = collect(
            [
                str(a),
                str(b),
                str(a),  # duplicate path
                str(d / "SHA256SUMS.txt"),
                str(d / "SHA256SUMS.txt.minisig"),
            ]
        )
        if [p.name for p in files] != [
            "wayshard-server-v0.0.0-linux-amd64",
            "wayshard-v0.0.0-linux-amd64",
        ]:
            raise SystemExit(f"collect failed: {files}")
        out = d / "out.txt"
        write_sums(files, out)
        rows = out.read_text(encoding="utf-8").strip().splitlines()
        if len(rows) != 2:
            raise SystemExit(f"expected 2 rows, got {rows!r}")
        hashes = {line.split()[0] for line in rows}
        if len(hashes) != 2:
            raise SystemExit("duplicate hashes for distinct files")
        names = [line.split()[-1] for line in rows]
        if names.count("wayshard-server-v0.0.0-linux-amd64") != 1:
            raise SystemExit("server checksum duplicated")
    print("checksums.py self-test ok")


def main() -> None:
    ap = argparse.ArgumentParser()
    ap.add_argument("--out", help="output SHA256SUMS.txt path")
    ap.add_argument("--self-test", action="store_true")
    ap.add_argument("files", nargs="*")
    args = ap.parse_args()
    if args.self_test:
        self_test()
        return
    if not args.out:
        raise SystemExit("--out is required")
    files = collect(args.files)
    if not files:
        raise SystemExit("no files to checksum")
    write_sums(files, Path(args.out))
    print(f"wrote {args.out} ({len(files)} files)")


if __name__ == "__main__":
    main()
