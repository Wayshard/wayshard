// Wayshard session timeline model.
//
// Adapted from the imported OpenCode application timeline model/rows
// (third_party/opencode-v1.18.31/packages/app/src/pages/session/timeline/model.ts,
// rows.ts, timeline-row.ts): the behavior-bearing structure is retained — a
// single ordered row projection with stable row keys and row reconciliation, so
// messages and Wayshard run stages live in one row model rather than a custom
// block appended beneath the messages. OpenCode message/part projections are
// replaced by the Wayshard message/run/stage model.
import type { Run, Stage } from "@wayshard/sdk"
import { stageDisplay } from "../../../../wayshard/adapter"

export interface MessageRow {
  tag: "message"
  key: string
  id: string
  role: string
}

export interface RunRow {
  tag: "run"
  key: string
  run: Run
}

export interface StageRow {
  tag: "stage"
  key: string
  stage: Stage
}

export type TimelineRow = MessageRow | RunRow | StageRow

export function rowKey(row: TimelineRow): string {
  switch (row.tag) {
    case "message":
      return `message:${row.id}`
    case "run":
      return `run:${row.run.id}`
    case "stage":
      return `stage:${row.stage.id}`
  }
}

function rowsEqual(a: TimelineRow, b: TimelineRow): boolean {
  if (a.tag !== b.tag) return false
  if (a.tag === "message" && b.tag === "message") return a.id === b.id && a.role === b.role
  if (a.tag === "run" && b.tag === "run") return a.run.id === b.run.id && a.run.status === b.run.status
  if (a.tag === "stage" && b.tag === "stage") {
    return (
      a.stage.id === b.stage.id &&
      a.stage.status === b.stage.status &&
      a.stage.ordinal === b.stage.ordinal &&
      a.stage.kind === b.stage.kind
    )
  }
  return false
}

// reconcileRows preserves the previous row object when its stable key and
// content are unchanged, so Solid's <For> reuses the same DOM node instead of
// remounting the whole timeline on every event. Adapted from the upstream
// reuseTimelineRows row reconciliation.
export function reconcileRows(previous: TimelineRow[] | undefined, next: TimelineRow[]): TimelineRow[] {
  if (!previous?.length) return next
  const byKey = new Map(previous.map((row) => [row.key, row] as const))
  const out = next.map((row) => {
    const existing = byKey.get(row.key)
    return existing && rowsEqual(existing, row) ? existing : row
  })
  if (previous.length === out.length && previous.every((row, index) => row === out[index])) return previous
  return next.length === out.length ? out : next
}

export function buildTimelineRows(input: {
  messages: Array<{ id: string; role: string }>
  run: Run | null | undefined
  stages: Stage[]
}): TimelineRow[] {
  const rows: TimelineRow[] = input.messages.map((message) => ({
    tag: "message",
    key: `message:${message.id}`,
    id: message.id,
    role: message.role,
  }))
  if (input.run) rows.push({ tag: "run", key: `run:${input.run.id}`, run: input.run })
  for (const stage of input.stages) rows.push({ tag: "stage", key: `stage:${stage.id}`, stage })
  return rows
}

// activeMessageKey identifies the latest user turn, which the timeline marks as
// the active message for reveal/anchoring.
export function activeMessageKey(rows: TimelineRow[]): string | undefined {
  for (let i = rows.length - 1; i >= 0; i--) {
    const row = rows[i]
    if (row.tag === "message" && row.role === "user") return row.key
  }
  return undefined
}

export function stageLabel(kind: string): string {
  return stageDisplay(kind)
}

export function runLabel(run: { id: string; status: string } | null | undefined): string {
  return run ? `Run ${run.id.slice(0, 8)} · ${run.status}` : ""
}
