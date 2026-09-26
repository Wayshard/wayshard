# Official Wayshard releases

Official binaries are produced only by GitHub Actions on `Wayshard/wayshard`
from tags matching `v*` (example `v0.1.0`). Workstation uploads are not official.

**Signing policy:** every private key is generated and held by the Wayshard
maintainer. Releases do **not** require a paid developer program, commercial CA,
Apple Developer account, notarization, Microsoft/Azure signing, Google Play, or
any other external signing account.

Wayshard artifacts are **cryptographically signed by Wayshard**. They are **not**
trusted by Apple or Microsoft platform PKI. Users should expect OS warnings
until they explicitly trust the publisher.

There is no Tauri auto-updater (not required by the canonicals).

## What a tag builds

| Component | Artifact name pattern | How it is signed |
|---|---|---|
| Server | `wayshard-server-<tag>-<os>-<arch>[.exe]` | Checksums + minisign on `SHA256SUMS.txt` |
| CLI (raw) | `wayshard-<tag>-<os>-<arch>[.exe]` | Same |
| **CLI+TUI bundle** | `wayshard-<tag>-<os>-<arch>.tar.gz` (`.zip` on Windows) | Same |
| TUI (raw companion) | `wayshard-tui-<tag>-<os>-<arch>[.exe]` | Same |
| Embedded Web | inside Server | Same |
| CycloneDX SBOM | `wayshard-<tag>-sbom-go.cdx.json` | Same |
| Go buildinfo | `wayshard-<tag>-buildinfo-server-linux-amd64.txt` | Same (not an SBOM) |
| Notices | `LICENSE`, `NOTICE`, `THIRD_PARTY_NOTICES.md` | Same |
| Desktop Linux | `wayshard-desktop-<tag>-linux-<arch>.{AppImage,deb}` | Checksums + minisign only |
| Desktop macOS | `wayshard-desktop-<tag>-macos-<arch>.dmg` (`aarch64`, `x86_64`) | **Ad-hoc** `codesign -s -` (identity `-`). No Developer ID |
| Desktop Windows | `wayshard-desktop-<tag>-windows-x64.msi` and `-setup.exe` | **Self-signed Authenticode** from maintainer PFX |
| Android APK | `wayshard-<tag>-android.apk` | **JKS/PKCS12** upload key; APK Signature Scheme **v2 + v3** (v1/JAR is not used at minSdk 26) |
| Checksums | `SHA256SUMS.txt` + `SHA256SUMS.txt.minisig` | SHA-256 plus minisign |
| Per-component checksums | `SHA256SUMS-go.txt`, `SHA256SUMS-desktop-linux.txt`, `SHA256SUMS-desktop-macos.txt`, `SHA256SUMS-desktop-windows.txt`, `SHA256SUMS-android.txt` | SHA-256 only |

### Per-component checksums

Each release family also publishes its own SHA-256 manifest. Because the two
macOS desktop DMGs are built on separate matrix runners, they must **not** each
write `SHA256SUMS-desktop-macos.txt` (that raced on the shared asset name and
could publish only one architecture). A single `desktop-macos-checksums` job
runs after the whole desktop matrix, downloads both DMGs, and writes
`SHA256SUMS-desktop-macos.txt` with exactly one `aarch64` and one `x86_64` row
(`scripts/release/desktop-macos-checksums.sh`). The combined `checksums` job
waits for it. The Linux and Windows desktop legs are single-runner, so they
generate their own manifest in-matrix.

### Android Keystore R8 gate

`dev.wayshard.app.WayshardKeystore` is called only from Rust over JNI, so the
minified release build (Tauri enables `isMinifyEnabled = true`) would strip or
rename it without keep rules. `clients/desktop/android/WayshardKeystore.pro`
is installed by `android-keystore-patch.sh`, and the `android` job runs
`scripts/release/android-keystore-verify.py` against the signed APK's
`classes*.dex`, failing the release if the class or its
`encrypt`/`decrypt`/`deleteKey` methods are absent.

### Application icons

`assets/branding/wayshard.png` is the canonical mark. Desktop PNG/ICNS/ICO and
Web favicon/touch/PWA derivatives are committed. Because the Android Gradle
project is generated fresh in CI, the `android` job runs
`scripts/release/android-icons.sh` after `tauri android init` to regenerate the
launcher, adaptive foreground, monochrome and background resources from
`clients/desktop/src-tauri/app-icon.json`. The manifest points at the canonical
mark for desktop/browser use and at the safe-area-padded
`assets/branding/wayshard-android-fg.png` for the adaptive foreground and
monochrome mask (`bg_color` `#0e0f12`). `@tauri-apps/cli` is pinned to the same
major.minor as the locked `tauri` Rust crate. `icon_policy_test.sh` guards the
manifest, the committed derivative sizes, the safe-area padding, and the
CLI/crate alignment.

### Windows installer icon

`assets/branding/wayshard.ico` (the canonical mark at Windows sizes, 16-256px,
32-bit with alpha) is set as `bundle.windows.nsis.installerIcon` in
`clients/desktop/src-tauri/tauri.conf.json`. Without it NSIS falls back to its
default installer icon. NSIS compiles the ICO frames into the installer's PE
resources byte-for-byte, so the Windows desktop job runs
`scripts/release/verify-windows-installer-icon.py` against the built
`wayshard-desktop-<tag>-windows-x64-setup.exe` and fails if any Wayshard frame
is absent. `icon_policy_test.sh` validates the ICO frames and derivation, and
`release_policy_test.sh` fails if `installerIcon` is unset.

### Web social card

`clients/web/public/social-share.png` (1200x630, the canonical mark on the
`#0e0f12` deep background with the Wayshard wordmark) backs the `og:image` /
`twitter:image` metadata in `clients/web/index.html`. `icon_policy.py` checks
its size, background, brand colours and wiring, and the release `release` job
fails if the embedded Web build does not contain it, so the published server
always serves `/social-share.png`.

### Linux binaries

Server and CLI builds are CGO-free (`CGO_ENABLED=0`), so the Linux artifacts are
**statically linked** and run on any distro (musl/Alpine, minimal containers)
without glibc coupling. The `release` job asserts the Linux binaries are static;
`linux_static_test.sh` guards the amd64 host build in `release-scripts-test`.

### Tauri CLI alignment

`@tauri-apps/cli` is pinned to the same major.minor as the locked `tauri` Rust
crate (`Cargo.lock`), so the desktop/mobile template and the icon manifest are
generated by a matching toolchain. `icon_policy.py` fails if the npm CLI and the
crate drift apart.

### Android signing

The upload-key signing config enables APK Signature Scheme v2 and v3 only.
minSdk is 26, so the legacy v1/JAR scheme is never used and `apksigner verify`
reports it `false` at that minSdk; the config does not emit or claim it.
`package-android.sh` asserts v2 and v3 are present on the packaged APK.

### CLI+TUI bundle layout

The `tui` matrix builds a native `wayshard` CLI and a self-contained
`wayshard-tui` companion on each platform runner and packages them with
`scripts/release/package-cli-tui.sh`. Inside every archive the binaries use the
canonical runtime names the launcher expects, so a user extracts and runs with
no rename and no `WAYSHARD_TUI` override:

```text
wayshard-vX-linux-amd64.tar.gz
├── wayshard
├── wayshard-tui
├── LICENSE
├── NOTICE
└── THIRD_PARTY_NOTICES.md
```

Windows uses the same layout in `wayshard-vX-windows-amd64.zip` with `.exe`
suffixes. The raw CLI/TUI binaries remain published for advanced users, but the
raw `wayshard` binary alone is not a functional interactive client.

## Reproducible builds

Release builds are reproducible and self-consistent:

- The build date stamped into every Go binary (`internal/version.Date`) is
  derived from the tagged commit, never from wall-clock time, and
  `SOURCE_DATE_EPOCH` is pinned to the same commit in the `Makefile`.
- Go binaries are built with `-trimpath` and one shared `LDFLAGS` definition, so
  rebuilding the same commit yields byte-identical output.
- The CLI is built exactly once, in the `release` job. The CLI+TUI bundles fetch
  that published artifact instead of rebuilding it, so the standalone CLI and
  the CLI inside every archive are byte-identical on every platform.
- One verification script covers both the tagged and untagged paths:
  `scripts/release/verify-reproducible.sh --assets <tag> <commit> <dir>` runs in
  the tagged release `reproducible` job, and `--build <version> <commit> <dir>`
  builds the same artifact layout locally and runs the *identical* comparison in
  normal CI (the `reproducible` CI job) with no tag and no publishing. Both prove
  standalone `wayshard` equals the bundled `wayshard` per platform, and that a
  fresh same-commit rebuild of the host CLI equals the standalone binary.

## TUI version identity

`wayshard-tui --version` prints the release version and commit. The values are
compiled into the executable by `clients/tui/build.ts` (`WAYSHARD_TUI_VERSION`
and `WAYSHARD_TUI_COMMIT`); a source build reports the `0.0.0-dev` fallback. The
`tui` CI job builds with a fixture version and asserts the output, and the
release `tui` job stamps the real tag and commit.

The packaged companion is also independent of the caller's working directory and
module resolution. The build disables runtime autoloading of
`bunfig.toml`/`.env`/`tsconfig.json`/`package.json`, so a directory that happens
to contain a `preload` (for example the TUI workspace itself) can no longer make
the shipped binary fail with `preload not found`. `scripts/ci/tui_cwd_smoke.sh`
(part of the shipped `tui` smoke) proves this from the repo root, `clients/`,
`clients/tui`, a clean temp dir, a temp dir with unrelated `node_modules`, and a
temp dir with a `bunfig.toml` preload.

## Installing a release

`scripts/install.sh` (Linux/macOS) and `scripts/install.ps1` (Windows) resolve
the latest stable release, detect the host OS/architecture, verify the downloads
against `SHA256SUMS.txt` (and the minisign signature when the tool is present),
install atomically into a user-owned bin directory (`~/.local/bin` on
Linux/macOS, `%LOCALAPPDATA%\Wayshard\bin` on Windows), and add that directory
to the user `PATH` idempotently. Both honor `WAYSHARD_VERSION`,
`WAYSHARD_INSTALL_DIR`, `WAYSHARD_NO_MODIFY_PATH`, `WAYSHARD_MINISIGN`, and the
`WAYSHARD_*_BASE` overrides used by tests.

Their fixture-backed tests (`scripts/release/install_sh_test.sh` and
`scripts/release/install_ps1_test.ps1`) exercise latest-release resolution,
checksum verification, signature verification, atomic install, clean re-install,
idempotent PATH updates, and failure handling without contacting the live
release. The `installers` CI job runs them on Linux, macOS, and Windows.

## SBOM license metadata

The CycloneDX SBOM asserts a license only when cyclonedx-gomod detects one from
a dependency's real license file. If any component's license cannot be detected
the release fails instead of fabricating a license.

## CI runner and action hygiene

The CI and release workflows pin Linux jobs to `ubuntu-24.04` and use action
versions that run on the current Node runtime (no Node 20-era majors), so runner
image drift and deprecated action runtimes are handled explicitly.

## Trust vs cryptography

| | Wayshard signed | Apple/Microsoft/Google trusted |
|---|---|---|
| Android APK | yes (maintainer JKS) | no Play App Signing / no Play account |
| macOS app/dmg | yes (ad-hoc) | no; Gatekeeper will warn |
| Windows MSI/EXE | yes (self-signed) | no; SmartScreen / unknown publisher |
| Linux packages | checksums + minisign | n/a |

## CI security

- `.github/workflows/ci.yml`: `contents: read`, **no secrets**.
- Fork PRs never receive Environment `release` secrets.
- No `TYPESAFE_API_KEY` in Actions.
- Private material is Environment **`release`** only.
- Publishing uses automatic `GITHUB_TOKEN` (`contents: write`). Do not create a PAT.

## 1. Create GitHub Environment `release`

Exact name: **`release`**.

Jobs: `release`, `tui`, `reproducible`, `desktop`, `desktop-macos-checksums`, `android`, `checksums` in `.github/workflows/release.yml`.

Recommended: required reviewers; restrict to tags `v*`.

## 2. Secrets actually consumed

All of these belong on Environment **`release`**. PR CI does not read them.

### Android (required for the `android` job)

| Name | Value |
|---|---|
| `WAYSHARD_ANDROID_KEYSTORE_BASE64` | Base64 of `upload-keystore.jks` |
| `WAYSHARD_ANDROID_KEYSTORE_PASSWORD` | keystore password |
| `WAYSHARD_ANDROID_KEY_ALIAS` | alias (example `upload`) |
| `WAYSHARD_ANDROID_KEY_PASSWORD` | key password (often same as store) |

Missing any → Android job fails; unsigned APK is not published.

### Windows (required for the Windows `desktop` matrix leg)

| Name | Value |
|---|---|
| `WAYSHARD_WINDOWS_PFX_BASE64` | Base64 of password-protected PFX/PKCS12 |
| `WAYSHARD_WINDOWS_PFX_PASSWORD` | PFX password |

Missing any → Windows desktop job fails. No unsigned installer is labeled signed.

### Release integrity / minisign (required for the `checksums` job)

| Name | Value |
|---|---|
| `WAYSHARD_RELEASE_MINISIGN_KEY_BASE64` | Base64 of `wayshard-release.minisign.sec` |
| `WAYSHARD_RELEASE_MINISIGN_PASSWORD` | passphrase protecting that secret key |

Also **commit** the matching public key as `keys/wayshard-release.minisign.pub`.
Missing secrets or missing public key → checksums job fails.

`GITHUB_TOKEN` is automatic. No macOS secrets. No Tauri updater secrets.

## 3. Generate every key locally

You can run the helper (prompts as needed):

```sh
umask 077
bash scripts/release/generate-signing-material.sh <secure-signing-directory>
```

Or run the exact commands below in a maintainer-controlled private directory
outside the repository (`umask 077`). Do not generate keys under the Wayshard
source tree.
Private keys are irreplaceable: losing the Android keystore breaks APK update
identity; losing the Windows PFX changes publisher identity; losing the
minisign secret requires publishing a new public key.

### Android JKS/PKCS12 (no Google Play)

```sh
keytool -genkeypair -v \
  -storetype PKCS12 \
  -keystore upload-keystore.jks \
  -keyalg RSA -keysize 2048 -validity 10000 \
  -alias upload \
  -storepass 'YOUR_ANDROID_STORE_PASSWORD' \
  -keypass 'YOUR_ANDROID_KEY_PASSWORD' \
  -dname "CN=Wayshard, O=Wayshard, C=US"

base64 -w0 upload-keystore.jks > upload-keystore.jks.b64   # GNU
base64 -i upload-keystore.jks > upload-keystore.jks.b64    # macOS
```

### Windows self-signed code-signing PFX (no commercial CA)

```sh
openssl req -x509 -newkey rsa:4096 -sha256 -days 3650 -nodes \
  -subj "/CN=Wayshard/" \
  -addext "extendedKeyUsage=codeSigning" \
  -keyout windows-codesign.key \
  -out windows-codesign.crt

openssl pkcs12 -export \
  -inkey windows-codesign.key \
  -in windows-codesign.crt \
  -name Wayshard \
  -passout pass:'YOUR_WINDOWS_PFX_PASSWORD' \
  -out windows-codesign.pfx

base64 -w0 windows-codesign.pfx > windows-codesign.pfx.b64
# macOS: base64 -i windows-codesign.pfx > windows-codesign.pfx.b64
```

Keep `windows-codesign.key` only offline. GitHub stores the password-protected PFX.

### minisign keypair (no third-party service)

```sh
minisign -G -p wayshard-release.minisign.pub -s wayshard-release.minisign.sec
base64 -w0 wayshard-release.minisign.sec > wayshard-release.minisign.sec.b64
# macOS: base64 -i wayshard-release.minisign.sec > wayshard-release.minisign.sec.b64
cp wayshard-release.minisign.pub keys/wayshard-release.minisign.pub
git add keys/wayshard-release.minisign.pub
```

`minisign -G` prompts for a passphrase; that passphrase is
`WAYSHARD_RELEASE_MINISIGN_PASSWORD`. Install minisign from
https://jedisct1.github.io/minisign/ (package or GitHub release binary).
Do not commit `*.sec`.

### Encode reminder

GitHub secrets are the **base64 files** (or the password strings), not the raw
binary paths.

### Back up every irreplaceable private key

```sh
umask 077
SIGNING_DIR="<secure-signing-directory>"
mkdir -p "$SIGNING_DIR"
cp -a upload-keystore.jks upload-keystore.jks.b64 \
  windows-codesign.key windows-codesign.crt windows-codesign.pfx windows-codesign.pfx.b64 \
  wayshard-release.minisign.sec wayshard-release.minisign.sec.b64 wayshard-release.minisign.pub \
  "$SIGNING_DIR/"
sha256sum "$SIGNING_DIR"/*
tar -czf wayshard-signing-material.tar.gz -C "$(dirname "$SIGNING_DIR")" "$(basename "$SIGNING_DIR")"
sha256sum wayshard-signing-material.tar.gz
```

Copy `wayshard-signing-material.tar.gz` to encrypted offline media. Do not put
private keys in git, chat, or an unencrypted cloud drive. After backup, shred
working copies outside the secure signing directory if you generated them in a
scratch directory.

## 4. GitHub UI

1. `https://github.com/Wayshard/wayshard` → **Settings → Environments → New environment** → `release`.
2. Restrict to `v*` tags; add reviewers.
3. Add the eight secrets listed above.
4. Commit `keys/wayshard-release.minisign.pub`.
5. **Actions → General**: allow the third-party actions used by CI/Release; allow workflows to request `contents: write`.
6. Do not add `TYPESAFE_API_KEY` to Actions.
7. `git tag v0.1.0 && git push origin v0.1.0`.

## 5. `gh` commands

```sh
gh api -X PUT repos/Wayshard/wayshard/environments/release

gh secret set WAYSHARD_ANDROID_KEYSTORE_BASE64 --env release -R Wayshard/wayshard < upload-keystore.jks.b64
gh secret set WAYSHARD_ANDROID_KEYSTORE_PASSWORD --env release -R Wayshard/wayshard
gh secret set WAYSHARD_ANDROID_KEY_ALIAS --env release -R Wayshard/wayshard -b upload
gh secret set WAYSHARD_ANDROID_KEY_PASSWORD --env release -R Wayshard/wayshard

gh secret set WAYSHARD_WINDOWS_PFX_BASE64 --env release -R Wayshard/wayshard < windows-codesign.pfx.b64
gh secret set WAYSHARD_WINDOWS_PFX_PASSWORD --env release -R Wayshard/wayshard

gh secret set WAYSHARD_RELEASE_MINISIGN_KEY_BASE64 --env release -R Wayshard/wayshard < wayshard-release.minisign.sec.b64
gh secret set WAYSHARD_RELEASE_MINISIGN_PASSWORD --env release -R Wayshard/wayshard

gh secret list --env release -R Wayshard/wayshard
```

## 6. Expected OS warnings (not bugs)

**macOS Gatekeeper:** ad-hoc signed apps from the internet typically show
“cannot be opened because the developer cannot be verified” / Privacy &
Security “Open Anyway”. That is expected. There is no Apple Developer ID and
no notarization.

**Windows SmartScreen / unknown publisher:** self-signed Authenticode is
cryptographically valid as Wayshard’s signature but is **not** in Microsoft’s
trusted CA store. SmartScreen and “Unknown publisher” prompts are expected
until the user trusts the certificate.

**Android:** sideloading a maintainer-signed APK does not go through Play
Protect as a Play-distributed app. Users install the APK and trust the
Wayshard signing key. No Play account is used.

## 7. Verify a release locally

```sh
minisign -V -p keys/wayshard-release.minisign.pub -m SHA256SUMS.txt
sha256sum -c SHA256SUMS.txt

# CLI+TUI bundle: verify then extract and run in place (no rename/override).
tar -xzf wayshard-vX-linux-amd64.tar.gz -C wayshard
./wayshard/wayshard help
./wayshard/wayshard
```

`SHA256SUMS.txt` covers every published asset, including each CLI+TUI bundle.
The combined checksums job runs only after `release`, `tui`, `desktop`,
`desktop-macos-checksums`, and `android` finish, and it fails if any expected
bundle is missing before the manifest is minisigned.

macOS: `codesign -dv --verbose=4 Wayshard.app` should mention `adhoc`. Verify
that `SHA256SUMS-desktop-macos.txt` lists both `-macos-aarch64.dmg` and
`-macos-x86_64.dmg` exactly once.

Windows: `Get-AuthenticodeSignature .\installer.msi` should show a signer
certificate (Status often `UnknownError`/`NotTrusted` for self-signed, never
`NotSigned`).

Android: `apksigner verify --verbose wayshard-vX.Y.Z-android.apk`. The release
job also runs `scripts/release/android-keystore-verify.py` on the APK, which
fails if R8 stripped the JNI-invoked `WayshardKeystore` helper.

## 8. Still external / manual

- Creating Environment `release` and storing the eight secrets
- Generating and backing up private keys
- Committing the minisign **public** key
- `TYPESAFE_API_KEY` on a running server
- User-installed harnesses and user networking
