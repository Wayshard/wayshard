// Wayshard session terminal panel.
//
// Adapted from the imported OpenCode application terminal panel
// (third_party/opencode-v1.18.31/packages/app/src/pages/session/terminal-panel-v2.tsx):
// the session-scoped terminal panel composition is retained. The terminal
// backend remains the Wayshard server-owned PTY rendered through the imported
// ghostty-web presentation.
import type { JSX } from "solid-js"
import { TerminalView } from "../../views"

export function SessionTerminalPanel(): JSX.Element {
  return (
    <div data-slot="session-terminal-panel" class="flex min-h-0 flex-1 flex-col overflow-hidden">
      <TerminalView />
    </div>
  )
}
