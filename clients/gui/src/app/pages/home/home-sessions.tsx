// Wayshard Home sessions column.
//
// Adapted from the imported OpenCode application sessions view
// (third_party/opencode-v1.18.31/packages/app/src/pages/home/home-sessions-view.tsx,
// home-sessions-controller.tsx and home-session-search-controller.ts): the
// grouped session list, search field, selected state and empty state are
// retained; OpenCode sessions are mapped to Wayshard Conversations.
import { For, Show, createEffect, createMemo, createSignal } from "solid-js"
import { useNavigate } from "../../router"
import { TextField } from "@wayshard/ui/text-field"
import { Button } from "@wayshard/ui/button"
import { Icon } from "@wayshard/ui/icon"
import { useGlobal } from "../../context/global"
import { useLayout } from "../../context/layout"

export function HomeSessions() {
  const global = useGlobal()
  const layout = useLayout()
  const navigate = useNavigate()
  const [query, setQuery] = createSignal("")

  // Load sessions for every project so the home view can group and search them.
  createEffect(() => {
    for (const project of global.projects.list()) void global.sessions.load(project.id)
  })

  const groups = createMemo(() => {
    const q = query().trim().toLowerCase()
    return global
      .projects
      .list()
      .map((project) => ({
        project,
        sessions: global
          .sessions
          .list(project.id)
          .filter((c) => !q || (c.title || "").toLowerCase().includes(q)),
      }))
      .filter((group) => group.sessions.length > 0)
  })

  const selectedConversation = () => layout.home.selection().conversationID

  function open(projectID: string, conversationID: string) {
    layout.home.setSelection({ projectID, conversationID })
    void global.sessions.open(projectID, conversationID)
    navigate(`/${projectID}/session/${conversationID}`)
  }

  async function newSession() {
    const projectID = layout.home.selection().projectID ?? global.projects.selected()?.id ?? global.projects.list()[0]?.id
    if (!projectID) return
    const conversation = await global.sessions.new(projectID)
    open(projectID, conversation.id)
  }

  return (
    <section class="flex min-h-0 flex-col gap-3 py-4">
      <header class="flex items-center gap-2">
        <TextField
          class="min-w-0 flex-1"
          variant="ghost"
          placeholder="Search sessions"
          value={query()}
          onInput={(e: InputEvent) => setQuery((e.currentTarget as HTMLInputElement).value)}
        />
        <Button size="small" variant="primary" icon="plus" onClick={() => void newSession()}>
          New
        </Button>
      </header>
      <div class="flex min-h-0 flex-col gap-4 overflow-hidden">
        <For each={groups()}>
          {(group) => (
            <div class="flex flex-col gap-1">
              <div class="px-2 text-[11px] font-medium uppercase tracking-[0.08em] text-v2-text-text-muted">
                {group.project.name}
              </div>
              <For each={group.sessions}>
                {(session) => (
                  <button
                    type="button"
                    data-session-id={session.id}
                    class="flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left hover:bg-v2-background-bg-layer-01 data-[active=true]:bg-v2-background-bg-layer-02"
                    data-active={session.id === selectedConversation()}
                    onClick={() => open(group.project.id, session.id)}
                  >
                    <Icon name="bubble-5" size="small" />
                    <span class="min-w-0 flex-1 truncate text-sm">{session.title || "Session"}</span>
                  </button>
                )}
              </For>
            </div>
          )}
        </For>
        <Show when={!groups().length}>
          <div class="px-2 text-sm text-v2-text-text-muted">
            {query() ? "No sessions match your search." : "No sessions yet. Start one to describe a task."}
          </div>
        </Show>
      </div>
    </section>
  )
}
