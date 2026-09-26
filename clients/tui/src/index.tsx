#!/usr/bin/env bun
// Wayshard TUI entry point. Interactive terminal client adapted from the
// imported OpenCode 2 terminal UI foundation.
//
// The Solid JSX transform is supplied by `bunfig.toml` `preload` for `bun run`
// and by `createSolidTransformPlugin()` in build.ts for the compiled release.
//
// Do NOT add a static `import "@opentui/solid/preload"` here. A compiled Bun
// executable resolves such imports at runtime, so the packaged companion picked
// up whatever `@opentui/solid` happened to be resolvable from the caller's
// working directory and failed with `preload not found` when that copy did not
// match the bundled one. Keeping the companion free of runtime module
// resolution makes it cwd-independent; see scripts/ci/tui_cwd_smoke.sh.
import { versionLine } from "./version"

// Non-interactive introspection must work without initializing the renderer, so
// `--version` / `--help` are answered before the application module is loaded.
// The companion is normally launched with no arguments, so scanning argv for
// the flag/subcommand is safe even though the executable path is included.
const wantsVersion =
  process.argv.includes("--version") || process.argv.includes("-v") || process.argv.includes("version")
const wantsHelp =
  process.argv.includes("--help") || process.argv.includes("-h") || process.argv.includes("help")

if (wantsVersion) {
  console.log(versionLine())
  process.exit(0)
}
if (wantsHelp) {
  console.log(`wayshard-tui — Wayshard interactive terminal client

  wayshard-tui --version   print the release version and commit
  wayshard-tui --help      print this help

The interactive client is normally launched by the wayshard CLI, which supplies
the server URL and device credential (WAYSHARD_URL, WAYSHARD_TOKEN).`)
  process.exit(0)
}

const { run } = await import("./app")
await run()
