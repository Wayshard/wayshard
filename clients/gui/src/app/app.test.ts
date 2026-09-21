// Tests for the adapted OpenCode-derived application shell modules.
import { describe, expect, test } from "bun:test"
import {
  activeCommandRegistrations,
  addCommandRegistration,
  commandPaletteOptions,
  displayKeybind,
  matchKeybind,
  parseKeybind,
  type CommandOption,
} from "./command"

describe("wayshard command architecture (adapted from packages/app/src/context/command.tsx)", () => {
  test("parseKeybind parses modifiers and key", () => {
    const kb = parseKeybind("mod+shift+k")[0]
    expect(kb.key).toBe("k")
    expect(kb.shift).toBe(true)
    expect(kb.ctrl || kb.meta).toBe(true)
    expect(parseKeybind("none")).toEqual([])
  })

  test("matchKeybind matches exact modifier state", () => {
    const event = { key: "k", ctrlKey: true, metaKey: false, shiftKey: false, altKey: false } as KeyboardEvent
    expect(matchKeybind(parseKeybind("ctrl+k"), event)).toBe(true)
    expect(matchKeybind(parseKeybind("ctrl+shift+k"), event)).toBe(false)
    expect(matchKeybind(parseKeybind("ctrl+j"), event)).toBe(false)
  })

  test("displayKeybind produces a human label", () => {
    expect(displayKeybind("ctrl+k")).toContain("K")
    expect(displayKeybind("none")).toBe("")
  })

  test("commandPaletteOptions hides disabled/hidden/suggested options", () => {
    const options: CommandOption[] = [
      { id: "a", title: "A" },
      { id: "b", title: "B", disabled: true },
      { id: "c", title: "C", hidden: true },
      { id: "suggested.x", title: "S" },
      { id: "file.open", title: "Open file" },
    ]
    expect(commandPaletteOptions(options).map((o) => o.id)).toEqual(["a"])
  })

  test("registration lifecycle is newest-first and deduplicated by key", () => {
    const a = { options: () => [] as CommandOption[] }
    const b = { key: "shared", options: () => [] as CommandOption[] }
    const c = { key: "shared", options: () => [] as CommandOption[] }
    let regs = addCommandRegistration([], a)
    regs = addCommandRegistration(regs, b)
    regs = addCommandRegistration(regs, c)
    expect(regs[0]).toBe(c)
    const active = activeCommandRegistrations(regs)
    expect(active).toContain(c)
    expect(active).not.toContain(b)
  })
})
