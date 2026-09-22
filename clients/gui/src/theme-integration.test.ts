// Guards the graphical theme/navigation integration that rendered validation
// proved was broken, and the adapted application composition: the app must
// import the adapted v2 theme layer and mount the adapted ThemeProvider, the
// "More" overflow must go through the shared dialog provider, and the narrow
// shell must expose a drawer toggle.
import { describe, expect, test } from "bun:test"
import { readFileSync } from "node:fs"
import { join } from "node:path"

const src = import.meta.dir
const styles = readFileSync(join(src, "styles.css"), "utf8")
const app = readFileSync(join(src, "app", "app.tsx"), "utf8")
const page = readFileSync(join(src, "app", "session-page.tsx"), "utf8")
const layout = readFileSync(join(src, "app", "pages", "layout.tsx"), "utf8")

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
    expect(page).toContain("function openMore()")
    expect(page).toMatch(/openMore[\s\S]*dialog\.show\(/)
  })
  test("no inline dialog rendered from a moreOpen signal", () => {
    expect(page).not.toContain("moreOpen")
  })
})

describe("narrow/mobile shell (R3)", () => {
  test("layout exposes a navigation toggle", () => {
    expect(layout).toContain("Toggle navigation")
    expect(layout).toContain("mobileOpen")
  })
})

describe("adapted application composition", () => {
  test("app root routes Home and Session through the adapted layout", () => {
    expect(app).toContain("AppLayout")
    expect(app).toContain("SessionRoute")
    expect(app).toContain("Home")
  })
  test("session page uses the adapted titlebar tab strip", () => {
    expect(page).toContain("Titlebar")
    expect(page).toContain("session-content")
  })
})
