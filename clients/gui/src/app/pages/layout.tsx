// Wayshard desktop application layout.
//
// Adapted from the imported OpenCode application layout
// (third_party/opencode-v1.18.31/packages/app/src/pages/layout.tsx): the
// sidebar + main composition is retained, using the adapted sidebar shell and
// project block. OpenCode's multi-server/workspace/permission/drag-drop layout
// state is replaced by the Wayshard layout context and server-owned projects.
// Narrow-width navigation is refined from the upstream mobile source in a later
// stage of this port.
import { Show, createMemo, createSignal, onCleanup, type JSX } from "solid-js"
import { Dialog } from "@wayshard/ui/dialog"
import { useDialog } from "@wayshard/ui/context/dialog"
import { Icon } from "@wayshard/ui/icon"
import type { Project } from "@wayshard/sdk"
import { useGlobal } from "../context/global"
import { useLayout } from "../context/layout"
import { SettingsView } from "../views"
import { SidebarContent } from "./layout/sidebar-shell"
import { ProjectPanel } from "./layout/sidebar-project"
import { SidebarMobile } from "./layout/sidebar-mobile"

function useNarrow() {
  const mq = typeof window === "object" && window.matchMedia ? window.matchMedia("(max-width: 820px)") : undefined
  const [narrow, setNarrow] = createSignal(mq?.matches ?? false)
  if (mq) {
    const onChange = () => setNarrow(mq.matches)
    mq.addEventListener?.("change", onChange)
    onCleanup(() => mq.removeEventListener?.("change", onChange))
  }
  return narrow
}

export function AppLayout(props: { children: JSX.Element }): JSX.Element {
  const global = useGlobal()
  const layout = useLayout()
  const dialog = useDialog()
  const narrow = useNarrow()
  const [mobileOpen, setMobileOpen] = createSignal(false)

  const projects = global.projects.list
  const selectedID = () =>
    layout.home.selection().projectID ?? global.projects.selected()?.id ?? projects()[0]?.id ?? null
  const selected = createMemo(() => projects().find((p) => p.id === selectedID()))

  function selectProject(project: Project) {
    layout.home.setSelection({ projectID: project.id, conversationID: null })
    void global.projects.select(project.id)
    setMobileOpen(false)
  }

  function openProject() {
    const path = window.prompt("Project path")
    if (path) void global.projects.open(path)
  }

  function openSettings() {
    dialog.show(() => (
      <Dialog title="Settings" size="large">
        <div class="wh-dialog-surface">
          <SettingsView />
        </div>
      </Dialog>
    ))
  }

  function openHelp() {
    window.open("https://wayshard.dev", "_blank", "noopener")
  }

  const sidebar = () => (
    <SidebarContent
      mobile={narrow()}
      opened={() => !narrow() || mobileOpen()}
      projects={projects}
      selectedProjectID={selectedID}
      onSelectProject={selectProject}
      onOpenProject={openProject}
      onOpenSettings={openSettings}
      onOpenHelp={openHelp}
      renderPanel={() => (
        <Show when={selected()}>
          {(project) => (
            <div class="flex h-full w-full min-h-0 flex-col overflow-y-auto bg-v2-background-bg-base px-2 py-3">
              <ProjectPanel project={project()} />
            </div>
          )}
        </Show>
      )}
    />
  )

  return (
    <div data-component="app-layout" class="flex h-full w-full min-h-0 bg-v2-background-bg-base text-v2-text-text-base">
      <Show when={narrow()}>
        <div class="flex items-center gap-2 px-2 py-1.5">
          <button
            type="button"
            class="flex size-8 items-center justify-center rounded-md hover:bg-v2-background-bg-layer-01"
            aria-label="Toggle navigation"
            aria-expanded={mobileOpen()}
            onClick={() => setMobileOpen(!mobileOpen())}
          >
            <Icon name="bullet-list" size="small" />
          </button>
        </div>
        <SidebarMobile open={mobileOpen()} onClose={() => setMobileOpen(false)}>
          {sidebar()}
        </SidebarMobile>
      </Show>
      <div classList={{ "hidden h-full min-h-0 md:flex": true, "w-[280px] shrink-0": true }}>
        {sidebar()}
      </div>
      <main class="flex min-h-0 min-w-0 flex-1 flex-col">{props.children}</main>
    </div>
  )
}
