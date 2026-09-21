// Wayshard TUI config adapter. Adapted from the imported OpenCode TUI config
// surface; Wayshard keybinds are simple defaults.
export interface TuiConfig {
  cursor?: "block" | "underline" | "line"
  keybinds: { gather(prefix: string, names: string[]): never[] }
}

const config: TuiConfig = {
  cursor: "block",
  keybinds: { gather: () => [] },
}

export function useTuiConfig(): TuiConfig {
  return config
}
