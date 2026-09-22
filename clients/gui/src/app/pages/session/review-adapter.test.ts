// Tests for the Wayshard review data adapter.
import { describe, expect, test } from "bun:test"
import { countChanges, hydrateRunDiffs, runDeltaToDiffs, workspaceEntries, type ReviewDiff } from "./review-adapter"

describe("countChanges", () => {
  test("counts added and removed lines", () => {
    const { additions, deletions } = countChanges("a\nb\nc\n", "a\nB\nc\nd\n")
    expect(additions).toBe(2)
    expect(deletions).toBe(1)
  })
  test("identical content has no changes", () => {
    expect(countChanges("x\n", "x\n")).toEqual({ additions: 0, deletions: 0 })
  })
})

describe("runDeltaToDiffs", () => {
  test("maps Wayshard file kinds to review status and preserves provenance", () => {
    const diffs = runDeltaToDiffs({
      files: [
        { path: "a.ts", kind: "modified", agentModified: true, preExisting: false },
        { path: "b.ts", kind: "added", agentModified: true, preExisting: true },
        { path: "c.ts", kind: "deleted", agentModified: true },
      ],
    })
    expect(diffs.map((d) => [d.file, d.status])).toEqual([
      ["a.ts", "modified"],
      ["b.ts", "added"],
      ["c.ts", "deleted"],
    ])
    expect(diffs[1].preExisting).toBe(true)
  })
  test("unknown kind defaults to modified", () => {
    expect(runDeltaToDiffs({ files: [{ path: "x", kind: "weird" }] })[0].status).toBe("modified")
  })
  test("empty delta yields no diffs", () => {
    expect(runDeltaToDiffs(undefined)).toEqual([])
  })
})

describe("workspaceEntries", () => {
  test("sorts paths and maps kinds", () => {
    expect(workspaceEntries({ "z.ts": { kind: "added" }, "a.ts": { kind: "modified" } })).toEqual([
      { path: "a.ts", kind: "modified" },
      { path: "z.ts", kind: "added" },
    ])
  })
})

describe("hydrateRunDiffs", () => {
  const stubClient = (files: Record<string, { snapshot?: string; run?: string; binary?: boolean }>) =>
    ({
      runFile: async (_runID: string, path: string, side: "run" | "snapshot") => {
        const entry = files[path] ?? {}
        const binary = entry.binary ?? false
        if (side === "snapshot") {
          return { path, content: entry.snapshot ?? "", hash: "h", binary, missing: entry.snapshot === undefined }
        }
        return { path, content: entry.run ?? "", hash: "h", binary, missing: entry.run === undefined }
      },
    }) as never

  test("fills before/after and line counts", async () => {
    const items: ReviewDiff[] = [{ file: "a.ts", additions: 0, deletions: 0, status: "modified" }]
    const client = stubClient({ "a.ts": { snapshot: "a\nb\n", run: "a\nB\n" } })
    const [hydrated] = await hydrateRunDiffs(client, "run_1", items)
    expect(hydrated.before).toBe("a\nb\n")
    expect(hydrated.after).toBe("a\nB\n")
    expect(hydrated.additions).toBe(1)
    expect(hydrated.deletions).toBe(1)
  })

  test("added files have no snapshot side", async () => {
    const items: ReviewDiff[] = [{ file: "new.ts", additions: 0, deletions: 0, status: "added" }]
    const client = stubClient({ "new.ts": { run: "hello\n" } })
    const [hydrated] = await hydrateRunDiffs(client, "run_1", items)
    expect(hydrated.before).toBe("")
    expect(hydrated.additions).toBe(1)
  })

  test("binary files keep zero counts and are flagged", async () => {
    const items: ReviewDiff[] = [{ file: "logo.png", additions: 0, deletions: 0, status: "modified" }]
    const client = stubClient({ "logo.png": { snapshot: "x", run: "y", binary: true } })
    const [hydrated] = await hydrateRunDiffs(client, "run_1", items)
    expect(hydrated.binary).toBe(true)
    expect(hydrated.additions).toBe(0)
    expect(hydrated.deletions).toBe(0)
  })
})
