// Guards the graphical theme/navigation integration that rendered validation
// proved was broken, and the adapted application composition: the app must
// import the adapted v2 theme layer and mount the adapted ThemeProvider, the
// "More" overflow must go through the shared dialog provider, and the narrow
// model must expose the upstream-derived mobile navigation.
import { describe, expect, test } from "bun:test"
import { readFileSync } from "node:fs"
import { join } from "node:path"

const src = import.meta.dir
const styles = readFileSync(join(src, "styles.css"), "utf8")
const app = readFileSync(join(src, "app", "app.tsx"), "utf8")
const page = readFileSync(join(src, "app", "session-page.tsx"), "utf8")
const layout = readFileSync(join(src, "app", "pages", "layout.tsx"), "utf8")
const titlebar = readFileSync(join(src, "app", "components", "titlebar.tsx"), "utf8")
const mobile = readFileSync(join(src, "app", "pages", "layout", "sidebar-mobile.tsx"), "utf8")

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
    expect(titlebar).toContain("function openMore()")
    expect(titlebar).toMatch(/openMore[\s\S]*dialog\.show\(/)
  })
  test("no inline dialog rendered from a moreOpen signal", () => {
    expect(titlebar).not.toContain("moreOpen")
  })
})

describe("narrow/mobile model (upstream-derived)", () => {
  test("titlebar exposes the mobile navigation toggle", () => {
    expect(titlebar).toContain("Toggle navigation")
    expect(titlebar).toContain("mobileSidebar")
  })
  test("sidebar-nav-mobile overlay uses layout mobileSidebar state", () => {
    expect(mobile).toContain('data-component="sidebar-nav-mobile"')
    expect(mobile).toContain("mobileSidebar")
  })
  test("layout hides the persistent sidebar below the xl breakpoint", () => {
    expect(layout).toContain("xl:flex")
    expect(layout).toContain("SidebarMobile")
  })
})

describe("adapted application composition", () => {
  test("app root routes Home and Session through the adapted layout", () => {
    expect(app).toContain("AppLayout")
    expect(app).toContain("SessionRoute")
    expect(app).toContain("Home")
  })
  test("session page uses the adapted timeline inside the content region", () => {
    expect(page).toContain("SessionTimeline")
    expect(page).toContain("session-content")
  })
})
