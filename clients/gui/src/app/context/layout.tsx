// Wayshard application layout context.
//
// Adapted from the imported OpenCode application layout context
// (third_party/opencode-v1.18.31/packages/app/src/context/layout.tsx): the
// application-level shape is retained (home project selection, sidebar
// collapse state, active work surface) while the backing state is Wayshard's.
import { createSignal } from "solid-js"
import { createSimpleContext } from "@wayshard/ui/context/helper"

export type HomeProjectSelection = {
  projectID: string | null
  conversationID: string | null
}

export type WorkSurface = "session" | "changes" | "files" | "terminal"

export const { use: useLayout, provider: LayoutProvider } = createSimpleContext({
  name: "Layout",
  init: () => {
    const [selection, setSelection] = createSignal<HomeProjectSelection>({ projectID: null, conversationID: null })
    const [sidebarCollapsed, setSidebarCollapsed] = createSignal(false)
    const [surface, setSurface] = createSignal<WorkSurface>("session")

    return {
      home: {
        selection,
        setSelection,
      },
      sidebar: {
        collapsed: sidebarCollapsed,
        toggle: () => setSidebarCollapsed((v) => !v),
      },
      surface: {
        value: surface,
        set: setSurface,
      },
    }
  },
})
