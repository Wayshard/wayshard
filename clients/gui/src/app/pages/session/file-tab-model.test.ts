// Tests for the Wayshard file tab model (pure reducers).
import { describe, expect, test } from "bun:test"
import { activateFile, closeFile, createFileTabState, openFile, setMode, setScroll } from "./file-tab-model"

describe("file tab model", () => {
  test("openFile appends and activates", () => {
    let s = createFileTabState()
    s = openFile(s, "a.ts")
    s = openFile(s, "b.ts")
    expect(s.tabs).toEqual(["a.ts", "b.ts"])
    expect(s.active).toBe("b.ts")
  })

  test("openFile on an open file switches without duplicating", () => {
    let s = openFile(openFile(createFileTabState(), "a.ts"), "b.ts")
    s = openFile(s, "a.ts")
    expect(s.tabs).toEqual(["a.ts", "b.ts"])
    expect(s.active).toBe("a.ts")
  })

  test("closeFile of active picks the left neighbor", () => {
    let s = openFile(openFile(openFile(createFileTabState(), "a.ts"), "b.ts"), "c.ts")
    s = activateFile(s, "b.ts")
    s = closeFile(s, "b.ts")
    expect(s.tabs).toEqual(["a.ts", "c.ts"])
    expect(s.active).toBe("a.ts")
  })

  test("closeFile of first active picks the next tab", () => {
    let s = openFile(openFile(createFileTabState(), "a.ts"), "b.ts")
    s = activateFile(s, "a.ts")
    s = closeFile(s, "a.ts")
    expect(s.active).toBe("b.ts")
  })

  test("closeFile of the last tab clears active", () => {
    let s = openFile(createFileTabState(), "only.ts")
    s = closeFile(s, "only.ts")
    expect(s.tabs).toEqual([])
    expect(s.active).toBeUndefined()
  })

  test("closeFile of a non-active tab keeps active", () => {
    let s = openFile(openFile(createFileTabState(), "a.ts"), "b.ts")
    s = closeFile(s, "a.ts")
    expect(s.active).toBe("b.ts")
  })

  test("activateFile ignores unknown paths", () => {
    const s = createFileTabState()
    expect(activateFile(s, "ghost.ts")).toBe(s)
  })

  test("setMode switches the Changes/All organization", () => {
    expect(setMode(createFileTabState(), "all").mode).toBe("all")
  })

  test("setScroll records a position per file", () => {
    const s = setScroll(createFileTabState(), "a.ts", { x: 1, y: 42 })
    expect(s.scroll["a.ts"]).toEqual({ x: 1, y: 42 })
  })
})
