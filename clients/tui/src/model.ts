// Pure Wayshard TUI view-model helpers (extracted so they are testable without
// a terminal).
export const STAGE_LABELS: Record<string, string> = {
  plan: "Planning",
  explore: "Exploring",
  execute: "Executing",
  validate: "Validating",
  review: "Reviewing",
  repair: "Repairing",
  replan: "Replanning",
  integrate: "Integrating",
}

export function stageDisplay(kind: string): string {
  return STAGE_LABELS[kind] ?? kind.charAt(0).toUpperCase() + kind.slice(1)
}

export interface PaletteCommand {
  id: string
  title: string
}

export function filterCommands<T extends PaletteCommand>(commands: T[], query: string): T[] {
  const q = query.trim().toLowerCase()
  if (!q) return commands
  return commands.filter((c) => c.title.toLowerCase().includes(q))
}

export type TuiTab = "session" | "changes" | "files" | "routing" | "usage" | "approvals" | "settings"

export const TUI_TABS: TuiTab[] = ["session", "changes", "files", "routing", "usage", "approvals", "settings"]

export function cycleTab(current: TuiTab, delta: number): TuiTab {
  const index = TUI_TABS.indexOf(current)
  return TUI_TABS[(index + delta + TUI_TABS.length) % TUI_TABS.length]
}

export interface TuiCommand {
  id: string
  title: string
  category: string
}

export function paletteOptions(commands: TuiCommand[], query: string): TuiCommand[] {
  const filtered = filterCommands(commands, query)
  return filtered.map((c) => ({ ...c }))
}
