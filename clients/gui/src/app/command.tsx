// Wayshard command architecture.
//
// Adapted from the imported OpenCode 2 graphical client command context
// (third_party/opencode-v1.18.31/packages/app/src/context/command.tsx): the
// keybinding parser/matcher/formatter, palette option resolution and command
// registration lifecycle are retained. The command set, i18n and persistence are
// Wayshard.
import { createContext, createMemo, onCleanup, useContext, type Accessor, type JSX } from "solid-js"
import { createStore } from "solid-js/store"
import { makeEventListener } from "@solid-primitives/event-listener"

const IS_MAC = typeof navigator === "object" && /(Mac|iPod|iPhone|iPad)/.test(navigator.platform)

const SUGGESTED_PREFIX = "suggested."

export type KeybindConfig = string

export interface Keybind {
  key: string
  ctrl: boolean
  meta: boolean
  shift: boolean
  alt: boolean
}

export interface CommandOption {
  id: string
  title: string
  description?: string
  category?: string
  keybind?: KeybindConfig
  slash?: string
  suggested?: boolean
  disabled?: boolean
  hidden?: boolean
  when?: (event: KeyboardEvent) => boolean
  onSelect?: (source?: "palette" | "keybind" | "slash") => void
  onHighlight?: () => (() => void) | void
}

export type CommandRegistration = {
  key?: string
  options: Accessor<CommandOption[]>
}

export function commandPaletteOptions(options: CommandOption[]) {
  return options.filter(
    (option) => !option.disabled && !option.hidden && !option.id.startsWith(SUGGESTED_PREFIX) && option.id !== "file.open",
  )
}

export function resolveKeybindOption(candidates: CommandOption[] | undefined, event: KeyboardEvent) {
  return candidates?.find((option) => option.when?.(event)) ?? candidates?.find((option) => !option.when)
}

export function addCommandRegistration(registrations: CommandRegistration[], entry: CommandRegistration) {
  return [entry, ...registrations]
}

export function activeCommandRegistrations(registrations: CommandRegistration[]) {
  const keys = new Set<string>()
  return registrations.filter((entry) => {
    if (entry.key === undefined) return true
    if (keys.has(entry.key)) return false
    keys.add(entry.key)
    return true
  })
}

export function parseKeybind(config: string): Keybind[] {
  if (!config || config === "none") return []
  return config.split(",").map((combo) => {
    const parts = combo.trim().toLowerCase().split("+")
    const keybind: Keybind = { key: "", ctrl: false, meta: false, shift: false, alt: false }
    for (const part of parts) {
      switch (part) {
        case "ctrl":
        case "control":
          keybind.ctrl = true
          break
        case "meta":
        case "cmd":
        case "command":
          keybind.meta = true
          break
        case "mod":
          if (IS_MAC) keybind.meta = true
          else keybind.ctrl = true
          break
        case "alt":
        case "option":
          keybind.alt = true
          break
        case "shift":
          keybind.shift = true
          break
        default:
          keybind.key = part
          break
      }
    }
    return keybind
  })
}

export function normalizeKey(key: string): string {
  return key.toLowerCase()
}

export function matchKeybind(keybinds: Keybind[], event: KeyboardEvent): boolean {
  const eventKey = normalizeKey(event.key)
  for (const kb of keybinds) {
    if (
      kb.key === eventKey &&
      kb.ctrl === (event.ctrlKey || false) &&
      kb.meta === (event.metaKey || false) &&
      kb.shift === (event.shiftKey || false) &&
      kb.alt === (event.altKey || false)
    ) {
      return true
    }
  }
  return false
}

export function displayKeybind(config: KeybindConfig): string {
  return parseKeybind(config)
    .map((kb) => {
      const parts: string[] = []
      if (kb.ctrl) parts.push(IS_MAC ? "⌃" : "Ctrl")
      if (kb.alt) parts.push(IS_MAC ? "⌥" : "Alt")
      if (kb.shift) parts.push(IS_MAC ? "⇧" : "Shift")
      if (kb.meta) parts.push(IS_MAC ? "⌘" : "Meta")
      if (kb.key) parts.push(kb.key.length === 1 ? kb.key.toUpperCase() : kb.key)
      return parts.join(IS_MAC ? "" : "+")
    })
    .join(", ")
}

export interface CommandContext {
  options: Accessor<CommandOption[]>
  register(entry: CommandRegistration): () => void
  run(id: string, source?: "palette" | "keybind" | "slash"): void
  keybind(id: string): KeybindConfig | undefined
}

const ctx = createContext<CommandContext>()

export function CommandProvider(props: { children: JSX.Element; extra?: CommandOption[] }) {
  const [store, setStore] = createStore<{ registrations: CommandRegistration[] }>({ registrations: [] })

  const options = createMemo(() => activeCommandRegistrations(store.registrations).flatMap((entry) => entry.options()))
  const all = createMemo(() => [...(props.extra ?? []), ...options()])

  function run(id: string, source?: "palette" | "keybind" | "slash") {
    all()
      .find((o) => o.id === id)
      ?.onSelect?.(source)
  }

  const value: CommandContext = {
    options: all,
    register(entry) {
      setStore("registrations", (r) => addCommandRegistration(r, entry))
      return () => setStore("registrations", (r) => r.filter((e) => e !== entry))
    },
    run,
    keybind(id) {
      return all().find((o) => o.id === id)?.keybind
    },
  }

  const onKeyDown = (event: KeyboardEvent) => {
    for (const option of all()) {
      if (!option.keybind || option.disabled) continue
      if (matchKeybind(parseKeybind(option.keybind), event)) {
        event.preventDefault()
        option.onSelect?.("keybind")
        return
      }
    }
  }
  const dispose = makeEventListener(window, "keydown", onKeyDown, { capture: true })
  onCleanup(dispose)

  return <ctx.Provider value={value}>{props.children}</ctx.Provider>
}

export function useCommand(): CommandContext {
  const value = useContext(ctx)
  if (!value) throw new Error("useCommand must be used within CommandProvider")
  return value
}
