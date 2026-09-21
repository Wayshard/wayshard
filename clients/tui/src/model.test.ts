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
