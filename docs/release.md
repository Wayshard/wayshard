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
| Android APK | `wayshard-<tag>-android.apk` | **JKS/PKCS12** upload key, verified before publish |
| Checksums | `SHA256SUMS.txt` + `SHA256SUMS.txt.minisig` | SHA-256 plus minisign |

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

Jobs: `release`, `tui`, `desktop`, `android`, `checksums` in `.github/workflows/release.yml`.

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
The combined checksums job runs only after `release`, `tui`, `desktop`, and
`android` finish, and it fails if any expected bundle is missing before the
manifest is minisigned.

macOS: `codesign -dv --verbose=4 Wayshard.app` should mention `adhoc`.

Windows: `Get-AuthenticodeSignature .\installer.msi` should show a signer
certificate (Status often `UnknownError`/`NotTrusted` for self-signed, never
`NotSigned`).

Android: `apksigner verify --verbose wayshard-vX.Y.Z-android.apk`.

## 8. Still external / manual

- Creating Environment `release` and storing the eight secrets
- Generating and backing up private keys
- Committing the minisign **public** key
- `TYPESAFE_API_KEY` on a running server
- User-installed harnesses and user networking
