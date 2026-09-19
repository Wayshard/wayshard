#!/usr/bin/env python3
"""Ensure Tauri-generated app/build.gradle.kts signs release with keystore.properties."""

from __future__ import annotations

import sys
from pathlib import Path

IMPORTS = """import java.util.Properties
"""

SIGNING_BLOCK = '''
    signingConfigs {
        create("release") {
            val keystorePropertiesFile = rootProject.file("keystore.properties")
            if (!keystorePropertiesFile.exists()) {
                error("keystore.properties is required for a signed Wayshard Android release")
            }
            val keystoreProperties = Properties()
            keystorePropertiesFile.inputStream().use { keystoreProperties.load(it) }
            keyAlias = keystoreProperties.getProperty("keyAlias")
                ?: error("keyAlias missing from keystore.properties")
            keyPassword = keystoreProperties.getProperty("keyPassword")
                ?: keystoreProperties.getProperty("password")
                ?: error("keyPassword/password missing from keystore.properties")
            storeFile = file(keystoreProperties.getProperty("storeFile")
                ?: error("storeFile missing from keystore.properties"))
            storePassword = keystoreProperties.getProperty("storePassword")
                ?: keystoreProperties.getProperty("password")
                ?: error("storePassword/password missing from keystore.properties")
        }
    }
'''


def patch(path: Path) -> None:
    text = path.read_text(encoding="utf-8")
    if "import java.util.Properties" not in text:
        text = IMPORTS + text
    if "keystore.properties" not in text:
        needle = "android {"
        idx = text.find(needle)
        if idx < 0:
            raise SystemExit(f"{path}: no android block")
        insert = text.find("\n", idx) + 1
        text = text[:insert] + SIGNING_BLOCK + text[insert:]
    if 'signingConfig = signingConfigs.getByName("release")' not in text:
        marker = 'getByName("release") {'
        idx = text.find(marker)
        if idx < 0:
            raise SystemExit(f"{path}: no release buildType")
        brace = text.find("{", idx)
        text = (
            text[: brace + 1]
            + '\n            signingConfig = signingConfigs.getByName("release")'
            + text[brace + 1 :]
        )
    path.write_text(text, encoding="utf-8")
    print(f"patched {path}")


if __name__ == "__main__":
    if len(sys.argv) != 2:
        raise SystemExit("usage: android-patch-gradle.py <app/build.gradle.kts>")
    patch(Path(sys.argv[1]))
