// Wayshard terminal panel helpers.
//
// Adapted from the imported OpenCode session helpers
// (third_party/opencode-v1.18.31/packages/app/src/pages/session/helpers.ts):
// focusTerminalById and the numbered terminal-tab label are retained so the
// adapted panel can focus a specific server-owned terminal on selection.
export function focusTerminalById(id: string | undefined): boolean {
  if (!id) return false
  const wrapper = document.getElementById(`terminal-wrapper-${id}`)
  const terminal = wrapper?.querySelector('[data-component="terminal"]')
  if (!(terminal instanceof HTMLElement)) return false

  const textarea = terminal.querySelector("textarea")
  if (textarea instanceof HTMLTextAreaElement) {
    textarea.focus()
    return true
  }

  terminal.focus()
  terminal.dispatchEvent(
    typeof PointerEvent === "function"
      ? new PointerEvent("pointerdown", { bubbles: true, cancelable: true })
      : new MouseEvent("pointerdown", { bubbles: true, cancelable: true }),
  )
  return true
}

// Server PTYs carry no title metadata, so a terminal is labelled by its stable
// ordinal unless the shell reports a title. This preserves the upstream
// label precedence (meaningful title, else numbered) without inventing server
// metadata Wayshard does not store.
export function terminalTabLabel(input: { title?: string; number?: number }): string {
  const title = input.title?.trim() ?? ""
  if (title) return title
  const number = input.number ?? 0
  if (number > 0) return `Terminal ${number}`
  return "Terminal"
}
