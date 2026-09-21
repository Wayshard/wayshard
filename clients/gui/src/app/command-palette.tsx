// Wayshard command palette.
//
// Adapted from the imported OpenCode 2 graphical client command palette dialog
// (third_party/opencode-v1.18.31/packages/app/src/components/dialog-command-palette-v2.tsx):
// the search input, grouped option list, keybind hint and highlight/select
// behaviour are retained. The command set is Wayshard.
import { For, Show, createMemo, createSignal, onMount } from "solid-js"
import { Dialog } from "@wayshard/ui/dialog"
import { useDialog } from "@wayshard/ui/context/dialog"
import { TextField } from "@wayshard/ui/text-field"
import { ScrollView } from "@wayshard/ui/scroll-view"
import { commandPaletteOptions, displayKeybind, useCommand } from "./command"

export function CommandPalette() {
  const command = useCommand()
  const dialog = useDialog()
  const [query, setQuery] = createSignal("")
  const [active, setActive] = createSignal(0)

  const options = createMemo(() => {
    const all = commandPaletteOptions(command.options())
    const q = query().trim().toLowerCase()
    const filtered = q
      ? all.filter((o) => o.title.toLowerCase().includes(q) || (o.category ?? "").toLowerCase().includes(q))
      : all
    return filtered.slice(0, 100)
  })

  onMount(() => setActive(0))

  function select(index: number) {
    const option = options()[index]
    if (!option || option.disabled) return
    option.onSelect?.("palette")
    dialog.close()
  }

  return (
    <Dialog title="Command palette" size="large">
      <div class="wh-palette">
        <TextField
          autofocus
          placeholder="Type a command…"
          value={query()}
          onInput={(e: InputEvent) => {
            setQuery((e.currentTarget as HTMLInputElement).value)
            setActive(0)
          }}
          onKeyDown={(e: KeyboardEvent) => {
            if (e.key === "ArrowDown") {
              e.preventDefault()
              setActive((i) => Math.min(i + 1, options().length - 1))
            }
            if (e.key === "ArrowUp") {
              e.preventDefault()
              setActive((i) => Math.max(i - 1, 0))
            }
            if (e.key === "Enter") {
              e.preventDefault()
              select(active())
            }
          }}
        />
        <ScrollView class="wh-palette-list">
          <Show when={options().length} fallback={<div class="wh-muted">No matching commands</div>}>
            <For each={options()}>
              {(option, index) => (
                <button
                  class="wh-palette-item"
                  data-active={index() === active()}
                  onClick={() => select(index())}
                  onMouseEnter={() => setActive(index())}
                >
                  <span class="wh-palette-title">{option.title}</span>
                  <Show when={option.category}>
                    <span class="wh-muted">{option.category}</span>
                  </Show>
                  <Show when={option.keybind}>
                    <span class="wh-keybind">{displayKeybind(option.keybind!)}</span>
                  </Show>
                </button>
              )}
            </For>
          </Show>
        </ScrollView>
      </div>
    </Dialog>
  )
}
