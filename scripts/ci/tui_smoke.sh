#!/usr/bin/env bash
# Prove the shipped end-user path from the REAL release packaging script:
# build the Go CLI and the packaged Wayshard TUI companion, produce the same
# archive the release workflow publishes, extract it into a fresh directory, and
# run it exactly as a user would (no rename, no WAYSHARD_TUI, no PATH lookup).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

ext=""
case "$(uname -s)" in
  MINGW*|MSYS*|CYGWIN*) ext=".exe" ;;
esac
os="$(go env GOOS)"
arch="$(go env GOARCH)"

stage="$WORK/stage"
out="$WORK/out"
extract="$WORK/extract"
mkdir -p "$stage" "$out" "$extract"

cd "$ROOT"
go build -o "$stage/wayshard${ext}" ./cmd/wayshard
cp "clients/tui/wayshard-tui${ext}" "$stage/wayshard-tui${ext}"
chmod +x "$stage/wayshard-tui${ext}" 2>/dev/null || true

bash scripts/release/package-cli-tui.sh v0.0.0-smoke "$os" "$arch" "$stage" "$out"
shopt -s nullglob
archives=("$out"/wayshard-v0.0.0-smoke-*)
shopt -u nullglob
if [[ ${#archives[@]} -ne 1 ]]; then
  echo "expected exactly one smoke archive, found ${#archives[@]}" >&2
  exit 1
fi
archive="${archives[0]}"

case "$archive" in
  *.zip) unzip -q "$archive" -d "$extract" ;;
  *.tar.gz) tar -xzf "$archive" -C "$extract" ;;
  *) echo "unexpected archive $archive" >&2; exit 1 ;;
esac

cd "$extract"
unset WAYSHARD_TUI || true
if PATH="$WORK/empty-path" command -v "wayshard-tui${ext}" >/dev/null 2>&1; then
  echo "companion unexpectedly resolvable through PATH" >&2
  exit 1
fi

# A: scriptable subcommands remain non-interactive and never show the old prompt.
"$extract/wayshard${ext}" help > help.out 2>&1 || true
if grep -aq "wayshard>" help.out; then
  echo "retired interactive prompt reappeared" >&2
  exit 1
fi
if ! grep -aq "interactive client" help.out; then
  echo "help output missing interactive client entry" >&2
  exit 1
fi

# B: no-argument invocation launches the bundled TUI companion.
set +e
if command -v timeout >/dev/null 2>&1; then
  timeout 8 "$extract/wayshard${ext}" > tui.out 2>&1
else
  "$extract/wayshard${ext}" > tui.out 2>&1
fi
set -e
if grep -aq "wayshard>" tui.out; then
  echo "retired interactive prompt reappeared on no-argument launch" >&2
  exit 1
fi
if grep -aq "Wayshard TUI not found" tui.out; then
  echo "TUI companion was not located" >&2
  exit 1
fi
if ! grep -aq "Wayshard" tui.out; then
  echo "packaged TUI did not render" >&2
  exit 1
fi
echo "tui smoke ok"
