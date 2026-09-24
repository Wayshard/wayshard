#!/usr/bin/env bash
# Fail closed: PR CI stays secret-free; release.yml uses only maintainer-owned
# signing secrets; no paid/platform signing accounts.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
CI="$ROOT/.github/workflows/ci.yml"
REL="$ROOT/.github/workflows/release.yml"
TAURI="$ROOT/clients/desktop/src-tauri/tauri.conf.json"

fail() { echo "release_policy_test: $*" >&2; exit 1; }

if grep -E 'secrets\.|environment:' "$CI"; then
  fail "ci.yml must not reference secrets or environments"
fi
if grep -E 'TYPESAFE_API_KEY|WAYSHARD_VAULT' "$CI"; then
  fail "ci.yml must not reference runtime vault/Jev secrets"
fi

# The Android-target Rust path (#[cfg(target_os = "android")] command bodies)
# must be compiled in CI so it cannot escape host cargo test/check.
grep -q 'make android-check' "$CI" || fail "ci.yml must run the Android-target Rust check (make android-check)"

grep -q 'environment: release' "$REL" || fail "release.yml must use environment: release"
grep -q "github.repository == 'Wayshard/wayshard'" "$REL" || fail "official publish must be gated to Wayshard/wayshard"

required=(
  WAYSHARD_ANDROID_KEYSTORE_BASE64
  WAYSHARD_ANDROID_KEYSTORE_PASSWORD
  WAYSHARD_ANDROID_KEY_ALIAS
  WAYSHARD_ANDROID_KEY_PASSWORD
  WAYSHARD_WINDOWS_PFX_BASE64
  WAYSHARD_WINDOWS_PFX_PASSWORD
  WAYSHARD_RELEASE_MINISIGN_KEY_BASE64
  WAYSHARD_RELEASE_MINISIGN_PASSWORD
)
for n in "${required[@]}"; do
  grep -q "$n" "$REL" || fail "release.yml missing $n"
done

forbidden='APPLE_ID|APP_STORE_CONNECT|APPLE_API_KEY|NOTARIZE|NOTARYTOOL|DEVELOPER_ID_APPLICATION|AZURE_|SM_CLIENT_CERT|DIGICERT|GOOGLE_PLAY|PLAY_STORE|PLAY_CONSOLE|TAURI_SIGNING_PRIVATE_KEY|CSC_LINK|CSC_KEY_PASSWORD|WINDOWS_CERTIFICATE_THUMBPRINT'
if grep -E "$forbidden" "$REL" "$TAURI"; then
  fail "paid/platform signing identifiers must not appear as required workflow config"
fi

grep -q 'APPLE_SIGNING_IDENTITY: "-"' "$REL" || fail "macOS must ad-hoc sign with identity -"
grep -q 'prerelease:' "$REL" || fail "release.yml must classify prerelease tags"
test -s "$ROOT/clients/desktop/src-tauri/icons/icon.png" || fail "missing Tauri icon.png"
test -s "$ROOT/clients/desktop/src-tauri/icons/icon.ico" || fail "missing Tauri icon.ico"
test -s "$ROOT/clients/desktop/src-tauri/icons/icon.icns" || fail "missing Tauri icon.icns"
grep -q 'packages: platform-tools' "$REL" || fail "android job must not install obsolete sdkmanager tools package"
grep -q 'github.ref_name' "$REL" || fail "tag version scripts must use github.ref_name, not PowerShell \${GITHUB_REF_NAME}"
grep -q "contains(github.ref_name, '-rc.')" "$REL" || fail "release.yml must treat -rc. tags as prereleases"
grep -q 'make_latest:' "$REL" || fail "release.yml must not promote prereleases to latest"
python3 - "$TAURI" <<'PY'
import json, sys
cfg = json.load(open(sys.argv[1], encoding="utf-8"))
ident = cfg.get("bundle", {}).get("macOS", {}).get("signingIdentity")
if ident != "-":
    raise SystemExit(f"tauri macOS signingIdentity must be '-', got {ident!r}")
plugins = cfg.get("plugins") or {}
if "updater" in plugins:
    raise SystemExit("tauri updater plugin must not be configured")
print("tauri ad-hoc signing, no updater")
PY

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
if bash "$ROOT/scripts/release/android-sign.sh" >"$TMP/android.err" 2>&1; then
  fail "android-sign.sh must fail when secrets are missing"
fi
grep -q 'WAYSHARD_ANDROID_KEYSTORE_BASE64' "$TMP/android.err" || fail "android-sign.sh must name the missing secret"

echo "policy" > "$TMP/SHA256SUMS.txt"
if bash "$ROOT/scripts/release/minisign-sign.sh" "$TMP/SHA256SUMS.txt" >"$TMP/minisign.err" 2>&1; then
  fail "minisign-sign.sh must fail when secrets are missing"
fi
grep -q 'WAYSHARD_RELEASE_MINISIGN_KEY_BASE64' "$TMP/minisign.err" || fail "minisign-sign.sh must name the missing secret"

# --- CLI+TUI distribution unit and deterministic checksum coverage -----------
test -s "$ROOT/scripts/release/package-cli-tui.sh" || fail "missing scripts/release/package-cli-tui.sh"
grep -q 'package-cli-tui.sh' "$REL" || fail "release tui job must package the CLI+TUI bundle with package-cli-tui.sh"
grep -q 'package-cli-tui.sh' "$ROOT/scripts/ci/tui_smoke.sh" || fail "tui smoke must exercise the real packaging script"
grep -q 'wayshard-tui' "$REL" || fail "release tui job must build the wayshard-tui companion"

python3 - "$REL" "$ROOT/scripts/release/package-cli-tui.sh" <<'PY'
import re
import sys

text = open(sys.argv[1], encoding="utf-8").read()


def job_block(name):
    m = re.search(rf"^  {re.escape(name)}:\s*$", text, re.M)
    if not m:
        raise SystemExit(f"release.yml is missing job {name!r}")
    rest = text[m.end():]
    end = re.search(r"^  [A-Za-z0-9_-]+:\s*$", rest, re.M)
    return rest[: end.start()] if end else rest


checksums = job_block("checksums")
m = re.search(r"^    needs:\s*(.+)$", checksums, re.M)
if not m:
    raise SystemExit("checksums job must declare needs")
raw = m.group(1).strip()
if raw.startswith("["):
    deps = {d.strip() for d in raw.strip("[]").split(",") if d.strip()}
else:
    deps = set(re.findall(r"^\s+-\s+(\S+)", checksums[m.end():], re.M))
required = {"release", "tui", "desktop", "android"}
missing = required - deps
if missing:
    raise SystemExit(
        f"combined checksums must wait for every release producer; missing {sorted(missing)}"
    )

# The checksum job must verify the CLI+TUI bundles before signing the manifest.
if "Verify every expected CLI+TUI bundle is present" not in checksums:
    raise SystemExit("checksums job must assert all CLI+TUI bundles exist before minisigning")

script = open(sys.argv[2], encoding="utf-8").read()
for needle in ("wayshard-tui", "wayshard-${VERSION}", ".tar.gz", ".zip"):
    if needle not in script:
        raise SystemExit(f"package-cli-tui.sh must produce {needle!r}")
print("release asset dependency + bundle graph ok")
PY

# --- CLI+TUI runner labels (regression: retired macos-13) ---------------------
python3 - "$REL" <<'PY'
import re
import sys

text = open(sys.argv[1], encoding="utf-8").read()

if "macos-13" in text:
    raise SystemExit("release.yml must not pin any job to the retired macos-13 runner")

m = re.search(r"^  tui:\s*$", text, re.M)
if not m:
    raise SystemExit("release.yml is missing the tui job")
rest = text[m.end():]
end = re.search(r"^  [A-Za-z0-9_-]+:\s*$", rest, re.M)
block = rest[: end.start()] if end else rest

entries = []
for em in re.finditer(r"^          - os: (\S+)\n((?:            \S+: \S+\n)+)", block, re.M):
    fields = dict(re.findall(r"^            (\w+): (\S+)$", em.group(2), re.M))
    fields["os"] = em.group(1)
    entries.append(fields)
by_asset = {e.get("asset"): e for e in entries}

expected_assets = {"linux-amd64", "linux-arm64", "darwin-amd64", "darwin-arm64", "windows-amd64"}
missing = expected_assets - set(by_asset)
if missing:
    raise SystemExit(f"CLI+TUI matrix is missing assets: {sorted(missing)}")

darwin_amd64 = by_asset["darwin-amd64"]
want = {"os": "macos-15-intel", "target": "bun-darwin-x64", "goos": "darwin", "goarch": "amd64"}
for key, value in want.items():
    if darwin_amd64.get(key) != value:
        raise SystemExit(
            f"darwin-amd64 TUI matrix entry must set {key}={value!r}, got {darwin_amd64.get(key)!r}"
        )

darwin_arm64 = by_asset["darwin-arm64"]
if darwin_arm64.get("target") != "bun-darwin-arm64" or darwin_arm64.get("goarch") != "arm64":
    raise SystemExit(f"darwin-arm64 TUI matrix entry changed unexpectedly: {darwin_arm64!r}")

print("darwin-amd64 uses a supported Intel runner; CLI+TUI matrix complete")
PY

# --- Desktop platform coverage, reproducible Go stamping, AppImage icon -------
python3 - "$REL" <<'PY'
import re
import sys

text = open(sys.argv[1], encoding="utf-8").read()

m = re.search(r"^  desktop:\s*$", text, re.M)
if not m:
    raise SystemExit("release.yml is missing the desktop job")
rest = text[m.end():]
end = re.search(r"^  [A-Za-z0-9_-]+:\s*$", rest, re.M)
block = rest[: end.start()] if end else rest
if "macos-15-intel" not in block:
    raise SystemExit("desktop matrix must build Intel macOS (macos-15-intel)")
if block.count("target: macos") < 2:
    raise SystemExit("desktop matrix must build both macOS architectures")

if "rm -rf internal/webembed/dist" in text:
    raise SystemExit(
        "release.yml must preserve internal/webembed/dist/.gitkeep so the Go build stamp stays clean"
    )
if ".gitkeep" not in text:
    raise SystemExit("release.yml must preserve internal/webembed/dist/.gitkeep")

if "appimage-fix-diricon.sh" not in text:
    raise SystemExit("release.yml must fix the AppImage .DirIcon before packaging")

m = re.search(r"^  android:\s*$", text, re.M)
if not m:
    raise SystemExit("release.yml is missing the android job")
rest = text[m.end():]
end = re.search(r"^  [A-Za-z0-9_-]+:\s*$", rest, re.M)
android_block = rest[: end.start()] if end else rest
if "android-keystore-patch.sh" not in android_block:
    raise SystemExit("android job must install the Android Keystore credential helper")
print("desktop coverage, clean Go stamp, AppImage icon fix, Android keystore ok")
PY

echo "release policy test ok"
