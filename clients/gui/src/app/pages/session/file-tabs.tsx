// Wayshard session file tabs.
//
// Adapted from the imported OpenCode application session file tabs
// (third_party/opencode-v1.18.31/packages/app/src/pages/session/file-tabs.tsx):
// the session-scoped file browser panel is retained. The Wayshard file tree and
// editor with expectedHash conflict semantics are integrated here.
import type { JSX } from "solid-js"
import { FilesView } from "../../views"

export function SessionFileTabs(): JSX.Element {
  return (
    <div data-slot="session-file-tabs" class="flex min-h-0 flex-1 flex-col overflow-hidden">
      <FilesView />
    </div>
  )
}
