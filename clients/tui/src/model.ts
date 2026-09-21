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
