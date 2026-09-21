// Wayshard TUI locale helpers (adapted from the imported OpenCode TUI util).
export type Locale = string

function truncate(value: string, max: number): string {
  if (value.length <= max) return value
  return value.slice(0, Math.max(1, max - 1)) + "…"
}

export const Locale = {
  titlecase(value: string): string {
    if (!value) return value
    return value.charAt(0).toUpperCase() + value.slice(1)
  },
  truncate,
  truncateLeft(value: string, max: number): string {
    if (value.length <= max) return value
    return "…" + value.slice(value.length - Math.max(1, max - 1))
  },
  truncateMiddle(value: string, max: number): string {
    if (value.length <= max) return value
    const keep = Math.max(1, Math.floor((max - 1) / 2))
    return value.slice(0, keep) + "…" + value.slice(value.length - keep)
  },
}

export function currentLocale(): Locale {
  return process.env.LANG?.split(".")[0] ?? "en"
}

export function formatNumber(value: number): string {
  return new Intl.NumberFormat(currentLocale()).format(value)
}
