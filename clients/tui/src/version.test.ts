import { expect, test } from "bun:test"
import { TUI_COMMIT, TUI_VERSION, versionLine } from "./version"

// The release build replaces the injected identifiers (see build.ts); when the
// test runs from source they are absent, so the development fallback must hold
// and stay safe to reference through `typeof`.
test("source build uses the development fallback identity", () => {
  expect(TUI_VERSION).toBe("0.0.0-dev")
  expect(TUI_COMMIT).toBe("unknown")
})

test("versionLine is a single human-readable line", () => {
  expect(versionLine()).toBe("Wayshard TUI 0.0.0-dev (unknown)")
})
