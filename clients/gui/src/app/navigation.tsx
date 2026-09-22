// Wayshard application navigation model.
//
// Extracted from the retired custom shell so the adapted titlebar and session
// page share the primary work surfaces and advanced Wayshard surfaces. The
// primary surfaces (Session/Changes/Files/Terminal) and the advanced surfaces
// are Wayshard product requirements placed into the adapted OpenCode titlebar /
// dialog / palette composition.
import { createSignal } from "solid-js"
import type { AdvancedSurfaceKey } from "./views"

export type PrimaryTab = "session" | "changes" | "files" | "terminal"

export const PRIMARY_TABS: { key: PrimaryTab; label: string; icon: string; keybind: string }[] = [
  { key: "session", label: "Session", icon: "prompt", keybind: "mod+1" },
  { key: "changes", label: "Changes", icon: "diff", keybind: "mod+2" },
  { key: "files", label: "Files", icon: "bullet-list", keybind: "mod+3" },
  { key: "terminal", label: "Terminal", icon: "terminal", keybind: "mod+4" },
]

export const ADVANCED_SURFACES: { key: AdvancedSurfaceKey; label: string }[] = [
  { key: "usage", label: "Usage" },
  { key: "routing", label: "Routing" },
  { key: "context", label: "Context" },
  { key: "artifacts", label: "Artifacts" },
  { key: "knowledge", label: "Project knowledge" },
  { key: "harnesses", label: "Harnesses" },
  { key: "recovery", label: "Recovery & diagnostics" },
  { key: "approvals", label: "Approvals" },
  { key: "notifications", label: "Notifications" },
  { key: "settings", label: "Settings" },
]

const [activeTab, setActiveTab] = createSignal<PrimaryTab>("session")
const [pairingOpen, setPairingOpen] = createSignal(false)

export { activeTab, setActiveTab, pairingOpen, setPairingOpen }
