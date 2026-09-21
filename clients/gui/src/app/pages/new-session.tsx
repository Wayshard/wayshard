// Wayshard new-session route.
//
// Adapted from the imported OpenCode new-session flow
// (third_party/opencode-v1.18.31/packages/app/src/pages/new-session.tsx and
// pages/new-session/new-session-view.tsx): the project selection, task input and
// start affordance are retained; OpenCode model/provider/workspace selection is
// replaced by a Wayshard routing profile and artifact-only option. The full
// inherited layout is completed in a later stage of this port.
import { For, Show, createMemo, createSignal } from "solid-js"
import { useNavigate } from "../router"
import { Button } from "@wayshard/ui/button"
import { TextField } from "@wayshard/ui/text-field"
import { useGlobal } from "../context/global"
import { useLayout } from "../context/layout"

export function NewSessionRoute() {
  const global = useGlobal()
  const layout = useLayout()
  const navigate = useNavigate()
  const [text, setText] = createSignal("")
  const [profile, setProfile] = createSignal("auto")
  const [artifactOnly, setArtifactOnly] = createSignal(false)
  const [busy, setBusy] = createSignal(false)

  const projects = global.projects.list
  const selectedID = () => layout.home.selection().projectID ?? global.projects.selected()?.id ?? projects()[0]?.id ?? null
  const selected = createMemo(() => projects().find((p) => p.id === selectedID()))

  async function start() {
    const projectID = selectedID()
    const body = text().trim()
    if (!projectID || !body || busy()) return
    setBusy(true)
    try {
      const conversation = await global.sessions.new(projectID)
      await global.client().sendMessage(conversation.id, body, { artifactOnly: artifactOnly(), profile: profile() })
      navigate(`/${projectID}/session/${conversation.id}`)
    } finally {
      setBusy(false)
    }
  }

  return (
    <div class="m-2 min-h-0 flex-1 self-stretch overflow-hidden rounded-[10px] bg-v2-background-bg-base shadow-[var(--v2-elevation-raised)]">
      <div class="mx-auto flex h-full w-full max-w-[720px] flex-col gap-4 px-6 py-8">
        <h1 class="text-lg font-medium">New session</h1>
        <div class="flex flex-col gap-1">
          <span class="text-[11px] font-medium uppercase tracking-[0.08em] text-v2-text-text-muted">Project</span>
          <select
            class="rounded-md border border-v2-border-border-base bg-v2-background-bg-layer-01 px-2 py-1.5 text-sm"
            value={selectedID() ?? ""}
            onChange={(e) => {
              const id = e.currentTarget.value
              layout.home.setSelection({ projectID: id, conversationID: null })
              void global.projects.select(id)
            }}
          >
            <For each={projects()}>{(p) => <option value={p.id}>{p.name}</option>}</For>
          </select>
        </div>
        <TextField
          multiline
          class="min-h-[120px]"
          placeholder="Describe a task…"
          value={text()}
          onInput={(e: InputEvent) => setText((e.currentTarget as HTMLTextAreaElement).value)}
        />
        <div class="flex items-center gap-3">
          <label class="flex items-center gap-2 text-sm">
            <input type="checkbox" checked={artifactOnly()} onChange={(e) => setArtifactOnly(e.currentTarget.checked)} />
            Artifact only
          </label>
          <label class="flex items-center gap-2 text-sm">
            <span class="text-v2-text-text-muted">Profile</span>
            <select class="rounded-md border border-v2-border-border-base bg-v2-background-bg-layer-01 px-2 py-1" value={profile()} onChange={(e) => setProfile(e.currentTarget.value)}>
              <option value="auto">Auto</option>
              <option value="quality">Quality</option>
              <option value="speed">Speed</option>
              <option value="economy">Economy</option>
            </select>
          </label>
          <div class="flex-1" />
          <Button size="small" variant="secondary" onClick={() => navigate("/")}>
            Cancel
          </Button>
          <Button size="small" variant="primary" icon="arrow-up" disabled={busy() || !selected()} onClick={() => void start()}>
            Start session
          </Button>
        </div>
        <Show when={!projects().length}>
          <div class="text-sm text-v2-text-text-muted">Add a project on Home before starting a session.</div>
        </Show>
      </div>
    </div>
  )
}
