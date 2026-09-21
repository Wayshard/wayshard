// Guards the graphical theme/navigation integration that rendered validation
// proved was broken: the app must import the adapted v2 theme layer and mount
// the adapted ThemeProvider, the "More" overflow must go through the shared
// dialog provider, and the narrow shell must expose a drawer toggle.
import { describe, expect, test } from "bun:test"
import { readFileSync } from "node:fs"
import { join } from "node:path"

const src = import.meta.dir
const styles = readFileSync(join(src, "styles.css"), "utf8")
const app = readFileSync(join(src, "app", "App.tsx"), "utf8")

describe("graphical theme integration (R1)", () => {
  test("styles entry imports the adapted v2 theme layer", () => {
    expect(styles).toContain("@wayshard/ui/v2/styles/tailwind.css")
  })
  test("styles entry imports the adapted session-ui component styles", () => {
    expect(styles).toContain("session-ui/styles/index.css")
  })
  test("no compensating hard-coded color-scheme hack", () => {
    expect(styles).not.toMatch(/:root\s*\{[^}]*color-scheme:\s*dark/)
  })
  test("application root mounts the adapted ThemeProvider", () => {
    expect(app).toContain('from "@wayshard/ui/theme/context"')
    expect(app).toMatch(/<ThemeProvider[^>]*defaultTheme="oc-2"/)
  })
})

describe("More overflow control (R2)", () => {
  test("More opens through the shared dialog provider", () => {
    expect(app).toContain("function openMore()")
    expect(app).toMatch(/openMore[\s\S]*dialog\.show\(/)
  })
  test("no inline dialog rendered from a moreOpen signal", () => {
    expect(app).not.toContain("moreOpen")
  })
})

describe("narrow/mobile shell (R3)", () => {
  test("shell exposes a sidebar drawer toggle", () => {
    expect(app).toContain("wh-sidebar-toggle")
    expect(app).toContain("setSidebarOpen")
  })
  test("styles provide an off-canvas drawer below the breakpoint", () => {
    expect(styles).toMatch(/\.wh-sidebar\[data-open="true"\]\s*\{\s*transform/)
    expect(styles).toContain(".wh-backdrop")
  })
})
