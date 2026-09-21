import { describe, expect, test } from "bun:test"
import { filterCommands, stageDisplay } from "./model"

describe("wayshard tui view-model", () => {
  test("stage labels cover the orchestration stages", () => {
    expect(stageDisplay("plan")).toBe("Planning")
    expect(stageDisplay("validate")).toBe("Validating")
    expect(stageDisplay("integrate")).toBe("Integrating")
    expect(stageDisplay("unknown-stage")).toBe("Unknown-stage")
  })

  test("palette filtering is case-insensitive and preserves order", () => {
    const cmds = [
      { id: "a", title: "New session" },
      { id: "b", title: "Cancel run" },
      { id: "c", title: "Integrate run" },
    ]
    expect(filterCommands(cmds, "").length).toBe(3)
    expect(filterCommands(cmds, "run").map((c) => c.id)).toEqual(["b", "c"])
    expect(filterCommands(cmds, "SESSION").map((c) => c.id)).toEqual(["a"])
    expect(filterCommands(cmds, "zzz").length).toBe(0)
  })
})

import { TUI_TABS, cycleTab, paletteOptions } from "./model"

describe("wayshard tui navigation", () => {
  test("tab cycling wraps", () => {
    expect(cycleTab("session", 1)).toBe("changes")
    expect(cycleTab("settings", 1)).toBe("session")
    expect(cycleTab("session", -1)).toBe("settings")
    expect(TUI_TABS.length).toBe(7)
  })

  test("palette options filter by title", () => {
    const cmds = [
      { id: "run.cancel", title: "Cancel run", category: "run" },
      { id: "run.retry", title: "Retry run", category: "run" },
      { id: "session.new", title: "New session", category: "session" },
    ]
    expect(paletteOptions(cmds, "run").map((c) => c.id)).toEqual(["run.cancel", "run.retry"])
    expect(paletteOptions(cmds, "").length).toBe(3)
    expect(paletteOptions(cmds, "zzz").length).toBe(0)
  })
})
