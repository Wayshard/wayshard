// Wayshard Home projects column.
//
// Adapted from the imported OpenCode application projects view
// (third_party/opencode-v1.18.31/packages/app/src/pages/home/home-projects-view.tsx
// and home-projects-controller.tsx): the projects section, per-project
// selection, session counts and add/new-session affordances are retained; the
// OpenCode multi-server/workspace/WSL model is replaced by the Wayshard
// single-server Project model.
import { For, Show, onMount } from "solid-js"
import { useNavigate } from "../../router"
import { Button } from "@wayshard/ui/button"
import { Icon } from "@wayshard/ui/icon"
import { useGlobal } from "../../context/global"
import { useLayout } from "../../context/layout"

export function HomeProjects() {
  const global = useGlobal()
  const layout = useLayout()
  const navigate = useNavigate()

  const projects = global.projects.list
  const selectedID = () => layout.home.selection().projectID ?? global.projects.selected()?.id ?? null

  onMount(() => {
    const first = selectedID() ?? projects()[0]?.id ?? null
    if (first) void select(first, false)
  })

  async function select(id: string, go = true) {
    layout.home.setSelection({ projectID: id, conversationID: null })
    await global.projects.select(id)
    await global.sessions.load(id)
    if (go) return
  }

  async function addProject() {
    const path = window.prompt("Project path")
    if (!path) return
    const project = await global.projects.open(path)
    await select(project.id, false)
  }

  async function newSession() {
    const id = selectedID()
    if (!id) return
    const conversation = await global.sessions.new(id)
    navigate(`/${id}/session/${conversation.id}`)
  }

  return (
    <section class="flex min-h-0 flex-col gap-2 py-4">
      <header class="flex items-center justify-between px-2">
        <span class="text-[11px] font-medium uppercase tracking-[0.08em] text-v2-text-text-muted">Projects</span>
        <Button size="small" variant="ghost" icon="folder-add-left" onClick={() => void addProject()}>
          Add
        </Button>
      </header>
      <div class="flex min-h-0 flex-col gap-1">
        <For each={projects()}>
          {(project) => (
            <button
              type="button"
              data-project-id={project.id}
              class="flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left hover:bg-v2-background-bg-layer-01 data-[active=true]:bg-v2-background-bg-layer-02"
              data-active={project.id === selectedID()}
              onClick={() => void select(project.id)}
            >
              <Icon name="bullet-list" size="small" />
              <span class="min-w-0 flex-1 truncate text-sm">{project.name}</span>
              <Show when={global.sessions.count(project.id)}>
                <span class="text-[11px] text-v2-text-text-muted">{global.sessions.count(project.id)}</span>
              </Show>
            </button>
          )}
        </For>
        <Show when={!projects().length}>
          <div class="px-2 text-sm text-v2-text-text-muted">No projects yet. Add one to get started.</div>
        </Show>
      </div>
      <Show when={selectedID()}>
        <Button size="small" variant="secondary" icon="plus" onClick={() => void newSession()}>
          New session
        </Button>
      </Show>
    </section>
  )
}
