// Tests for the adapted terminal panel helpers.
import { describe, expect, test } from "bun:test"
import { terminalTabLabel } from "./terminal-helpers"

describe("terminalTabLabel (adapted from OpenCode terminal-label)", () => {
  test("prefers a shell-reported title", () => {
    expect(terminalTabLabel({ title: "vim", number: 2 })).toBe("vim")
  })
  test("falls back to the stable numbered label", () => {
    expect(terminalTabLabel({ number: 3 })).toBe("Terminal 3")
  })
  test("blank title falls back to the numbered label", () => {
    expect(terminalTabLabel({ title: "   ", number: 1 })).toBe("Terminal 1")
  })
  test("no title or number yields the generic label", () => {
    expect(terminalTabLabel({})).toBe("Terminal")
  })
})
