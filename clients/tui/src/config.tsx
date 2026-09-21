// Wayshard TUI config adapter. Adapted from the imported OpenCode TUI config
// surface; Wayshard keybinds are simple defaults.
export interface TuiConfig {
  cursor?: "block" | "underline" | "line"
  keybinds: { gather(prefix: string, names: string[]): never[]; get(name: string): never[] }
  scrollAcceleration?: unknown
  scroll?: unknown
}

const config: TuiConfig = {
  cursor: "block",
  keybinds: { gather: () => [], get: () => [] },
}

export function useTuiConfig(): TuiConfig {
  return config
}
