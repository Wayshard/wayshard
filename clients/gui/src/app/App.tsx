// Wayshard shared graphical client.
//
// The application shell, navigation hierarchy, command architecture, layout,
// session/file tabs and palette are adapted from the imported OpenCode 2
// graphical client (third_party/opencode-v1.18.31/packages/app/src). The design
// system is @wayshard/ui; the session presentation is the adapted session-ui.
// The domain, state and backend are Wayshard (@wayshard/sdk). Web, Desktop
// (Tauri 2) and Android (Tauri 2) all mount this same application.
import { For, Show, createMemo, createSignal, onCleanup, onMount, type JSX } from "solid-js"
import { Button } from "@wayshard/ui/button"
import { Icon } from "@wayshard/ui/icon"
import { Tag } from "@wayshard/ui/tag"
import { Spinner } from "@wayshard/ui/spinner"
import { Dialog } from "@wayshard/ui/dialog"
import { DialogProvider, useDialog } from "@wayshard/ui/context/dialog"
import { FileComponentProvider } from "@wayshard/ui/context/file"
import { DataProvider } from "@wayshard/gui/session-ui/context"
import { SessionTurn } from "@wayshard/gui/session-ui/components/session-turn"
import { StateProvider, useWayshard } from "../wayshard/state"
import { buildData, stageDisplay } from "../wayshard/adapter"
import { CommandProvider, useCommand } from "./command"
import { AppLayout } from "./layout"
import { CommandPalette } from "./command-palette"
import { SessionTab } from "./session-tab"
import { ChangesView, FilesView, TerminalView, AdvancedSurface, type AdvancedSurfaceKey } from "./views"

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

export { activeTab, setActiveTab }

function FileFallback(props: { path?: string; content?: string }) {
  return <pre class="wh-file-view">{props.content ?? ""}</pre>
}

function Shell() {
  const ws = useWayshard()
  const command = useCommand()
  const dialog = useDialog()
  const [moreOpen, setMoreOpen] = createSignal(false)

  function openPalette() {
    dialog.show(() => <CommandPalette />)
  }

  function openAdvanced(key: AdvancedSurfaceKey) {
    setMoreOpen(false)
    dialog.show(() => (
      <Dialog title={ADVANCED_SURFACES.find((s) => s.key === key)?.label ?? key} size="x-large">
        <div class="wh-dialog-surface">
          <AdvancedSurface view={key} />
        </div>
      </Dialog>
    ))
  }

  onMount(() => {
    const dispose = command.register({
      options: () => [
        { id: "command.palette", title: "Command palette", category: "Navigation", keybind: "mod+k", onSelect: openPalette },
        { id: "surface.more", title: "More surfaces…", category: "Navigation", keybind: "mod+shift+m", onSelect: () => setMoreOpen(true) },
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
    <AppLayout
      titlebar={
        <header class="wh-titlebar">
          <div class="wh-titlebar-left">
            <span class="wh-wordmark">Wayshard</span>
            <span class="wh-truncate">{ws.activeProject()?.name ?? "No project"}</span>
          </div>
          <nav class="wh-tabs" role="tablist">
            <For each={PRIMARY_TABS}>
              {(tab) => (
                <button class="wh-tab-button" role="tab" aria-selected={activeTab() === tab.key} data-active={activeTab() === tab.key} onClick={() => setActiveTab(tab.key)}>
                  <Icon name={tab.icon as never} size="small" />
                  {tab.label}
                </button>
              )}
            </For>
            <button class="wh-tab-button" data-active={moreOpen()} onClick={() => setMoreOpen(true)}>
              <Icon name="bullet-list" size="small" />
              More
            </button>
          </nav>
          <div class="wh-titlebar-right">
            <Show when={ws.state.run}>
              <Tag>{ws.state.run!.status}</Tag>
            </Show>
            <Button size="small" variant="ghost" icon="console" onClick={openPalette}>
              ⌘K
            </Button>
          </div>
        </header>
      }
    >
      <Show when={!ws.state.connected}>
        <div class="wh-connection" data-state="disconnected">
          <span>{ws.state.connectionError ? `Disconnected: ${ws.state.connectionError}` : "Connecting to Wayshard server…"}</span>
          <Button size="small" variant="secondary" onClick={() => void ws.refreshAll()}>
            Reconnect
          </Button>
        </div>
      </Show>
      <div class="wh-body">
        <Sidebar />
        <div class="wh-content">
          <Show when={activeTab() === "session"} fallback={<PrimaryView tab={activeTab()} />}>
            <DataProvider data={data()} directory={ws.activeProject()?.path ?? ""} sessionID={ws.state.activeConversationID ?? undefined}>
              <SessionView />
            </DataProvider>
          </Show>
        </div>
      </div>
      <Show when={moreOpen()}>
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
      </Show>
    </AppLayout>
  )
}

function PrimaryView(props: { tab: PrimaryTab }) {
  return (
    <Show when={props.tab === "changes"} fallback={<Show when={props.tab === "files"} fallback={<TerminalView />}><FilesView /></Show>}>
      <ChangesView />
    </Show>
  )
}

function Sidebar() {
  const ws = useWayshard()
  return (
    <aside class="wh-sidebar">
      <div class="wh-sidebar-section">
        <div class="wh-sidebar-label">Projects</div>
        <Show when={ws.state.projects.length} fallback={<div class="wh-empty">No projects</div>}>
          <For each={ws.state.projects}>
            {(p) => (
              <button class="wh-nav-item" data-active={p.id === ws.state.activeProjectID} onClick={() => void ws.selectProject(p.id)}>
                <Icon name="bullet-list" size="small" />
                <span class="wh-truncate">{p.name}</span>
              </button>
            )}
          </For>
        </Show>
      </div>
      <div class="wh-sidebar-section">
        <div class="wh-sidebar-label">Sessions</div>
        <Show when={ws.state.conversations.length} fallback={<div class="wh-empty">No sessions</div>}>
          <For each={ws.state.conversations}>
            {(c) => (
              <button class="wh-nav-item" data-active={c.id === ws.state.activeConversationID} onClick={() => void ws.selectConversation(c.id)}>
                <Icon name="bubble-5" size="small" />
                <span class="wh-truncate">{c.title || "Session"}</span>
              </button>
            )}
          </For>
        </Show>
        <Button size="small" variant="ghost" icon="arrow-right" onClick={() => void ws.newConversation()}>
          New session
        </Button>
      </div>
    </aside>
  )
}

function SessionView() {
  const ws = useWayshard()
  const [draft, setDraft] = createSignal("")
  const [artifactOnly, setArtifactOnly] = createSignal(false)
  const [profile, setProfile] = createSignal("auto")

  const userMessages = createMemo(() => ws.state.messages.filter((m) => m.role === "user"))

  async function send() {
    const text = draft().trim()
    if (!text) return
    setDraft("")
    await ws.send(text, { artifactOnly: artifactOnly(), profile: profile() })
  }

  return (
    <div class="wh-session">
      <div class="wh-session-stream">
        <Show when={userMessages().length} fallback={<EmptyState title="No messages yet" body="Describe a task to start a run." />}>
          <For each={userMessages()}>{(m) => <SessionTurn sessionID={ws.state.activeConversationID!} messageID={m.id} />}</For>
        </Show>
        <Show when={ws.state.run}>
          <RunTimeline />
        </Show>
      </div>
      <div class="wh-composer">
        <textarea
          class="wh-composer-input"
          placeholder="Describe a task…"
          value={draft()}
          disabled={ws.state.busy}
          onInput={(e) => setDraft(e.currentTarget.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) {
              e.preventDefault()
              void send()
            }
          }}
        />
        <div class="wh-composer-actions">
          <label class="wh-checkbox">
            <input type="checkbox" checked={artifactOnly()} onChange={(e) => setArtifactOnly(e.currentTarget.checked)} />
            Artifact only
          </label>
          <select class="wh-select" value={profile()} onChange={(e) => setProfile(e.currentTarget.value)}>
            <option value="auto">Auto</option>
            <option value="quality">Quality</option>
            <option value="speed">Speed</option>
            <option value="economy">Economy</option>
          </select>
          <Show when={ws.state.busy} fallback={<span class="wh-muted">⌘/Ctrl+Enter to send</span>}>
            <Spinner />
          </Show>
          <Button variant="primary" size="small" onClick={() => void send()} disabled={ws.state.busy}>
            Send
          </Button>
        </div>
      </div>
    </div>
  )
}

function RunTimeline() {
  const ws = useWayshard()
  const [expanded, setExpanded] = createSignal<string | null>(null)
  return (
    <section class="wh-timeline" aria-label="Run timeline">
      <div class="wh-timeline-header">
        <span>Run {ws.state.run?.id.slice(0, 8)}</span>
        <Tag>{ws.state.run?.status}</Tag>
      </div>
      <For each={ws.state.stages}>
        {(stage) => (
          <div class="wh-stage" data-status={stage.status}>
            <button class="wh-stage-row" onClick={() => setExpanded(expanded() === stage.id ? null : stage.id)}>
              <Icon name="chevron-right" size="small" />
              <span class="wh-stage-kind">{stageDisplay(stage.kind)}</span>
              <span class="wh-muted">{stage.status}</span>
            </button>
            <Show when={expanded() === stage.id}>
              <div class="wh-stage-detail">
                <div>Stage ID: {stage.id}</div>
                <div>Ordinal: {stage.ordinal}</div>
              </div>
            </Show>
          </div>
        )}
      </For>
    </section>
  )
}

export function EmptyState(props: { title: string; body?: string; action?: JSX.Element }) {
  return (
    <div class="wh-empty-state">
      <div class="wh-empty-title">{props.title}</div>
      <Show when={props.body}>
        <div class="wh-muted">{props.body}</div>
      </Show>
      <Show when={props.action}>{props.action}</Show>
    </div>
  )
}

export function ErrorState(props: { title: string; detail?: string }) {
  return (
    <div class="wh-empty-state" data-state="error">
      <div class="wh-empty-title">{props.title}</div>
      <Show when={props.detail}>
        <pre class="wh-error-detail">{props.detail}</pre>
      </Show>
    </div>
  )
}

export function WayshardApp() {
  return (
    <StateProvider>
      <CommandProvider>
        <DialogProvider>
          <FileComponentProvider component={FileFallback}>
            <Shell />
          </FileComponentProvider>
        </DialogProvider>
      </CommandProvider>
    </StateProvider>
  )
}
