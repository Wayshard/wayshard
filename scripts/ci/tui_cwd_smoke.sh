#!/usr/bin/env bash
# Regression: the packaged Wayshard TUI must be independent of the caller's
# working directory and module resolution.
#
# A compiled Bun executable autoloads `bunfig.toml` (and `.env`) from the cwd at
# runtime unless the build disables it. That made the shipped companion apply an
# unrelated `preload` and fail with:
#
#   error: preload not found "@opentui/solid/preload"
#
# whenever it was launched from a directory containing such a config — the TUI's
# own workspace (`clients/tui`), or any unrelated project. build.ts now disables
# runtime autoloading; this script proves it.
#
# Usage: tui_cwd_smoke.sh <path-to-wayshard-tui> [expected-version]
set -euo pipefail

TUI_ARG="${1:?usage: tui_cwd_smoke.sh <path-to-wayshard-tui> [expected-version]}"
EXPECT_VERSION="${2:-}"
TUI="$(cd "$(dirname "$TUI_ARG")" && pwd)/$(basename "$TUI_ARG")"
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
WORK="$(mktemp -d "${TMPDIR:-/tmp}/wayshard-tui-cwd.XXXXXX")"
trap 'rm -rf "$WORK"' EXIT

clean="$WORK/clean"
withnm="$WORK/with-node-modules"
withfig="$WORK/with-bunfig"
mkdir -p "$clean" "$withnm/node_modules/@opentui/solid" "$withfig"
cat >"$withnm/node_modules/@opentui/solid/package.json" <<'EOF'
{ "name": "@opentui/solid", "version": "0.0.0-unrelated" }
EOF
cat >"$withfig/bunfig.toml" <<'EOF'
preload = ["@opentui/solid/preload"]
EOF

fail() { echo "tui cwd smoke FAIL: $*" >&2; exit 1; }

run_version() {
  local dir="$1" label="$2" out
  out="$(cd "$dir" && "$TUI" --version 2>&1 || true)"
  echo "[$label] $out"
  grep -q "preload not found" <<<"$out" && fail "preload resolution error from $label"
  grep -q "Wayshard TUI" <<<"$out" || fail "no version reported from $label"
  if [[ -n "$EXPECT_VERSION" ]]; then
    grep -qF "$EXPECT_VERSION" <<<"$out" || fail "version mismatch from $label: $out"
  fi
}

run_version "$ROOT" "repo-root"
run_version "$ROOT/clients" "clients"
run_version "$ROOT/clients/tui" "clients-tui-workspace"
run_version "$clean" "clean-temp"
run_version "$withnm" "temp-unrelated-node-modules"
run_version "$withfig" "temp-bunfig-preload"

# Full render from the directory that used to break module loading.
if command -v timeout >/dev/null 2>&1; then
  (cd "$withfig" && timeout 10 "$TUI" >"$WORK/render.out" 2>&1) || true
  grep -q "preload not found" "$WORK/render.out" && fail "render failed from a bunfig-preload directory"
  grep -q "Wayshard" "$WORK/render.out" || fail "TUI did not render from a bunfig-preload directory"
fi

echo "tui cwd smoke ok"
