// Wayshard shared graphical client.
//
// The layout, visual language, design tokens and message/session rendering are
// adapted from the imported OpenCode 2 client foundation (@wayshard/ui and
// @wayshard/gui/session-ui). The domain model, navigation and backend are
// Wayshard (@wayshard/sdk). Web, Desktop (Tauri 2) and Android (Tauri 2) all
// mount this same application.
import { For, Match, Show, Switch, createMemo, createSignal, onCleanup, onMount, type JSX } from "solid-js"
import { Button } from "@wayshard/ui/button"
import { IconButton } from "@wayshard/ui/icon-button"
import { Icon } from "@wayshard/ui/icon"
import { Tag } from "@wayshard/ui/tag"
import { Spinner } from "@wayshard/ui/spinner"
import { TextField } from "@wayshard/ui/text-field"
import { Dialog } from "@wayshard/ui/dialog"
import { DialogProvider } from "@wayshard/ui/context/dialog"
import { FileComponentProvider } from "@wayshard/ui/context/file"
import { DataProvider } from "@wayshard/gui/session-ui/context"
import { SessionTurn } from "@wayshard/gui/session-ui/components/session-turn"
import { StateProvider, useWayshard } from "../wayshard/state"
import { buildData, stageDisplay } from "../wayshard/adapter"
import { Views } from "./views"
import { CommandPalette } from "./command-palette"

export type ViewKey =
  | "session"
  | "changes"
  | "files"
  | "terminal"
  | "usage"
  | "routing"
  | "context"
  | "artifacts"
  | "knowledge"
  | "harnesses"
  | "recovery"
  | "approvals"
  | "notifications"
  | "settings"

export const NAV: { key: ViewKey; label: string; icon: string }[] = [
  { key: "session", label: "Session", icon: "prompt" },
  { key: "changes", label: "Changes", icon: "diff" },
  { key: "files", label: "Files", icon: "bullet-list" },
  { key: "terminal", label: "Terminal", icon: "terminal" },
  { key: "usage", label: "Usage", icon: "brain" },
  { key: "routing", label: "Routing", icon: "fork" },
  { key: "context", label: "Context", icon: "archive" },
  { key: "artifacts", label: "Artifacts", icon: "checklist" },
  { key: "knowledge", label: "Knowledge", icon: "console" },
  { key: "harnesses", label: "Harnesses", icon: "prompt" },
  { key: "recovery", label: "Recovery", icon: "arrow-up" },
  { key: "approvals", label: "Approvals", icon: "check-small" },
  { key: "notifications", label: "Notifications", icon: "bubble-5" },
  { key: "settings", label: "Settings", icon: "settings" },
]

const [activeView, setActiveView] = createSignal<ViewKey>("session")

export { activeView, setActiveView }

function FileFallback(props: { path?: string; content?: string }) {
  return (
    <pre data-component="file-view" class="wh-file-view">
      {props.content ?? ""}
    </pre>
  )
}

function Shell() {
  const ws = useWayshard()
  const [paletteOpen, setPaletteOpen] = createSignal(false)

  onMount(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "k") {
        e.preventDefault()
        setPaletteOpen(true)
      }
    }
    window.addEventListener("keydown", onKey)
    onCleanup(() => window.removeEventListener("keydown", onKey))
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
    <DialogProvider>
      <FileComponentProvider component={FileFallback}>
        <div data-component="wayshard-app" class="wh-app">
          <Show when={!ws.state.connected}>
            <ConnectionBar />
          </Show>
          <div class="wh-shell">
            <Sidebar />
            <main class="wh-main">
              <Topbar onCommand={() => setPaletteOpen(true)} />
              <div class="wh-view">
                <Show when={activeView() === "session"} fallback={<Views view={activeView()} />}>
                  <DataProvider data={data()} directory={ws.activeProject()?.path ?? ""} sessionID={ws.state.activeConversationID ?? undefined}>
                    <SessionView />
                  </DataProvider>
                </Show>
              </div>
            </main>
          </div>
          <CommandPalette open={paletteOpen()} onClose={() => setPaletteOpen(false)} />
        </div>
      </FileComponentProvider>
    </DialogProvider>
  )
}

function ConnectionBar() {
  const ws = useWayshard()
  return (
    <div class="wh-connection" data-state="disconnected">
      <span>
        {ws.state.connectionError ? `Disconnected: ${ws.state.connectionError}` : "Connecting to Wayshard server…"}
      </span>
      <Button size="small" variant="secondary" onClick={() => void ws.refreshAll()}>
        Reconnect
      </Button>
    </div>
  )
}

function Sidebar() {
  const ws = useWayshard()
  return (
    <aside class="wh-sidebar">
      <div class="wh-sidebar-header">
        <span class="wh-wordmark">Wayshard</span>
        <Tag>{ws.state.meta?.version ?? "dev"}</Tag>
      </div>
      <div class="wh-sidebar-section">
        <div class="wh-sidebar-label">Projects</div>
        <Show when={ws.state.projects.length} fallback={<div class="wh-empty">No projects</div>}>
          <For each={ws.state.projects}>
            {(p) => (
              <button
                class="wh-nav-item"
                data-active={p.id === ws.state.activeProjectID}
                onClick={() => void ws.selectProject(p.id)}
              >
                <Icon name="bullet-list" size="small" />
                <span class="wh-truncate">{p.name}</span>
              </button>
            )}
          </For>
        </Show>
        <Button size="small" variant="ghost" icon="arrow-right" onClick={() => void promptOpenProject(ws)}>
          Open project
        </Button>
      </div>
      <div class="wh-sidebar-section">
        <div class="wh-sidebar-label">Sessions</div>
        <Show when={ws.state.conversations.length} fallback={<div class="wh-empty">No sessions</div>}>
          <For each={ws.state.conversations}>
            {(c) => (
              <button
                class="wh-nav-item"
                data-active={c.id === ws.state.activeConversationID}
                onClick={() => void ws.selectConversation(c.id)}
              >
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
      <nav class="wh-sidebar-nav">
        <For each={NAV}>
          {(item) => (
            <button class="wh-nav-item" data-active={activeView() === item.key} onClick={() => setActiveView(item.key)}>
              <Icon name={item.icon as never} size="small" />
              <span>{item.label}</span>
            </button>
          )}
        </For>
      </nav>
      <div class="wh-sidebar-footer">
        <Tag>{ws.state.approvals.length} approvals</Tag>
        <Tag>{ws.state.notifications.length} notices</Tag>
      </div>
    </aside>
  )
}

function Topbar(props: { onCommand: () => void }) {
  const ws = useWayshard()
  return (
    <header class="wh-topbar">
      <div class="wh-topbar-left">
        <span class="wh-truncate">{ws.activeProject()?.name ?? "Wayshard"}</span>
        <Show when={ws.activeConversation()}>
          <span class="wh-muted">/ {ws.activeConversation()!.title || "Session"}</span>
        </Show>
      </div>
      <div class="wh-topbar-right">
        <Show when={ws.state.run}>
          <Tag>{ws.state.run!.status}</Tag>
        </Show>
        <Show when={ws.state.run && (ws.state.run!.status === "running" || ws.state.run!.status === "planning")}>
          <Button size="small" variant="ghost" onClick={() => void ws.cancelRun()}>
            Cancel
          </Button>
        </Show>
        <Button size="small" variant="ghost" icon="console" onClick={props.onCommand}>
          Command
        </Button>
      </div>
    </header>
  )
}

async function promptOpenProject(ws: ReturnType<typeof useWayshard>) {
  const path = window.prompt("Project path")
  if (path) await ws.openProject(path)
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
          <For each={userMessages()}>
            {(m) => <SessionTurn sessionID={ws.state.activeConversationID!} messageID={m.id} />}
          </For>
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
        <Show when={ws.state.run?.degradedRouting}>
          <Tag>degraded routing</Tag>
        </Show>
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
      <Shell />
    </StateProvider>
  )
}
