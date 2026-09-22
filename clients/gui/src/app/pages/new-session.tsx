// Wayshard new-session route.
//
// Adapted from the imported OpenCode new-session flow
// (third_party/opencode-v1.18.31/packages/app/src/pages/new-session/new-session-view.tsx):
// the composition is retained — a `session-new-design` surface with centered
// wordmark, prompt composer and a project/status row below it. OpenCode's
// provider/workspace/git controllers are replaced by Wayshard project selection
// and routing profile; the prompt input is the imported PromptInputV2 composer.
import { For, Show, createMemo, createSignal } from "solid-js"
import { useNavigate } from "../router"
import { useGlobal } from "../context/global"
import { useLayout } from "../context/layout"
import { Composer } from "../composer"

export function NewSessionRoute(): import("solid-js").JSX.Element {
  const global = useGlobal()
  const layout = useLayout()
  const navigate = useNavigate()
  const [busy, setBusy] = createSignal(false)

  const projects = global.projects.list
  const selectedID = () =>
    layout.home.selection().projectID ?? global.projects.selected()?.id ?? projects()[0]?.id ?? null
  const selected = createMemo(() => projects().find((p) => p.id === selectedID()))

  async function start(input: { text: string; profile: string; artifactOnly: boolean }) {
    const projectID = selectedID()
    const body = input.text.trim()
    if (!projectID || !body || busy()) return
    setBusy(true)
    try {
      const conversation = await global.sessions.new(projectID)
      await global.client().sendMessage(conversation.id, body, { artifactOnly: input.artifactOnly, profile: input.profile })
      navigate(`/${projectID}/session/${conversation.id}`)
    } finally {
      setBusy(false)
    }
  }

  return (
    <div class="flex h-full min-h-0 flex-1 flex-col">
      <div
        data-component="session-new-design"
        class="relative flex-1 min-h-0 overflow-hidden rounded-[10px] bg-v2-background-bg-deep"
      >
        <div class="absolute inset-x-0 top-[25.375%] flex justify-center px-6">
          <div class="w-full max-w-[720px]">
            <div class="text-center text-2xl font-semibold text-v2-text-text-inverse">Wayshard</div>
            <div class="mt-8 flex flex-col gap-6">
              <Composer onSubmit={(input) => void start(input)} onCancel={() => navigate("/")} />
              <Show
                when={selected()}
                fallback={<div class="text-center text-sm text-v2-text-text-faint">Add a project to start.</div>}
              >
                {(project) => (
                  <div class="flex min-h-7 items-center justify-center gap-3 text-sm text-v2-text-text-faint">
                    <select
                      class="rounded-md border border-v2-border-border-base bg-v2-background-bg-layer-01 px-2 py-1"
                      value={selectedID() ?? ""}
                      onChange={(e) => {
                        const id = e.currentTarget.value
                        layout.home.setSelection({ projectID: id, conversationID: null })
                        void global.projects.select(id)
                      }}
                    >
                      <For each={projects()}>{(p) => <option value={p.id}>{p.name}</option>}</For>
                    </select>
                    <span class="max-w-[360px] truncate">{project().path}</span>
                  </div>
                )}
              </Show>
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}
