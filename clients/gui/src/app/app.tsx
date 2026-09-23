// Wayshard graphical application root.
//
// Adapted from the imported OpenCode application root
// (third_party/opencode-v1.18.31/packages/app/src/app.tsx): the application-level
// provider tree and route structure are retained (theme, state/data, layout,
// command, dialog, file component providers; Home / Session / New Session
// routes) while every OpenCode server/session/provider context is replaced by a
// Wayshard context backed by @wayshard/sdk. Routing uses the adapted Wayshard
// router (see ./router.tsx). Web, Desktop (Tauri 2) and Android (Tauri 2) all
// mount this same application.
import { createMemo } from "solid-js"
import { Dynamic } from "solid-js/web"
import { ThemeProvider } from "@wayshard/ui/theme/context"
import { DialogProvider } from "@wayshard/ui/context/dialog"
import { FileComponentProvider } from "@wayshard/ui/context/file"
import { StateProvider, useWayshard } from "../wayshard/state"
import { GlobalProvider } from "./context/global"
import { LayoutProvider } from "./context/layout"
import { CommandProvider } from "./command"
import { FileFallback } from "./session-page"
import { PairingGate } from "./pairing"
import { useRouteMatch } from "./router"
import { Home } from "./pages/home"
import { AppLayout } from "./pages/layout"
import { SessionRoute } from "./pages/session-route"
import { NewSessionRoute } from "./pages/new-session"
import { pairingOpen } from "./navigation"

function Routes() {
  const match = useRouteMatch()
  // Route selection is resolved as a plain memo and swapped via Dynamic. The
  // adapted provider tree did not propagate the router signal through top-level
  // Show/Switch children, so the selected view is computed directly here.
  const view = createMemo(() => {
    const name = match().name
    if (name === "session") return SessionRoute
    if (name === "new-session") return NewSessionRoute
    return Home
  })
  return (
    <AppLayout>
      <Dynamic component={view()} />
    </AppLayout>
  )
}

function PairingOverlay() {
  const ws = useWayshard()
  // The pairing gate is always mounted and revealed with a reactive inline
  // style: attribute/style reactivity works reliably in this adapted provider
  // tree, while conditional children in this position do not update. The gate
  // shows when the operator opens it or when the server rejected an
  // authenticated request (missing/expired session cookie or stale device
  // credential).
  const active = () => pairingOpen() || ws.state.authRequired
  return (
    <div data-component="pairing-overlay" style={{ display: active() ? "contents" : "none" }}>
      <PairingGate />
    </div>
  )
}

export function WayshardApp() {
  return (
    <ThemeProvider defaultTheme="oc-2" defaultColorScheme="dark">
      <StateProvider>
        <GlobalProvider>
          <LayoutProvider>
            <CommandProvider>
              <DialogProvider>
                <FileComponentProvider component={FileFallback}>
                  <Routes />
                  <PairingOverlay />
                </FileComponentProvider>
              </DialogProvider>
            </CommandProvider>
          </LayoutProvider>
        </GlobalProvider>
      </StateProvider>
    </ThemeProvider>
  )
}
