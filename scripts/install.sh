#!/bin/sh
# Wayshard installer for Linux and macOS.
#
#   curl -fsSL https://raw.githubusercontent.com/Wayshard/wayshard/main/scripts/install.sh | sh
#
# It resolves the latest stable GitHub release, detects the host OS/architecture,
# downloads the CLI+TUI archive and the matching server binary, verifies them
# against the release SHA256SUMS.txt (and the minisign signature when the
# minisign tool is available), then installs them atomically into a user-owned
# bin directory and adds that directory to the user PATH idempotently.
#
# Environment overrides (all optional):
#   WAYSHARD_VERSION        pin a release tag instead of resolving latest
#   WAYSHARD_INSTALL_DIR    install directory (default: ~/.local/bin)
#   WAYSHARD_NO_MODIFY_PATH set to 1 to skip shell-rc PATH edits
#   WAYSHARD_MINISIGN       auto (default) | require | skip
#   WAYSHARD_MINISIGN_PUB   local path or URL of the minisign public key
#   WAYSHARD_REPO           owner/repo (default: Wayshard/wayshard)
#   WAYSHARD_API_BASE       GitHub API base (default: https://api.github.com)
#   WAYSHARD_DOWNLOAD_BASE  release download base
#   WAYSHARD_RAW_BASE       raw source base (for the committed public key)
#
# The last three exist so tests can point at a local fixture instead of GitHub.
set -eu

REPO="${WAYSHARD_REPO:-Wayshard/wayshard}"
VERSION="${WAYSHARD_VERSION:-}"
API_BASE="${WAYSHARD_API_BASE:-https://api.github.com}"
DOWNLOAD_BASE="${WAYSHARD_DOWNLOAD_BASE:-https://github.com/${REPO}/releases/download}"
RAW_BASE="${WAYSHARD_RAW_BASE:-https://raw.githubusercontent.com/${REPO}/main}"
INSTALL_DIR="${WAYSHARD_INSTALL_DIR:-$HOME/.local/bin}"
MODIFY_PATH="${WAYSHARD_NO_MODIFY_PATH:-0}"
MINISIGN_MODE="${WAYSHARD_MINISIGN:-auto}"
MINISIGN_PUB="${WAYSHARD_MINISIGN_PUB:-${RAW_BASE}/keys/wayshard-release.minisign.pub}"

say() { printf '%s\n' "$*"; }
err() { printf 'wayshard-install: %s\n' "$*" >&2; exit 1; }

work=""
stage=""
cleanup() {
  [ -n "$stage" ] && rm -rf "$stage" 2>/dev/null || true
  [ -n "$work" ] && rm -rf "$work" 2>/dev/null || true
}
trap cleanup EXIT HUP INT TERM

download() {
  # download <url> <dest>
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "$1" -o "$2"
  elif command -v wget >/dev/null 2>&1; then
    wget -q "$1" -O "$2"
  else
    err "neither curl nor wget is available"
  fi
}

fetch_stdout() {
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "$1"
  elif command -v wget >/dev/null 2>&1; then
    wget -qO- "$1"
  else
    err "neither curl nor wget is available"
  fi
}

file_sha256() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$1" | awk '{print $1}'
  elif command -v openssl >/dev/null 2>&1; then
    openssl dgst -sha256 -r "$1" | awk '{print $1}'
  else
    err "no SHA-256 tool found (need sha256sum, shasum, or openssl)"
  fi
}

case "$(uname -s)" in
  Linux) OS=linux ;;
  Darwin) OS=darwin ;;
  *) err "unsupported operating system: $(uname -s) (supported: Linux, macOS)" ;;
esac
case "$(uname -m)" in
  x86_64 | amd64) ARCH=amd64 ;;
  arm64 | aarch64) ARCH=arm64 ;;
  *) err "unsupported architecture: $(uname -m) (supported: x86_64, arm64)" ;;
esac

if [ -z "$VERSION" ]; then
  say "Resolving the latest stable release of $REPO ..."
  latest="$(fetch_stdout "$API_BASE/repos/$REPO/releases/latest" || true)"
  [ -n "$latest" ] || err "could not query $API_BASE/repos/$REPO/releases/latest"
  VERSION="$(printf '%s' "$latest" | sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -n 1)"
  [ -n "$VERSION" ] || err "could not parse the latest release tag from the API response"
fi

archive="wayshard-${VERSION}-${OS}-${ARCH}.tar.gz"
server="wayshard-server-${VERSION}-${OS}-${ARCH}"
sums="SHA256SUMS.txt"
sig="${sums}.minisig"

work="$(mktemp -d "${TMPDIR:-/tmp}/wayshard-install.XXXXXX")"

say "Installing Wayshard ${VERSION} (${OS}-${ARCH}) ..."
download "$DOWNLOAD_BASE/$VERSION/$archive" "$work/$archive" || err "failed to download $archive from $DOWNLOAD_BASE/$VERSION"
download "$DOWNLOAD_BASE/$VERSION/$server" "$work/$server" || err "failed to download $server from $DOWNLOAD_BASE/$VERSION"
download "$DOWNLOAD_BASE/$VERSION/$sums" "$work/$sums" || err "failed to download $sums from $DOWNLOAD_BASE/$VERSION"
chmod +x "$work/$server" 2>/dev/null || true

# minisign (optional but used automatically when the tool is present).
if [ "$MINISIGN_MODE" != "skip" ] && command -v minisign >/dev/null 2>&1; then
  if download "$DOWNLOAD_BASE/$VERSION/$sig" "$work/$sig" 2>/dev/null; then
    case "$MINISIGN_PUB" in
      *://*)
        download "$MINISIGN_PUB" "$work/pubkey" || err "failed to download the minisign public key from $MINISIGN_PUB"
        ;;
      *)
        [ -f "$MINISIGN_PUB" ] || err "minisign public key not found: $MINISIGN_PUB"
        cp "$MINISIGN_PUB" "$work/pubkey"
        ;;
    esac
    minisign -V -p "$work/pubkey" -m "$work/$sums" >/dev/null 2>&1 \
      || err "minisign signature verification failed for $sums"
    say "minisign signature verified"
  else
    [ "$MINISIGN_MODE" = "require" ] && err "missing $sig but WAYSHARD_MINISIGN=require"
    say "note: $sig not published; continuing with SHA-256 verification only"
  fi
elif [ "$MINISIGN_MODE" = "require" ]; then
  err "minisign is required but was not found on PATH"
else
  say "note: minisign not installed; verifying SHA-256 checksums only"
fi

verify_sum() {
  # verify_sum <filename>
  expected="$(awk -v n="$1" '$2 == n { print $1; exit }' "$work/$sums")"
  [ -n "$expected" ] || err "no checksum entry for $1 in $sums"
  actual="$(file_sha256 "$work/$1")"
  [ "$actual" = "$expected" ] || err "checksum mismatch for $1 (expected $expected, got $actual)"
  say "checksum ok: $1"
}
verify_sum "$archive"
verify_sum "$server"

mkdir -p "$INSTALL_DIR"
[ -w "$INSTALL_DIR" ] || err "install directory is not writable: $INSTALL_DIR"

# Stage inside the install directory so the final renames stay on one filesystem.
stage="$(mktemp -d "$INSTALL_DIR/.wayshard-stage.XXXXXX")"
tar -xzf "$work/$archive" -C "$stage"
[ -f "$stage/wayshard" ] || err "archive is missing the wayshard binary"
[ -f "$stage/wayshard-tui" ] || err "archive is missing the wayshard-tui companion"
chmod 0755 "$stage/wayshard" "$stage/wayshard-tui"
chmod 0755 "$work/$server"

# Rename into place. Each mv is atomic and only happens after every download and
# verification above succeeded, so a failed install never leaves a half-written
# file under the final names.
mv -f "$stage/wayshard" "$INSTALL_DIR/wayshard"
mv -f "$stage/wayshard-tui" "$INSTALL_DIR/wayshard-tui"
mv -f "$work/$server" "$INSTALL_DIR/wayshard-server"

on_path=0
case ":${PATH}:" in
  *":$INSTALL_DIR:"*) on_path=1 ;;
esac

if [ "$MODIFY_PATH" = "1" ]; then
  say "skipping PATH modification (WAYSHARD_NO_MODIFY_PATH=1)"
else
  start_marker="# >>> wayshard installer >>>"
  end_marker="# <<< wayshard installer <<<"
  path_line="export PATH=\"$INSTALL_DIR:\$PATH\""
  update_rc() {
    rc="$1"
    if [ -f "$rc" ] && grep -qF "$start_marker" "$rc" 2>/dev/null; then
      return 0
    fi
    {
      printf '\n%s\n' "$start_marker"
      printf '%s\n' "$path_line"
      printf '%s\n' "$end_marker"
    } >>"$rc"
    say "added $INSTALL_DIR to PATH in $rc"
  }
  case "${SHELL:-}" in
    */zsh) preferred="$HOME/.zshrc" ;;
    */bash) preferred="$HOME/.bashrc" ;;
    *) preferred="$HOME/.profile" ;;
  esac
  updated=0
  for rc in "$HOME/.profile" "$HOME/.bashrc" "$HOME/.zshrc"; do
    if [ -f "$rc" ]; then
      update_rc "$rc"
      updated=1
    fi
  done
  if [ "$updated" -eq 0 ]; then
    update_rc "$preferred"
  fi
fi

say ""
say "Wayshard ${VERSION} installed to ${INSTALL_DIR}:"
say "  ${INSTALL_DIR}/wayshard        start the interactive client"
say "  ${INSTALL_DIR}/wayshard-tui    terminal client companion"
say "  ${INSTALL_DIR}/wayshard-server local server daemon"
if [ "$on_path" != "1" ]; then
  say ""
  say "Restart your shell, or run: export PATH=\"${INSTALL_DIR}:\$PATH\""
fi
say "Then run 'wayshard-server' and, in another terminal, 'wayshard'."
