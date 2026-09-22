// Wayshard session terminal panel.
//
// Adapted from the imported OpenCode application terminal panel
// (third_party/opencode-v1.18.31/packages/app/src/pages/session/terminal-panel-v2.tsx):
// the panel composition is retained (a session-scoped terminal panel hosting the
// renderer). The terminal backend remains the Wayshard server-owned PTY rendered
// through the imported ghostty-web presentation; the panel hosts the renderer
// directly rather than delegating to a generic view.
import { Show, type JSX } from "solid-js"
import { useWayshard } from "../../../wayshard/state"
import { EmptyState } from "../../components/state-views"
import { Terminal } from "../../terminal"

export function SessionTerminalPanel(): JSX.Element {
  const ws = useWayshard()
  return (
    <div data-slot="session-terminal-panel" class="flex min-h-0 flex-1 flex-col overflow-hidden">
      <Show
        when={ws.state.activeProjectID}
        fallback={<EmptyState title="No project selected" body="Open a project to use a terminal." />}
      >
        <Terminal projectId={ws.state.activeProjectID!} />
      </Show>
    </div>
  )
}
