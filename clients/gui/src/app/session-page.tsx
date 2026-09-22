// Wayshard session page.
//
// Adapted from the imported OpenCode application session page
// (third_party/opencode-v1.18.31/packages/app/src/pages/session.tsx): the
// composition is retained — a titlebar with the work-surface tab strip, a
// session content region, and a docked composer region at the bottom — with the
// Wayshard primary surfaces (Session/Changes/Files/Terminal) and advanced
// surfaces. The message presentation is the adapted session-ui; the domain and
// data path are Wayshard. Replaces the retired custom shell.
import { For, Show, createMemo, onCleanup, onMount, type JSX } from "solid-js"
import { Button } from "@wayshard/ui/button"
import { Dialog } from "@wayshard/ui/dialog"
import { useDialog } from "@wayshard/ui/context/dialog"
import { DataProvider } from "@wayshard/gui/session-ui/context"
import { useWayshard } from "../wayshard/state"
import { buildData } from "../wayshard/adapter"
import { useCommand } from "./command"
import { CommandPalette } from "./command-palette"
import { AdvancedSurface, type AdvancedSurfaceKey } from "./views"
import { Composer } from "./composer"
import { ComposerRegion } from "./components/composer-region"
import { Titlebar } from "./components/titlebar"
import { SessionReviewTab } from "./pages/session/review-tab"
import { SessionFileTabs } from "./pages/session/file-tabs"
import { SessionTerminalPanel } from "./pages/session/terminal-panel-v2"
import { SessionTimeline } from "./pages/session/timeline/message-timeline"
import { ADVANCED_SURFACES, PRIMARY_TABS, activeTab, setActiveTab, setPairingOpen, type PrimaryTab } from "./navigation"

export function FileFallback(props: { path?: string; content?: string }) {
  return <pre class="wh-file-view">{props.content ?? ""}</pre>
}

export function SessionPage(): JSX.Element {
  const ws = useWayshard()
  const command = useCommand()
  const dialog = useDialog()

  function openPalette() {
    dialog.show(() => <CommandPalette />)
  }

  function openAdvanced(key: AdvancedSurfaceKey) {
    dialog.show(() => (
      <Dialog title={ADVANCED_SURFACES.find((s) => s.key === key)?.label ?? key} size="x-large">
        <div class="wh-dialog-surface">
          <AdvancedSurface view={key} />
        </div>
      </Dialog>
    ))
  }

  function openMore() {
    dialog.show(() => (
      <Dialog title="More surfaces" size="large">
        <div class="wh-more-grid">
          <For each={ADVANCED_SURFACES}>
            {(s) => (
              <button class="wh-more-item" onClick={() => openAdvanced(s.key)}>
                {s.label}
              </button>
            )}
          </For>
        </div>
      </Dialog>
    ))
  }

  onMount(() => {
    const dispose = command.register({
      options: () => [
        { id: "command.palette", title: "Command palette", category: "Navigation", keybind: "mod+k", onSelect: openPalette },
        { id: "surface.more", title: "More surfaces…", category: "Navigation", keybind: "mod+shift+m", onSelect: openMore },
        ...PRIMARY_TABS.map((t) => ({
          id: `tab.${t.key}`,
          title: `Go to ${t.label}`,
          category: "Navigation",
          keybind: t.keybind,
          onSelect: () => setActiveTab(t.key),
        })),
        ...ADVANCED_SURFACES.map((s) => ({
          id: `surface.${s.key}`,
          title: s.label,
          category: "Surfaces",
          onSelect: () => openAdvanced(s.key),
        })),
        {
          id: "project.open",
          title: "Open project…",
          category: "Project",
          onSelect: async () => {
            const path = window.prompt("Project path")
            if (path) await ws.openProject(path)
          },
        },
        { id: "connect.pair", title: "Connect / pair device…", category: "Connection", onSelect: () => setPairingOpen(true) },
        { id: "session.new", title: "New session", category: "Session", onSelect: () => void ws.newConversation() },
        { id: "run.cancel", title: "Cancel run", category: "Run", onSelect: () => void ws.cancelRun() },
        { id: "run.retry", title: "Retry run", category: "Run", onSelect: () => void ws.retryRun() },
        { id: "run.integrate", title: "Integrate run", category: "Run", onSelect: () => void ws.integrateRun() },
      ],
    })
    onCleanup(dispose)
  })

  const data = createMemo(() =>
    buildData({
      project: ws.activeProject(),
      conversations: ws.state.conversations,
      activeConversationID: ws.state.activeConversationID,
      messages: ws.state.messages,
      runs: ws.state.runsByConversation,
      runView: ws.state.run ? { run: ws.state.run, stages: ws.state.stages } : undefined,
    }),
  )

  return (
    <div data-component="session" class="flex h-full min-h-0 flex-col bg-v2-background-bg-base">
      <Titlebar onMore={openMore} onPalette={openPalette} />
      <Show when={!ws.state.connected}>
        <div class="wh-connection" data-state="disconnected">
          <span>{ws.state.connectionError ? `Disconnected: ${ws.state.connectionError}` : "Connecting to Wayshard server…"}</span>
          <Button size="small" variant="secondary" onClick={() => void ws.refreshAll()}>
            Reconnect
          </Button>
        </div>
      </Show>
      <div data-slot="session-content" class="flex min-h-0 flex-1 flex-col">
        <Show when={activeTab() === "session"} fallback={<PrimaryView tab={activeTab()} />}>
          <DataProvider data={data()} directory={ws.activeProject()?.path ?? ""} sessionID={ws.state.activeConversationID ?? undefined}>
            <SessionView />
          </DataProvider>
        </Show>
      </div>
    </div>
  )
}

function PrimaryView(props: { tab: PrimaryTab }) {
  return (
    <Show when={props.tab === "changes"} fallback={<Show when={props.tab === "files"} fallback={<SessionTerminalPanel />}><SessionFileTabs /></Show>}>
      <SessionReviewTab />
    </Show>
  )
}

function SessionView() {
  const ws = useWayshard()
  return (
    <div data-slot="session-region" class="wh-session">
      <SessionTimeline />
      <ComposerRegion
        promptInput={
          <Composer
            onSubmit={(input) => void ws.send(input.text, { artifactOnly: input.artifactOnly, profile: input.profile })}
            onCancel={() => void ws.cancelRun()}
          />
        }
      />
    </div>
  )
}
