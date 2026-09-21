// Wayshard TUI theme. Reuses the imported OpenCode TUI theme system and theme
// assets (Wayshard-owned copy); no OpenCode runtime dependency.
import { DEFAULT_THEMES, resolveTheme, tint, type Theme } from "./theme/index"

export type { Theme }
export { tint }

export function loadTheme(name = process.env.WAYSHARD_TUI_THEME ?? "tokyonight"): Theme {
  const json = DEFAULT_THEMES[name] ?? DEFAULT_THEMES["tokyonight"] ?? Object.values(DEFAULT_THEMES)[0]
  return resolveTheme(json, "dark")
}
