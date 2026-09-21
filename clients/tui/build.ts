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
const result = await Bun.build({
  entrypoints: ["./src/index.tsx"],
  target: "bun",
  plugins: [createSolidTransformPlugin()],
  compile: target ? { outfile, target } : { outfile },
})
if (!result.success) {
  for (const log of result.logs) console.error(log)
  process.exit(1)
}
console.log(`built ${outfile}${target ? ` (${target})` : ""}`)
