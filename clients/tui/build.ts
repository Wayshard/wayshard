// Wayshard TUI production build.
//
// Uses the OpenTUI Solid Bun plugin so the compiled executable applies the
// Solid JSX transform (the same transform the dev preload installs). Produces a
// self-contained executable so end users do not need Bun or a repository
// checkout.
//
// TUI_TARGET selects a cross target (for example bun-linux-arm64). Cross targets
// require the matching @opentui/core-<platform> package, which only installs on
// a matching native runner; the release workflow builds each platform on its own
// runner.
import { createSolidTransformPlugin } from "@opentui/solid/bun-plugin"

const outfile = process.env.TUI_OUTFILE ?? "wayshard-tui"
const target = process.env.TUI_TARGET
// Release identity is compiled into the executable so `wayshard-tui --version`
// reports the release tag/commit without reading any external file.
const version = process.env.WAYSHARD_TUI_VERSION ?? "0.0.0-dev"
const commit = process.env.WAYSHARD_TUI_COMMIT ?? "unknown"

// A packaged companion must be independent of the caller's working directory.
// A compiled Bun executable otherwise autoloads bunfig.toml (and .env) from the
// cwd at runtime, so launching the TUI from a directory containing a bunfig
// `preload` — for example the TUI's own workspace source tree, or any unrelated
// project — failed with `preload not found "@opentui/solid/preload"`. Disable
// every runtime autoload so the shipped binary carries all of its own code.
// Regression coverage: scripts/ci/tui_cwd_smoke.sh.
const compile: Bun.Build.CompileBuildOptions = {
  outfile,
  autoloadBunfig: false,
  autoloadDotenv: false,
  autoloadTsconfig: false,
  autoloadPackageJson: false,
}
if (target) compile.target = target

const result = await Bun.build({
  entrypoints: ["./src/index.tsx"],
  target: "bun",
  plugins: [createSolidTransformPlugin()],
  define: {
    __WAYSHARD_TUI_VERSION__: JSON.stringify(version),
    __WAYSHARD_TUI_COMMIT__: JSON.stringify(commit),
  },
  compile,
})
if (!result.success) {
  for (const log of result.logs) console.error(log)
  process.exit(1)
}
console.log(`built ${outfile}${target ? ` (${target})` : ""} version=${version} commit=${commit}`)
