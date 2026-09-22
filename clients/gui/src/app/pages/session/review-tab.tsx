// Wayshard session review (Changes) tab.
//
// Adapted from the imported OpenCode application review tab
// (third_party/opencode-v1.18.31/packages/app/src/pages/session/review-tab.tsx):
// the panel composition is retained (a session-scoped review surface). The
// Wayshard Changes semantics are preserved — Run changes (task-start snapshot →
// run final) and Workspace changes (current source workspace state) — rendered
// through the adapted session-ui diff component.
import type { JSX } from "solid-js"
import { ChangesView } from "../../views"

export function SessionReviewTab(): JSX.Element {
  return (
    <div data-slot="session-review" class="flex min-h-0 flex-1 flex-col overflow-hidden">
      <ChangesView />
    </div>
  )
}
