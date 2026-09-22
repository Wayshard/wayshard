// Wayshard sidebar project block.
//
// Adapted from the imported OpenCode application project sidebar
// (third_party/opencode-v1.18.31/packages/app/src/pages/layout/sidebar-project.tsx):
// the project header (icon + name + actions) and its session list are retained.
// OpenCode worktree/sandbox/notification state is replaced by Wayshard Projects
// and Conversations.
import { For, Show, createEffect, createMemo } from "solid-js"
import type { Project } from "@wayshard/sdk"
import { useGlobal } from "../../context/global"
import { ProjectIcon, SessionItem, NewSessionItem } from "./sidebar-items"

export function ProjectPanel(props: { project: Project; dense?: boolean }): import("solid-js").JSX.Element {
  const global = useGlobal()
  const sessions = createMemo(() => global.sessions.list(props.project.id))

  createEffect(() => {
    void global.sessions.load(props.project.id)
  })

  return (
    <div data-project-id={props.project.id} class="flex min-h-0 w-full flex-col gap-1">
      <div class="flex items-center gap-2 px-2 py-1">
        <ProjectIcon project={props.project} />
        <span class="min-w-0 flex-1 truncate text-sm font-medium text-v2-text-text-base">
          {props.project.name || props.project.path}
        </span>
      </div>
      <div class="flex flex-col gap-0.5">
        <For each={sessions()}>
          {(session) => <SessionItem project={props.project} session={session} dense={props.dense} />}
        </For>
        <Show when={!sessions().length}>
          <div class="px-2 py-1 text-[11px] text-v2-text-text-muted">No sessions yet</div>
        </Show>
        <NewSessionItem project={props.project} dense={props.dense} />
      </div>
    </div>
  )
}
