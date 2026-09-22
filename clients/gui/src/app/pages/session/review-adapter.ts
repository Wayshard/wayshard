// Wayshard review data adapter.
//
// Turns the Wayshard domain (run delta + run/snapshot file reads, workspace
// change metadata) into the view model the adapted session-ui SessionReview
// component expects, so generic inherited review presentation stays free of
// direct @wayshard/sdk/backend calls. Run changes are the run starting snapshot
// -> run final; pre-existing user changes are carried through and never
// attributed to the agent.
import { diffLines } from "diff"
import type { WayshardClient } from "@wayshard/sdk"

export type ReviewDiffStatus = "added" | "deleted" | "modified"

export interface ReviewDiff {
  file: string
  additions: number
  deletions: number
  status: ReviewDiffStatus
  before?: string
  after?: string
  binary?: boolean
  preExisting?: boolean
  agentModified?: boolean
}

export interface WorkspaceEntry {
  path: string
  kind: ReviewDiffStatus
}

interface RawFileDelta {
  path?: string
  kind?: string
  agentModified?: boolean
  preExisting?: boolean
}

function toStatus(kind: string | undefined): ReviewDiffStatus {
  if (kind === "added") return "added"
  if (kind === "deleted") return "deleted"
  return "modified"
}

export function countChanges(before: string, after: string): { additions: number; deletions: number } {
  let additions = 0
  let deletions = 0
  for (const part of diffLines(before, after)) {
    if (part.added) additions += part.count ?? 0
    else if (part.removed) deletions += part.count ?? 0
  }
  return { additions, deletions }
}

export function runDeltaToDiffs(delta: { files?: RawFileDelta[] } | undefined): ReviewDiff[] {
  return (delta?.files ?? [])
    .filter((f): f is RawFileDelta & { path: string } => typeof f.path === "string")
    .map((f) => ({
      file: f.path,
      additions: 0,
      deletions: 0,
      status: toStatus(f.kind),
      preExisting: f.preExisting,
      agentModified: f.agentModified,
    }))
}

export function workspaceEntries(files: Record<string, { kind?: string }> | undefined): WorkspaceEntry[] {
  return Object.entries(files ?? {})
    .map(([path, meta]) => ({ path, kind: toStatus(meta?.kind) }))
    .sort((a, b) => a.path.localeCompare(b.path))
}

// mapLimit bounds concurrent file reads so a large run delta cannot open an
// unbounded number of requests at once.
async function mapLimit<T, R>(items: T[], limit: number, fn: (item: T) => Promise<R>): Promise<R[]> {
  const out = new Array<R>(items.length)
  let next = 0
  const workers = Array.from({ length: Math.max(1, Math.min(limit, items.length)) }, async () => {
    for (;;) {
      const index = next++
      if (index >= items.length) return
      out[index] = await fn(items[index])
    }
  })
  await Promise.all(workers)
  return out
}

// hydrateRunDiffs fetches both sides of each changed file and computes line
// counts. Binary files keep zero counts and are flagged so the review does not
// pretend to diff them as text.
export async function hydrateRunDiffs(
  client: WayshardClient,
  runID: string,
  items: ReviewDiff[],
): Promise<ReviewDiff[]> {
  return mapLimit(items, 6, async (item) => {
    try {
      const [before, after] = await Promise.all([
        item.status === "added" ? Promise.resolve(undefined) : client.runFile(runID, item.file, "snapshot"),
        item.status === "deleted" ? Promise.resolve(undefined) : client.runFile(runID, item.file, "run"),
      ])
      const binary = Boolean(before?.binary || after?.binary)
      if (binary) return { ...item, binary: true, before: "", after: "", additions: 0, deletions: 0 }
      const b = before && !before.missing ? before.content : ""
      const a = after && !after.missing ? after.content : ""
      const { additions, deletions } = countChanges(b, a)
      return { ...item, before: b, after: a, additions, deletions }
    } catch {
      // A file that cannot be read stays listed with no content rather than
      // disappearing from the review.
      return item
    }
  })
}
