// Wayshard TUI keymap. Adapted from the imported OpenCode TUI keymap; the
// binding/mode-stack surface used by the inherited dialog components is kept,
// backed by a simple Wayshard implementation.
import { onCleanup } from "solid-js"
import { useKeyboard } from "@opentui/solid"
import type { KeyEvent } from "@opentui/core"

export interface Binding {
  key: unknown
  desc?: string
  group?: string
  cmd?: unknown
}

export interface BindingsInput {
  enabled?: boolean
  priority?: number
  target?: () => unknown
  bindings?: Binding[]
  commands?: { name: string; title: string; category?: string; run: () => void }[]
}

export function useBindings(input: () => BindingsInput) {
  useKeyboard((key: KeyEvent) => {
    const cfg = input()
    if (cfg.enabled === false) return
    const name = key.name === "return" ? "return" : key.name
    const combo = `${key.ctrl ? "ctrl+" : ""}${key.shift ? "shift+" : ""}${name}`
    for (const b of cfg.bindings ?? []) {
      const bindKey = String(b.key)
      if (bindKey === name || bindKey === combo) {
        if (typeof b.cmd === "function") (b.cmd as () => void)()
        return
      }
    }
  })
}

export function useCommandShortcut(_name: string): () => string {
  return () => ""
}

export function useKeymapSelector<T = unknown>(_selector?: (keymap: KeymapLike) => T): () => any {
  return () => new Map<string, unknown>()
}

export interface KeymapLike {
  getCommandBindings(query?: unknown): Map<string, unknown>
}

export function formatKeyBindings(_bindings?: unknown, _config?: unknown): string {
  return ""
}

export interface ModeStack {
  push(mode: string): () => void
}

export function useWayshardModeStack(): ModeStack {
  return {
    push() {
      return () => {}
    },
  }
}


export { onCleanup }
