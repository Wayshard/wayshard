#!/usr/bin/env bash
# Build the Go CLI and the packaged Wayshard TUI companion, install them into a
# throwaway directory as a user would receive them, and prove the shipped
# interactive path launches the real adapted TUI (not the retired prompt).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

ext=""
case "$(uname -s)" in
  MINGW*|MSYS*|CYGWIN*) ext=".exe" ;;
esac

cd "$ROOT"
go build -o "$WORK/wayshard${ext}" ./cmd/wayshard
cp "clients/tui/wayshard-tui${ext}" "$WORK/wayshard-tui${ext}"
chmod +x "$WORK/wayshard-tui${ext}" 2>/dev/null || true

cd "$WORK"
# A: scriptable subcommands remain non-interactive and never show the old prompt.
"$WORK/wayshard${ext}" help > help.out 2>&1 || true
if grep -aq "wayshard>" help.out; then
  echo "retired interactive prompt reappeared" >&2
  exit 1
fi
if ! grep -aq "interactive client" help.out; then
  echo "help output missing interactive client entry" >&2
  exit 1
fi

# B: no-argument invocation launches the packaged TUI companion.
set +e
timeout 8 "$WORK/wayshard${ext}" > tui.out 2>&1
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
