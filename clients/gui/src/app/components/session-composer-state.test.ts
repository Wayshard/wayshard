// Tests for the adapted composer dock state machine (pure boundary).
import { describe, expect, test } from "bun:test"
import { composerDockState } from "./session-composer-state"

describe("composerDockState (adapted from OpenCode session-composer-state todoState)", () => {
  test("switching to a session with pending requests opens the dock", () => {
    expect(composerDockState({ count: 2, sessionChanged: true, wasOpen: false })).toBe("open")
  })

  test("switching to a session with no requests hides the dock", () => {
    expect(composerDockState({ count: 0, sessionChanged: true, wasOpen: true })).toBe("hide")
  })

  test("a newly arriving request opens the dock", () => {
    expect(composerDockState({ count: 1, sessionChanged: false, wasOpen: false })).toBe("open")
  })

  test("resolving the last request closes an open dock", () => {
    expect(composerDockState({ count: 0, sessionChanged: false, wasOpen: true })).toBe("close")
  })

  test("a dock that was never open hides instead of animating closed", () => {
    expect(composerDockState({ count: 0, sessionChanged: false, wasOpen: false })).toBe("hide")
  })
})
