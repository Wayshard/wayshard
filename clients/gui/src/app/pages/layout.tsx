// Wayshard desktop + narrow application layout.
//
// Adapted from the imported OpenCode application layout
// (third_party/opencode-v1.18.31/packages/app/src/pages/layout.tsx): the
// composition is retained — an app-level titlebar over a sidebar + main region.
// The persistent sidebar is shown from the `xl` breakpoint; below it the sidebar
// is the `sidebar-nav-mobile` overlay (see ./layout/sidebar-mobile.tsx), matching
// upstream's narrow-width model. OpenCode's multi-server/workspace/permission/
// drag-drop layout state is replaced by the Wayshard layout context.
import { Show, createMemo, type JSX } from "solid-js"
import { Dialog } from "@wayshard/ui/dialog"
import { useDialog } from "@wayshard/ui/context/dialog"
import type { Project } from "@wayshard/sdk"
import { useGlobal } from "../context/global"
import { useLayout } from "../context/layout"
import { SettingsView } from "../views"
import { Titlebar } from "../components/titlebar"
import { SidebarContent } from "./layout/sidebar-shell"
import { ProjectPanel } from "./layout/sidebar-project"
import { SidebarMobile } from "./layout/sidebar-mobile"

export function AppLayout(props: { children: JSX.Element }): JSX.Element {
  const global = useGlobal()
  const layout = useLayout()
  const dialog = useDialog()

  const projects = global.projects.list
  const selectedID = () =>
    layout.home.selection().projectID ?? global.projects.selected()?.id ?? projects()[0]?.id ?? null
  const selected = createMemo(() => projects().find((p) => p.id === selectedID()))

  function selectProject(project: Project) {
    layout.home.setSelection({ projectID: project.id, conversationID: null })
    void global.projects.select(project.id)
    layout.mobileSidebar.hide()
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

  function sidebarPanel(mobile?: boolean) {
    return (
      <SidebarContent
        mobile={mobile}
        opened={() => true}
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
  }

  return (
    <div
      data-component="app-layout"
      class="relative flex h-full w-full min-h-0 flex-col bg-v2-background-bg-base text-v2-text-text-base"
    >
      <Titlebar />
      <div class="relative flex min-h-0 flex-1">
        <div class="hidden h-full min-h-0 w-[280px] shrink-0 xl:flex">{sidebarPanel()}</div>
        <SidebarMobile>{sidebarPanel(true)}</SidebarMobile>
        <main class="flex min-h-0 min-w-0 flex-1 flex-col">{props.children}</main>
      </div>
    </div>
  )
}
