// Wayshard TUI locale helpers (adapted from the imported OpenCode TUI util).
export type Locale = string

export const Locale = {
  titlecase(value: string): string {
    if (!value) return value
    return value.charAt(0).toUpperCase() + value.slice(1)
  },
}

export function currentLocale(): Locale {
  return process.env.LANG?.split(".")[0] ?? "en"
}

export function formatNumber(value: number): string {
  return new Intl.NumberFormat(currentLocale()).format(value)
}
