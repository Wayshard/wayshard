#!/usr/bin/env python3
"""Release gate: the Android Keystore JNI helper must survive R8 in the APK.

``dev.wayshard.app.WayshardKeystore`` is reached only from Rust over JNI
(``JNIEnv::find_class`` + ``call_static_method``). R8 cannot see a Java/Kotlin
reference, so the minified release build strips or renames the class and its
``encrypt``/``decrypt``/``deleteKey`` methods unless the ProGuard keep rules are
in effect. This gate inspects the *built* ``classes*.dex`` and fails closed if
the class or any required static method is absent.

Usage:
  android-keystore-verify.py [--package dev.wayshard.app] <apk-or-dex>
  android-keystore-verify.py --self-test

Exit status is non-zero when the helper is missing or renamed.
"""

from __future__ import annotations

import argparse
import hashlib
import io
import re
import struct
import sys
import zipfile
import zlib
from pathlib import Path

DEFAULT_PACKAGE = "dev.wayshard.app"
CLASS_SIMPLE = "WayshardKeystore"
REQUIRED_METHODS = ("encrypt", "decrypt", "deleteKey")

ACC_STATIC = 0x0008
NO_INDEX = 0xFFFFFFFF


def target_descriptor(package: str) -> str:
    return "L" + package.replace(".", "/") + "/" + CLASS_SIMPLE + ";"


# --------------------------------------------------------------------------- #
# DEX parsing
# --------------------------------------------------------------------------- #


def _uleb128(data: bytes, off: int) -> tuple[int, int]:
    result = 0
    shift = 0
    while True:
        if off >= len(data):
            raise ValueError("truncated uleb128")
        byte = data[off]
        off += 1
        result |= (byte & 0x7F) << shift
        if (byte & 0x80) == 0:
            return result, off
        shift += 7
        if shift > 35:
            raise ValueError("uleb128 is too long")


def _cstr(data: bytes, off: int) -> bytes:
    end = data.find(b"\x00", off)
    if end < 0:
        raise ValueError("unterminated string data")
    return data[off:end]


def _parse_class_data_methods(data: bytes, off: int) -> list[tuple[int, int]]:
    """Return ``(method_idx, access_flags)`` for every method the class defines."""
    static_fields, off = _uleb128(data, off)
    instance_fields, off = _uleb128(data, off)
    direct_methods, off = _uleb128(data, off)
    virtual_methods, off = _uleb128(data, off)

    for _ in range(static_fields + instance_fields):
        _, off = _uleb128(data, off)  # field_idx_diff
        _, off = _uleb128(data, off)  # access_flags

    methods: list[tuple[int, int]] = []
    idx = 0
    for _ in range(direct_methods + virtual_methods):
        diff, off = _uleb128(data, off)
        flags, off = _uleb128(data, off)
        _, off = _uleb128(data, off)  # code_off
        idx += diff
        methods.append((idx, flags))
    return methods


def scan_dex(data: bytes, descriptor: str) -> dict | None:
    """Describe the target class in one dex, or ``None`` when it is absent."""
    if len(data) < 0x70 or data[:4] != b"dex\n":
        raise ValueError("not a dex file")
    (
        string_ids_size,
        string_ids_off,
        type_ids_size,
        type_ids_off,
        proto_ids_size,
        proto_ids_off,
        _field_ids_size,
        _field_ids_off,
        method_ids_size,
        method_ids_off,
        class_defs_size,
        class_defs_off,
    ) = struct.unpack_from("<12I", data, 0x38)

    strings: list[str] = []
    for i in range(string_ids_size):
        (off,) = struct.unpack_from("<I", data, string_ids_off + 4 * i)
        _, pos = _uleb128(data, off)
        strings.append(_cstr(data, pos).decode("utf-8", "replace"))

    types: list[str] = []
    for i in range(type_ids_size):
        (descriptor_idx,) = struct.unpack_from("<I", data, type_ids_off + 4 * i)
        types.append(strings[descriptor_idx])

    method_names: dict[int, str] = {}
    method_class: dict[int, str] = {}
    for i in range(method_ids_size):
        class_idx, _proto_idx, name_idx = struct.unpack_from(
            "<HHI", data, method_ids_off + 8 * i
        )
        method_class[i] = types[class_idx]
        method_names[i] = strings[name_idx]

    classes: dict[str, int] = {}
    for i in range(class_defs_size):
        class_idx, _access, _super, _ifaces, _source, _ann, class_data_off, _values = (
            struct.unpack_from("<8I", data, class_defs_off + 32 * i)
        )
        classes[types[class_idx]] = class_data_off

    if descriptor not in classes:
        return None

    owned = {i: name for i, name in method_names.items() if method_class[i] == descriptor}
    any_names = set(owned.values())
    static_names: set[str] = set()
    class_data_off = classes[descriptor]
    if class_data_off:
        for method_idx, flags in _parse_class_data_methods(data, class_data_off):
            if method_idx in owned and (flags & ACC_STATIC):
                static_names.add(owned[method_idx])

    return {
        "any": any_names,
        "static": static_names,
        "has_class_data": bool(class_data_off),
    }


def _dex_blobs(path: Path) -> list[tuple[str, bytes]]:
    data = path.read_bytes()
    if data[:4] == b"dex\n":
        return [(path.name, data)]
    with zipfile.ZipFile(io.BytesIO(data)) as zf:
        names = sorted(
            (n for n in zf.namelist() if re.fullmatch(r"classes\d*\.dex", n)),
            key=lambda n: (len(n), n),
        )
        return [(n, zf.read(n)) for n in names]


def verify_bytes(blobs: list[tuple[str, bytes]], package: str) -> tuple[bool, str]:
    descriptor = target_descriptor(package)
    required = set(REQUIRED_METHODS)
    for name, data in blobs:
        info = scan_dex(data, descriptor)
        if info is None:
            continue
        if info["has_class_data"]:
            missing = sorted(required - info["static"])
            label = "static "
        else:
            missing = sorted(required - info["any"])
            label = ""
        if missing:
            return False, f"{name}: {descriptor} is missing {label}methods {missing}"
        return True, f"{name}: {descriptor} present with {sorted(required)}"
    return False, f"{descriptor} is absent (R8 stripped or renamed the helper)"


def verify_path(path: Path, package: str) -> tuple[bool, str]:
    if not path.is_file():
        return False, f"{path} does not exist"
    blobs = _dex_blobs(path)
    if not blobs:
        return False, f"{path}: no classes*.dex found"
    return verify_bytes(blobs, package)


# --------------------------------------------------------------------------- #
# Synthetic DEX builder (self-test only)
# --------------------------------------------------------------------------- #


def _uleb128_bytes(value: int) -> bytes:
    out = bytearray()
    while True:
        byte = value & 0x7F
        value >>= 7
        if value:
            out.append(byte | 0x80)
        else:
            out.append(byte)
            return bytes(out)


def build_dex(
    class_descriptor: str = "Ldev/wayshard/app/WayshardKeystore;",
    method_names: tuple[str, ...] = REQUIRED_METHODS,
) -> bytes:
    """Build a minimal, well-formed dex defining static ``(String)->String`` methods.

    Every table is ordered as the dex format (and dexdump's verifier) requires:
    string_ids by content, type_ids by descriptor string index, method_ids by
    (class, name, proto) and class_data by ascending method index.
    """
    strings = sorted(
        {
            class_descriptor,
            "Ljava/lang/String;",
            "Ljava/lang/Object;",
            "LL",
            *method_names,
        }
    )
    str_index = {text: i for i, text in enumerate(strings)}

    type_descriptors = sorted(
        {"Ljava/lang/String;", class_descriptor, "Ljava/lang/Object;"},
        key=lambda descriptor: str_index[descriptor],
    )
    type_index = {descriptor: i for i, descriptor in enumerate(type_descriptors)}

    protos = [
        {
            "shorty": str_index["LL"],
            "return": type_index["Ljava/lang/String;"],
            "params": [type_index["Ljava/lang/String;"]],
        }
    ]

    methods = sorted(
        (type_index[class_descriptor], str_index[name], 0) for name in method_names
    )

    class_data = bytearray()
    class_data += _uleb128_bytes(0)  # static fields
    class_data += _uleb128_bytes(0)  # instance fields
    class_data += _uleb128_bytes(len(methods))  # direct methods
    class_data += _uleb128_bytes(0)  # virtual methods
    prev = 0
    for i in range(len(methods)):
        class_data += _uleb128_bytes(i - prev)  # method_idx_diff
        # public static native: a code item is optional for native methods, so
        # the synthetic dex stays well-formed without emitting bytecode.
        class_data += _uleb128_bytes(ACC_STATIC | 0x0001 | 0x0100)
        class_data += _uleb128_bytes(0)  # code_off
        prev = i

    header_size = 0x70
    string_ids_off = header_size
    type_ids_off = string_ids_off + 4 * len(strings)
    proto_ids_off = type_ids_off + 4 * len(type_descriptors)
    method_ids_off = proto_ids_off + 12 * len(protos)
    class_defs_off = method_ids_off + 8 * len(methods)
    data_off = class_defs_off + 32  # one class def

    data = bytearray()
    string_offsets: list[int] = []
    for text in strings:
        string_offsets.append(data_off + len(data))
        raw = text.encode("utf-8")
        data += _uleb128_bytes(len(raw)) + raw + b"\x00"
    while (data_off + len(data)) % 4:
        data += b"\x00"
    type_list_off = data_off + len(data)
    params = protos[0]["params"]
    data += struct.pack("<I", len(params))
    for param in params:
        data += struct.pack("<HH", param, 0)
    class_data_off = data_off + len(data)
    data += class_data
    while (data_off + len(data)) % 4:
        data += b"\x00"
    map_off = data_off + len(data)

    map_items = [
        (0x0000, 1, 0),
        (0x0001, len(strings), string_ids_off),
        (0x0002, len(type_descriptors), type_ids_off),
        (0x0003, len(protos), proto_ids_off),
        (0x0005, len(methods), method_ids_off),
        (0x0006, 1, class_defs_off),
        (0x2002, len(strings), string_offsets[0]),
        (0x1001, 1, type_list_off),
        (0x2000, 1, class_data_off),
        (0x1000, 1, map_off),
    ]
    # Real dex files (and dexdump's verifier) require map items in ascending
    # offset order, with the map_list itself last.
    map_items.sort(key=lambda item: item[2])
    data += struct.pack("<I", len(map_items))
    for item_type, size, offset in map_items:
        data += struct.pack("<HHII", item_type, 0, size, offset)

    file_size = data_off + len(data)
    class_def = (
        type_index[class_descriptor],
        0x0001,  # public
        type_index["Ljava/lang/Object;"],
        0,
        NO_INDEX,
        0,
        class_data_off,
        0,
    )

    out = bytearray()
    out += b"dex\n035\x00"
    out += b"\x00" * 4  # checksum
    out += b"\x00" * 20  # signature
    out += struct.pack("<I", file_size)
    out += struct.pack("<I", header_size)
    out += struct.pack("<I", 0x12345678)
    out += struct.pack("<I", 0)  # link_size
    out += struct.pack("<I", 0)  # link_off
    out += struct.pack("<I", map_off)
    out += struct.pack("<II", len(strings), string_ids_off)
    out += struct.pack("<II", len(type_descriptors), type_ids_off)
    out += struct.pack("<II", len(protos), proto_ids_off)
    out += struct.pack("<II", 0, 0)  # field_ids
    out += struct.pack("<II", len(methods), method_ids_off)
    out += struct.pack("<II", 1, class_defs_off)
    out += struct.pack("<II", len(data), data_off)
    for offset in string_offsets:
        out += struct.pack("<I", offset)
    for descriptor in type_descriptors:
        out += struct.pack("<I", str_index[descriptor])
    for proto in protos:
        out += struct.pack("<III", proto["shorty"], proto["return"], type_list_off)
    for class_idx, name_idx, proto_idx in methods:
        out += struct.pack("<HHI", class_idx, proto_idx, name_idx)
    out += struct.pack("<8I", *class_def)
    out += data

    out[12:32] = hashlib.sha1(out[32:]).digest()
    struct.pack_into("<I", out, 8, zlib.adler32(out[12:]) & 0xFFFFFFFF)
    return bytes(out)


def _zip_apk(dex: bytes) -> bytes:
    buf = io.BytesIO()
    with zipfile.ZipFile(buf, "w", zipfile.ZIP_STORED) as zf:
        zf.writestr("classes.dex", dex)
    return buf.getvalue()


def self_test() -> None:
    import tempfile

    good = build_dex()
    if scan_dex(good, target_descriptor(DEFAULT_PACKAGE)) is None:
        raise SystemExit("self-test: builder did not produce a recognised class")
    ok, detail = verify_bytes([("classes.dex", good)], DEFAULT_PACKAGE)
    if not ok:
        raise SystemExit(f"self-test: valid helper rejected: {detail}")

    with tempfile.TemporaryDirectory() as td:
        apk = Path(td) / "app.apk"
        apk.write_bytes(_zip_apk(good))
        ok, detail = verify_path(apk, DEFAULT_PACKAGE)
        if not ok:
            raise SystemExit(f"self-test: valid APK rejected: {detail}")
        stripped_apk = Path(td) / "stripped.apk"
        stripped_apk.write_bytes(_zip_apk(build_dex(class_descriptor="Ldev/wayshard/app/Other;")))
        ok, detail = verify_path(stripped_apk, DEFAULT_PACKAGE)
        if ok:
            raise SystemExit("self-test: stripped APK was accepted")

    renamed = build_dex(method_names=("encrypt", "decrypt", "deleteKeyX"))
    ok, detail = verify_bytes([("classes.dex", renamed)], DEFAULT_PACKAGE)
    if ok:
        raise SystemExit("self-test: renamed method was accepted")
    if "deleteKey" not in detail:
        raise SystemExit(f"self-test: rename not reported: {detail}")

    wrong_class = build_dex(class_descriptor="Ldev/wayshard/app/Other;")
    ok, detail = verify_bytes([("classes.dex", wrong_class)], DEFAULT_PACKAGE)
    if ok or "absent" not in detail:
        raise SystemExit(f"self-test: stripped class was accepted: {detail}")

    partial = build_dex(method_names=("encrypt",))
    ok, _ = verify_bytes([("classes.dex", partial)], DEFAULT_PACKAGE)
    if ok:
        raise SystemExit("self-test: partial helper was accepted")

    print("android-keystore-verify self-test ok")


def main() -> None:
    ap = argparse.ArgumentParser()
    ap.add_argument("--package", default=DEFAULT_PACKAGE)
    ap.add_argument("--self-test", action="store_true")
    ap.add_argument("path", nargs="?")
    args = ap.parse_args()

    if args.self_test:
        self_test()
        return
    if not args.path:
        raise SystemExit("usage: android-keystore-verify.py [--package PKG] <apk-or-dex>")

    ok, detail = verify_path(Path(args.path), args.package)
    if not ok:
        raise SystemExit(f"android-keystore-verify: {detail}")
    print(f"android-keystore-verify: {detail}")


if __name__ == "__main__":
    main()
