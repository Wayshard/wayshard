import { registerCustomTheme } from "@pierre/diffs"
import { WayshardTheme } from "./marked-theme"

let registered = false

export function registerWayshardTheme() {
  if (registered) return
  registered = true
  registerCustomTheme("Wayshard", () => Promise.resolve(WayshardTheme))
}
