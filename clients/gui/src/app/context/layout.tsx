// Wayshard application layout context.
//
// Adapted from the imported OpenCode application layout context
// (third_party/opencode-v1.18.31/packages/app/src/context/layout.tsx): the
// application-level shape is retained (home project selection, sidebar state,
// mobile sidebar state, active work surface) while the backing state is
// Wayshard's. The mobile sidebar state mirrors upstream's
// `mobileSidebar` (opened/show/hide/toggle), used by the app-level titlebar and
// the narrow-width `sidebar-nav-mobile` overlay.
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
    const [mobileSidebarOpened, setMobileSidebarOpened] = createSignal(false)
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
      mobileSidebar: {
        opened: mobileSidebarOpened,
        show: () => setMobileSidebarOpened(true),
        hide: () => setMobileSidebarOpened(false),
        toggle: () => setMobileSidebarOpened((v) => !v),
      },
      surface: {
        value: surface,
        set: setSurface,
      },
    }
  },
})
