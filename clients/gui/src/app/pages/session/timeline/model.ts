// Wayshard session timeline model.
//
// Adapted from the imported OpenCode application timeline model
// (third_party/opencode-v1.18.31/packages/app/src/pages/session/timeline/model.ts):
// the stage/run presentation helpers are retained; OpenCode message/part
// projections are replaced by the Wayshard run/stage model.
import { stageDisplay } from "../../../../wayshard/adapter"

export function stageLabel(kind: string): string {
  return stageDisplay(kind)
}

export function runLabel(run: { id: string; status: string } | null | undefined): string {
  return run ? `Run ${run.id.slice(0, 8)} · ${run.status}` : ""
}
