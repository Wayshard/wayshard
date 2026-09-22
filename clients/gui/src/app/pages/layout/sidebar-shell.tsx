// Wayshard sidebar shell.
//
// Adapted from the imported OpenCode application sidebar shell
// (third_party/opencode-v1.18.31/packages/app/src/pages/layout/sidebar-shell.tsx):
// the two-part composition is retained — a narrow icon rail of projects with
// add/settings/help affordances, and an expandable panel. OpenCode's
// drag-and-drop project reordering is dropped (Wayshard projects are ordered by
// the server).
import { For, Show, createMemo, type Accessor, type JSX } from "solid-js"
import { IconButton } from "@wayshard/ui/icon-button"
import { Tooltip } from "@wayshard/ui/tooltip"
import type { Project } from "@wayshard/sdk"
import { ProjectIcon } from "./sidebar-items"

export function SidebarContent(props: {
  mobile?: boolean
  opened: Accessor<boolean>
  projects: Accessor<Project[]>
  selectedProjectID: Accessor<string | null>
  onSelectProject: (project: Project) => void
  onOpenProject: () => void
  onOpenSettings: () => void
  onOpenHelp: () => void
  renderPanel: () => JSX.Element
}): JSX.Element {
  const expanded = createMemo(() => !!props.mobile || props.opened())
  const placement = () => (props.mobile ? "bottom" : "right")

  return (
    <div class="flex h-full w-full min-w-0 overflow-hidden">
      <div data-component="sidebar-rail" class="w-16 shrink-0 bg-v2-background-bg-base flex flex-col items-center overflow-hidden">
        <div class="flex-1 min-h-0 w-full">
          <div class="h-full w-full flex flex-col items-center gap-3 px-3 py-3 overflow-y-auto">
            <For each={props.projects()}>
              {(project) => (
                <Tooltip placement={placement()} value={project.name || project.path}>
                  <button
                    type="button"
                    class="rounded-md p-0.5 data-[active=true]:ring-1 data-[active=true]:ring-v2-border-border-focus"
                    data-active={project.id === props.selectedProjectID()}
                    aria-label={project.name || project.path}
                    onClick={() => props.onSelectProject(project)}
                  >
                    <ProjectIcon project={project} />
                  </button>
                </Tooltip>
              )}
            </For>
            <Tooltip placement={placement()} value="Open project">
              <IconButton icon="folder-add-left" variant="ghost" size="large" onClick={props.onOpenProject} aria-label="Open project" />
            </Tooltip>
          </div>
        </div>
        <div class="shrink-0 w-full pt-3 pb-6 flex flex-col items-center gap-2">
          <Tooltip placement={placement()} value="Settings">
            <IconButton icon="settings-gear" variant="ghost" size="large" onClick={props.onOpenSettings} aria-label="Settings" />
          </Tooltip>
          <Tooltip placement={placement()} value="Help">
            <IconButton icon="prompt" variant="ghost" size="large" onClick={props.onOpenHelp} aria-label="Help" />
          </Tooltip>
        </div>
      </div>

      <div
        classList={{ "flex-1 flex h-full min-h-0 min-w-0 overflow-hidden": true, "pointer-events-none": !expanded() }}
        aria-hidden={!expanded()}
      >
        <Show when={expanded()}>{props.renderPanel()}</Show>
      </div>
    </div>
  )
}
