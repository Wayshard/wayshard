// Wayshard sidebar items.
//
// Adapted from the imported OpenCode application sidebar items
// (third_party/opencode-v1.18.31/packages/app/src/pages/layout/sidebar-items.tsx):
// the project icon (avatar + notification dot) and session row structure are
// retained. OpenCode's server-sync/notification/permission/agent-color model is
// replaced by Wayshard Projects and Conversations.
import { Show, createMemo } from "solid-js"
import { Avatar } from "@wayshard/ui/avatar"
import { Icon } from "@wayshard/ui/icon"
import { Tooltip } from "@wayshard/ui/tooltip"
import type { Conversation, Project } from "@wayshard/sdk"
import { useNavigate } from "../../router"
import { useLayout } from "../../context/layout"

export function ProjectIcon(props: { project: Project; class?: string; notify?: boolean }): import("solid-js").JSX.Element {
  const name = createMemo(() => props.project.name || props.project.path)
  return (
    <div class={`relative size-8 shrink-0 rounded ${props.class ?? ""}`}>
      <div class="size-full rounded overflow-clip">
        <Avatar fallback={name()} class="size-full rounded" />
      </div>
      <Show when={props.notify}>
        <div class="absolute top-px right-px size-1.5 rounded-full z-10 bg-v2-text-text-accent" />
      </Show>
    </div>
  )
}

export function SessionItem(props: { project: Project; session: Conversation; dense?: boolean }): import("solid-js").JSX.Element {
  const navigate = useNavigate()
  const layout = useLayout()
  const active = () => layout.home.selection().conversationID === props.session.id
  const title = () => props.session.title || "Session"

  return (
    <button
      type="button"
      data-session-id={props.session.id}
      class={`group/session relative w-full min-w-0 rounded-md pr-3 text-left transition-colors hover:bg-v2-background-bg-layer-01 data-[active=true]:bg-v2-background-bg-layer-02 ${props.dense ? "py-0.5" : "py-1"}`}
      data-active={active()}
      onClick={() => navigate(`/${props.project.id}/session/${props.session.id}`)}
    >
      <div class="flex min-w-0 items-center gap-1">
        <span class="shrink-0 size-6 flex items-center justify-center text-v2-icon-icon-muted">
          <Icon name="bubble-5" size="small" />
        </span>
        <Tooltip placement="right" value={title()}>
          <span class="text-sm text-v2-text-text-base min-w-0 flex-1 truncate">{title()}</span>
        </Tooltip>
      </div>
    </button>
  )
}

export function NewSessionItem(props: { project: Project; dense?: boolean }): import("solid-js").JSX.Element {
  const navigate = useNavigate()
  return (
    <button
      type="button"
      class={`flex w-full min-w-0 items-center gap-2 rounded-md pl-2 pr-3 text-left hover:bg-v2-background-bg-layer-01 ${props.dense ? "py-0.5" : "py-1"}`}
      onClick={() => navigate("/new-session")}
    >
      <span class="shrink-0 size-6 flex items-center justify-center text-v2-icon-icon-muted">
        <Icon name="new-session" size="small" />
      </span>
      <span class="text-sm text-v2-text-text-base min-w-0 flex-1 truncate">New session</span>
    </button>
  )
}
