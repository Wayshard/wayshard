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
import { Show, onCleanup, onMount } from "solid-js"
import { ThemeProvider } from "@wayshard/ui/theme/context"
import { DialogProvider } from "@wayshard/ui/context/dialog"
import { FileComponentProvider } from "@wayshard/ui/context/file"
import { StateProvider, useWayshard } from "../wayshard/state"
import { GlobalProvider } from "./context/global"
import { LayoutProvider } from "./context/layout"
import { CommandProvider } from "./command"
import { FileFallback } from "./session-page"
import { PairingGate } from "./pairing"
import { syncRouter, useRouteMatch } from "./router"
import { Home } from "./pages/home"
import { AppLayout } from "./pages/layout"
import { SessionRoute } from "./pages/session-route"
import { NewSessionRoute } from "./pages/new-session"
import { pairingOpen } from "./navigation"

function Routes() {
  const match = useRouteMatch()
  return (
    <>
      <Show when={match().name === "session"}>
        <AppLayout>
          <SessionRoute />
        </AppLayout>
      </Show>
      <Show when={match().name === "new-session"}>
        <AppLayout>
          <NewSessionRoute />
        </AppLayout>
      </Show>
      <Show when={match().name === "home"}>
        <AppLayout>
          <Home />
        </AppLayout>
      </Show>
    </>
  )
}

function PairingOverlay() {
  const ws = useWayshard()
  const needsPairing = () => pairingOpen() || (!ws.state.connected && !ws.state.connection.token)
  return (
    <Show when={needsPairing()}>
      <PairingGate />
    </Show>
  )
}

function RouterLifecycle() {
  onMount(() => {
    const onPop = () => syncRouter()
    window.addEventListener("popstate", onPop)
    onCleanup(() => window.removeEventListener("popstate", onPop))
  })
  return null
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
                  <RouterLifecycle />
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
