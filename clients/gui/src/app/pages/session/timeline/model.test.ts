// Tests for the Wayshard timeline model (row projection + reconciliation).
import { describe, expect, test } from "bun:test"
import { activeMessageKey, buildTimelineRows, reconcileRows, rowKey, type TimelineRow } from "./model"

const run = { id: "run_1", status: "running" } as never
const stages = [
  { id: "st_1", kind: "plan", ordinal: 1, status: "complete" },
  { id: "st_2", kind: "execute", ordinal: 2, status: "running" },
] as never

describe("buildTimelineRows", () => {
  test("projects messages, then run, then stages into one ordered model", () => {
    const rows = buildTimelineRows({
      messages: [
        { id: "m1", role: "user" },
        { id: "m2", role: "assistant" },
      ],
      run,
      stages,
    })
    expect(rows.map((r) => r.tag)).toEqual(["message", "message", "run", "stage", "stage"])
    expect(rows.map(rowKey)).toEqual([
      "message:m1",
      "message:m2",
      "run:run_1",
      "stage:st_1",
      "stage:st_2",
    ])
  })

  test("omits run/stage rows without a run", () => {
    const rows = buildTimelineRows({ messages: [{ id: "m1", role: "user" }], run: null, stages: [] })
    expect(rows.map((r) => r.tag)).toEqual(["message"])
  })
})

describe("activeMessageKey", () => {
  test("selects the latest user message", () => {
    const rows = buildTimelineRows({
      messages: [
        { id: "m1", role: "user" },
        { id: "m2", role: "assistant" },
        { id: "m3", role: "user" },
      ],
      run,
      stages: [],
    })
    expect(activeMessageKey(rows)).toBe("message:m3")
  })

  test("returns undefined when there is no user message", () => {
    expect(activeMessageKey([])).toBeUndefined()
  })
})

describe("reconcileRows", () => {
  const first = buildTimelineRows({
    messages: [{ id: "m1", role: "user" }],
    run,
    stages: [stages[0]],
  })

  test("reuses unchanged rows by stable key", () => {
    const next = buildTimelineRows({
      messages: [{ id: "m1", role: "user" }],
      run,
      stages: [stages[0]],
    })
    const merged = reconcileRows(first, next)
    expect(merged[0]).toBe(first[0])
    expect(merged).toBe(first)
  })

  test("replaces a changed row while keeping unchanged rows stable", () => {
    const next = buildTimelineRows({
      messages: [{ id: "m1", role: "user" }],
      run,
      stages: [{ id: "st_1", kind: "plan", ordinal: 1, status: "failed" } as never],
    })
    const merged = reconcileRows(first, next)
    expect(merged[0]).toBe(first[0])
    expect(merged[2]).not.toBe(first[2])
    expect((merged[2] as { stage: { status: string } }).stage.status).toBe("failed")
  })

  test("returns the incoming rows when there is no previous state", () => {
    const rows: TimelineRow[] = []
    expect(reconcileRows(undefined, rows)).toBe(rows)
  })
})
