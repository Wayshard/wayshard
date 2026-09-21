// Wayshard command palette, adapted from the imported OpenCode command
// architecture: keyboard-first navigation over Wayshard actions.
import { For, Show, createMemo, createSignal } from "solid-js"
import { Dialog } from "@wayshard/ui/dialog"
import { TextField } from "@wayshard/ui/text-field"
import { useWayshard } from "../wayshard/state"
import { NAV, setActiveView, type ViewKey } from "./App"

interface Command {
  id: string
  title: string
  run?: () => void | Promise<void>
}

export function CommandPalette(props: { open: boolean; onClose: () => void }) {
  const ws = useWayshard()
  const [query, setQuery] = createSignal("")

  const commands = createMemo<Command[]>(() => {
    const list: Command[] = NAV.map((n) => ({
      id: `view:${n.key}`,
      title: `Go to ${n.label}`,
      run: () => { setActiveView(n.key as ViewKey) },
    }))
    list.push(
      {
        id: "project:open",
        title: "Open project…",
        run: async () => {
          const path = window.prompt("Project path")
          if (path) await ws.openProject(path)
        },
      },
      { id: "session:new", title: "New session", run: () => ws.newConversation() },
      { id: "run:cancel", title: "Cancel run", run: () => ws.cancelRun() },
      { id: "run:retry", title: "Retry run", run: () => ws.retryRun() },
      { id: "run:integrate", title: "Integrate run", run: () => ws.integrateRun() },
      { id: "focus:composer", title: "Focus composer", run: () => { setActiveView("session") } },
    )
    return list
  })

  const filtered = createMemo(() => {
    const q = query().toLowerCase()
    if (!q) return commands()
    return commands().filter((c) => c.title.toLowerCase().includes(q))
  })

  async function exec(cmd: Command) {
    await cmd.run?.()
    props.onClose()
  }

  return (
    <Show when={props.open}>
      <Dialog title="Command palette" size="large">
        <TextField
          autofocus
          placeholder="Type a command…"
          value={query()}
          onInput={(e: InputEvent) => setQuery((e.currentTarget as HTMLInputElement).value)}
          onKeyDown={(e: KeyboardEvent) => {
            if (e.key === "Escape") props.onClose()
            if (e.key === "Enter" && filtered().length) void exec(filtered()[0])
          }}
        />
        <ul class="wh-command-list">
          <For each={filtered()}>
            {(c) => (
              <li>
                <button class="wh-nav-item" onClick={() => void exec(c)}>
                  {c.title}
                </button>
              </li>
            )}
          </For>
        </ul>
      </Dialog>
    </Show>
  )
}
