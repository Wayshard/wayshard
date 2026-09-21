// Wayshard TUI theme context. Adapted from the imported OpenCode TUI theme
// context; backed by the imported theme system/assets copied into ./theme.
import { createContext, useContext, type JSX } from "solid-js"
import { createStore } from "solid-js/store"
import { RGBA } from "@opentui/core"
import { DEFAULT_THEMES, resolveTheme, tint, type Theme } from "../theme/index"

export type { Theme }
export { tint }

interface ThemeContextValue {
  theme: Theme
  setTheme(name: string): void
}

const ThemeContext = createContext<ThemeContextValue>()

export function ThemeProvider(props: { name?: string; children: JSX.Element }) {
  const [store, setStore] = createStore<{ name: string }>({ name: props.name ?? process.env.WAYSHARD_TUI_THEME ?? "tokyonight" })
  const value: ThemeContextValue = {
    get theme() {
      const json = DEFAULT_THEMES[store.name] ?? DEFAULT_THEMES["tokyonight"] ?? Object.values(DEFAULT_THEMES)[0]
      return resolveTheme(json, "dark")
    },
    setTheme(name: string) {
      if (DEFAULT_THEMES[name]) setStore("name", name)
    },
  }
  return <ThemeContext.Provider value={value}>{props.children}</ThemeContext.Provider>
}

export function useTheme() {
  const ctx = useContext(ThemeContext)
  if (!ctx) throw new Error("useTheme must be used within ThemeProvider")
  return ctx
}

export { RGBA }
