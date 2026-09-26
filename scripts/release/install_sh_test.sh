#!/usr/bin/env bash
# Fixture-backed test for scripts/install.sh.
#
# Builds a fake release (CLI+TUI archive, server binary, SHA256SUMS.txt, an
# optional minisign signature, and a GitHub "latest release" API document) under
# a temp directory, then runs the real installer against it with file:// bases.
# It never contacts the live GitHub release.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
WORK="$(mktemp -d "${TMPDIR:-/tmp}/wayshard-install-test.XXXXXX")"
trap 'rm -rf "$WORK"' EXIT

VERSION="v9.9.9"
PY="$(command -v python3 || command -v python || true)"
[ -n "$PY" ] || { echo "python3 is required for this test" >&2; exit 1; }

case "$(uname -s)" in
  Linux) os=linux ;;
  Darwin) os=darwin ;;
  *) echo "unsupported test OS: $(uname -s)" >&2; exit 1 ;;
esac
case "$(uname -m)" in
  x86_64 | amd64) arch=amd64 ;;
  arm64 | aarch64) arch=arm64 ;;
  *) echo "unsupported test arch: $(uname -m)" >&2; exit 1 ;;
esac

fail() { echo "FAIL: $*" >&2; exit 1; }

# make_release <dest> <tag>
make_release() {
  local dest="$1" tag="$2"
  local download="$dest/download/$tag"
  local api="$dest/api/repos/Wayshard/wayshard/releases"
  local stage="$dest/stage"
  mkdir -p "$download" "$api" "$stage"

  cat >"$stage/wayshard" <<'EOF'
#!/bin/sh
echo "Wayshard CLI fixture"
EOF
  cat >"$stage/wayshard-tui" <<'EOF'
#!/bin/sh
echo "Wayshard TUI fixture"
EOF
  chmod 0755 "$stage/wayshard" "$stage/wayshard-tui"
  cp "$ROOT/LICENSE" "$ROOT/NOTICE" "$ROOT/THIRD_PARTY_NOTICES.md" "$stage/"

  bash "$ROOT/scripts/release/package-cli-tui.sh" "$tag" "$os" "$arch" "$stage" "$download" >/dev/null

  cat >"$download/wayshard-server-$tag-$os-$arch" <<'EOF'
#!/bin/sh
echo "Wayshard server fixture"
EOF
  chmod 0755 "$download/wayshard-server-$tag-$os-$arch"

  "$PY" "$ROOT/scripts/release/checksums.py" --out "$download/SHA256SUMS.txt" "$download"/* >/dev/null

  printf '{"tag_name":"%s"}\n' "$tag" >"$api/latest"

  if command -v minisign >/dev/null 2>&1 && minisign -G -W -p "$dest/pub" -s "$dest/sec" >/dev/null 2>&1; then
    if minisign -S -s "$dest/sec" -m "$download/SHA256SUMS.txt" -t "$tag" >/dev/null 2>&1; then
      echo yes
      return 0
    fi
  fi
  echo no
}

# run_installer <dest> <bindir> <homedir> [extra env via caller]
run_installer() {
  local dest="$1" bindir="$2" homedir="$3"
  WAYSHARD_INSTALL_DIR="$bindir" \
    WAYSHARD_API_BASE="file://$dest/api" \
    WAYSHARD_DOWNLOAD_BASE="file://$dest/download" \
    WAYSHARD_MINISIGN_PUB="$dest/pub" \
    HOME="$homedir" \
    SHELL="/bin/sh" \
    sh "$ROOT/scripts/install.sh"
}

echo "==> happy path (latest resolution, checksums${minisign_note:-})"
dest="$WORK/rel"
have_minisign="$(make_release "$dest" "$VERSION")"
[ "$have_minisign" = "yes" ] && echo "    fixture minisign key generated" || echo "    minisign unavailable; checksum-only path"

bindir="$WORK/bin"
homedir="$WORK/home"
mkdir -p "$homedir"

if ! out="$(run_installer "$dest" "$bindir" "$homedir" 2>&1)"; then
  echo "$out" >&2
  fail "installer exited non-zero"
fi
echo "$out" | sed 's/^/    /'

[ -x "$bindir/wayshard" ] || fail "wayshard not installed/executable"
[ -x "$bindir/wayshard-tui" ] || fail "wayshard-tui not installed/executable"
[ -x "$bindir/wayshard-server" ] || fail "wayshard-server not installed/executable"
"$bindir/wayshard" | grep -q "Wayshard CLI fixture" || fail "installed CLI does not run"
"$bindir/wayshard-server" | grep -q "Wayshard server fixture" || fail "installed server does not run"

# PATH block is added exactly once and survives a re-run (clean upgrade).
[ -f "$homedir/.profile" ] || fail "installer did not update ~/.profile"
[ "$(grep -c '>>> wayshard installer >>>' "$homedir/.profile")" -eq 1 ] || fail "PATH block not written exactly once"

echo "==> re-run (clean upgrade is idempotent)"
if ! out2="$(run_installer "$dest" "$bindir" "$homedir" 2>&1)"; then
  echo "$out2" >&2
  fail "second installer run exited non-zero"
fi
[ "$(grep -c '>>> wayshard installer >>>' "$homedir/.profile")" -eq 1 ] || fail "PATH block duplicated on re-run"
[ -x "$bindir/wayshard" ] || fail "wayshard missing after re-run"

echo "==> checksum failure is rejected and installs nothing"
bad="$WORK/bad"
make_release "$bad" "$VERSION" >/dev/null
printf 'corrupt' >>"$bad/download/$VERSION/wayshard-$VERSION-$os-$arch.tar.gz"
badbin="$WORK/badbin"
badhome="$WORK/badhome"
mkdir -p "$badhome"
if run_installer "$bad" "$badbin" "$badhome" >/dev/null 2>&1; then
  fail "installer accepted a corrupted archive"
fi
[ ! -e "$badbin/wayshard" ] || fail "corrupted install wrote wayshard"

echo "==> unsupported architecture is rejected cleanly"
if WAYSHARD_TEST_ROOT="$ROOT" WAYSHARD_TEST_WORK="$WORK" bash -c '
  uname() { case "$1" in -s) echo Linux ;; -m) echo MIPS ;; *) command uname "$@" ;; esac; }
  export -f uname
  export WAYSHARD_INSTALL_DIR="$WAYSHARD_TEST_WORK/badarch"
  exec bash "$WAYSHARD_TEST_ROOT/scripts/install.sh"
' >/dev/null 2>&1; then
  fail "installer accepted an unsupported architecture"
fi
[ ! -e "$WORK/badarch/wayshard" ] || fail "unsupported-arch run installed a binary"

echo "install_sh_test ok"
