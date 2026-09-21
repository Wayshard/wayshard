// Wayshard TUI — adapted from the imported OpenCode 2 terminal client.
//
// The dialog framework, toast, spinner, border, theme system, keymap surface and
// command-palette patterns descend from `packages/tui/src`. The domain, data
// path and backend are Wayshard (@wayshard/sdk): projects, conversations,
// messages, runs, stages, changes, files, routing, usage, approvals,
// notifications, integration and diagnostics.
import { render, useRenderer, useTerminalDimensions } from "@opentui/solid"
import { ErrorBoundary, For, Match, Show, Switch, createMemo, createSignal, onCleanup, onMount } from "solid-js"
import { createStore } from "solid-js/store"
import { TextAttributes } from "@opentui/core"
import { WayshardClient, parseInvitation, randomNonce, verifyServerIdentity, type Approval, type Conversation, type Notification, type Project, type Run, type Stage } from "@wayshard/sdk"
import { hostname } from "node:os"
import { mkdirSync, readFileSync, writeFileSync } from "node:fs"
import { dirname, join } from "node:path"
import { DialogPrompt } from "./ui/dialog-prompt"
import { ThemeProvider, useTheme } from "./context/theme"
import { KVProvider } from "./context/kv"
import { ClipboardProvider } from "./context/clipboard"
import { DialogProvider, Dialog, useDialog } from "./ui/dialog"
import { DialogSelect } from "./ui/dialog-select"
import { ToastProvider, Toast, useToast } from "./ui/toast"
import { useBindings } from "./keymap"
import { Spinner } from "./component/spinner"
import { cycleTab as cycleTabModel, filterCommands, stageDisplay, TUI_TABS, type TuiTab } from "./model"

type Tab = TuiTab
const TABS = TUI_TABS

interface TuiState {
  connected: boolean
  error: string
  projects: Project[]
  conversations: Conversation[]
  activeProjectID: string | null
  activeConversationID: string | null
  messages: { id: string; role: string; body: string }[]
  run: Run | null
  stages: Stage[]
  approvals: Approval[]
  notifications: Notification[]
  tab: Tab
  usage: Array<Record<string, unknown>>
  routes: Array<Record<string, unknown>>
  runChanges: string[]
  files: Array<{ name: string; dir: boolean }>
  status: string
}

function initialState(): TuiState {
  return {
    connected: false,
    error: "",
    projects: [],
    conversations: [],
    activeProjectID: null,
    activeConversationID: null,
    messages: [],
    run: null,
    stages: [],
    approvals: [],
    notifications: [],
    tab: "session",
    usage: [],
    routes: [],
    runChanges: [],
    files: [],
    status: "",
  }
}

function tuiTokenPath(): string {
  return join(process.env.XDG_CONFIG_HOME ?? join(process.env.HOME ?? ".", ".config"), "wayshard", "device.token")
}

function loadTuiToken(): string {
  try {
    return readFileSync(tuiTokenPath(), "utf8").trim()
  } catch {
    return ""
  }
}

function saveTuiToken(token: string) {
  try {
    const p = tuiTokenPath()
    mkdirSync(dirname(p), { recursive: true, mode: 0o700 })
    writeFileSync(p, token, { mode: 0o600 })
  } catch {
    // ignore
  }
}

function createClient(): WayshardClient {
  return new WayshardClient(process.env.WAYSHARD_URL ?? "http://127.0.0.1:7420", process.env.WAYSHARD_TOKEN ?? loadTuiToken())
}

function Shell() {
  const { theme } = useTheme()
  const dialog = useDialog()
  const toast = useToast()
  const renderer = useRenderer()
  const dimensions = useTerminalDimensions()
  const client = createClient()
  const [state, setState] = createStore<TuiState>(initialState())
  const [prompt, setPrompt] = createSignal("")

  async function refresh() {
    try {
      const projects = await client.projects()
      setState("projects", projects)
      const pid = state.activeProjectID ?? projects[0]?.id ?? null
      setState("activeProjectID", pid)
      if (pid) {
        const conversations = await client.conversations(pid)
        setState("conversations", conversations)
        const cid = state.activeConversationID ?? conversations[0]?.id ?? null
        setState("activeConversationID", cid)
        if (cid) setState("messages", (await client.messages(cid)).map((m) => ({ id: m.id, role: m.role, body: m.body })))
        setState("files", (await client.listFiles(pid, "")) as Array<{ name: string; dir: boolean }>)
      }
      if (state.run) {
        const [run, stages, usage, routes] = await Promise.all([
          client.run(state.run.id),
          client.stages(state.run.id),
          client.usage(state.run.id),
          client.routes(state.run.id),
        ])
        setState("run", run)
        setState("stages", stages)
        setState("usage", usage as Array<Record<string, unknown>>)
        setState("routes", routes as Array<Record<string, unknown>>)
        const changes = (await client.runChanges(state.run.id)) as { delta?: { files?: Array<{ path: string }> } }
        setState("runChanges", (changes.delta?.files ?? []).map((f) => f.path))
      }
      const [approvals, notifications] = await Promise.all([client.approvals(), client.notifications()])
      setState("approvals", approvals)
      setState("notifications", notifications)
      setState("connected", true)
      setState("error", "")
    } catch (err) {
      setState("connected", false)
      setState("error", String(err))
    }
  }

  onMount(() => {
    void refresh()
    const timer = setInterval(() => void refresh(), 1500)
    onCleanup(() => clearInterval(timer))
  })

  async function send() {
    const text = prompt().trim()
    if (!text || !state.activeConversationID) return
    setPrompt("")
    try {
      const res = await client.sendMessage(state.activeConversationID, text)
      if (res?.run) setState("run", res.run)
      toast.show({ variant: "success", message: "Run started", title: "Wayshard" })
      await refresh()
    } catch (err) {
      toast.error(err)
    }
  }

  async function selectProject(id: string) {
    setState("activeProjectID", id)
    setState("activeConversationID", null)
    setState("run", null)
    setState("stages", [])
    await refresh()
  }

  async function selectConversation(id: string) {
    setState("activeConversationID", id)
    setState("run", null)
    setState("stages", [])
    await refresh()
  }

  async function newConversation() {
    if (!state.activeProjectID) return
    const c = await client.createConversation(state.activeProjectID)
    setState("activeConversationID", c.id)
    await refresh()
  }

  async function resolveApproval(id: string, status: "allowed" | "denied") {
    await client.resolveApproval(id, status)
    toast.show({ variant: status === "allowed" ? "success" : "warning", message: `Approval ${status}`, title: "Approvals" })
    await refresh()
  }

  const commands = createMemo(() => [
    { id: "new-session", title: "New session", run: () => void newConversation() },
    { id: "cancel-run", title: "Cancel run", run: () => state.run && void client.cancel(state.run.id).then(refresh) },
    { id: "retry-run", title: "Retry run", run: () => state.run && void client.retry(state.run.id).then(refresh) },
    { id: "integrate-run", title: "Integrate run", run: () => state.run && void client.integrate(state.run.id).then(refresh) },
    { id: "refresh", title: "Refresh", run: () => void refresh() },
    { id: "settings", title: "Settings / connection", run: () => openSettings() },
    { id: "connect.pair", title: "Pair device (verified identity)", run: () => pairDevice() },
  ])

  function openPalette() {
    dialog.replace(() => (
      <DialogSelect<{ id: string; title: string; run: () => void | Promise<void> }>
        title="Command palette"
        placeholder="Type a command…"
        options={commands().map((c) => ({ title: c.title, value: c, category: c.id.split(".")[0] }))}
        onSelect={(option) => {
          void option.value.run()
          dialog.clear()
        }}
      />
    ))
  }

  function pairDevice() {
    dialog.replace(() => (
      <DialogPrompt
        title="Pair device"
        description={() => (
          <text fg={theme.textMuted}>
            Paste the Wayshard pairing invitation (Server, Fingerprint, URL, Code). The server identity is verified before any
            credential is saved.
          </text>
        )}
        placeholder="Paste pairing invitation…"
        onConfirm={(text) => void completePair(text)}
        onCancel={() => dialog.clear()}
      />
    ))
  }

  async function completePair(text: string) {
    const inv = parseInvitation(text)
    if (!inv || !inv.code || !inv.serverId || !inv.fingerprint) {
      toast.show({ variant: "error", title: "Pairing", message: "Invalid or incomplete pairing invitation." })
      dialog.clear()
      return
    }
    const url = inv.advertisedUrl ?? client.baseUrl
    const verifier = new WayshardClient(url, "")
    try {
      const nonce = randomNonce()
      const challenge = await verifier.pairingChallenge(nonce)
      const result = await verifyServerIdentity(nonce, challenge, { serverId: inv.serverId, fingerprint: inv.fingerprint })
      if (!result.ok) throw new Error(`server identity verification failed: ${result.reason}`)
      const res = await verifier.pair(inv.code, hostname(), "cli", { serverId: inv.serverId, fingerprint: inv.fingerprint })
      if (!res.credential) throw new Error("server did not return a credential")
      if (res.serverId && res.serverId !== inv.serverId) throw new Error("paired server id does not match the invited identity")
      if (res.fingerprint && res.fingerprint !== inv.fingerprint) throw new Error("paired fingerprint does not match the invited identity")
      saveTuiToken(res.credential)
      client.baseUrl = url
      client.token = res.credential
      toast.show({ variant: "success", title: "Paired", message: `Verified ${inv.fingerprint}` })
      void refresh()
    } catch (err) {
      toast.error(err)
    }
    dialog.clear()
  }

  function openSettings() {
    dialog.replace(
      () => (
        <Dialog size="medium" onClose={() => dialog.clear()}>
          <box flexDirection="column" paddingLeft={2} paddingRight={2} gap={1}>
            <text fg={theme.primary} attributes={TextAttributes.BOLD}>
              Settings
            </text>
            <text fg={theme.text}>Server: {process.env.WAYSHARD_URL ?? "http://127.0.0.1:7420"}</text>
            <text fg={theme.textMuted}>
              Wayshard uses per-device pairing credentials, not usernames or passwords. Set WAYSHARD_URL and
              WAYSHARD_TOKEN (or complete pairing) to connect.
            </text>
            <text fg={theme.text}>Theme: {process.env.WAYSHARD_TUI_THEME ?? "tokyonight"}</text>
          </box>
        </Dialog>
      ),
      () => dialog.clear(),
    )
  }

  useBindings(() => ({
    bindings: [
      { key: "tab", desc: "Next tab", group: "Navigation", cmd: () => cycleTab(1) },
      { key: "k", desc: "Command palette", group: "Navigation", cmd: openPalette },
      { key: "r", desc: "Refresh", group: "Navigation", cmd: () => void refresh() },
      { key: "s", desc: "Settings", group: "Navigation", cmd: openSettings },
    ],
  }))

  function cycleTab(delta: number) {
    setState("tab", cycleTabModel(state.tab, delta))
  }

  const activeProject = () => state.projects.find((p) => p.id === state.activeProjectID)

  return (
    <box flexDirection="column" width="100%" height="100%" backgroundColor={theme.background}>
      <Toast />
      <box flexDirection="row" paddingLeft={1} paddingRight={1} backgroundColor={theme.backgroundPanel} height={1}>
        <text fg={theme.primary} attributes={TextAttributes.BOLD}>
          Wayshard
        </text>
        <text fg={theme.textMuted}>{`  ${activeProject()?.name ?? ""}  `}</text>
        <text fg={state.connected ? theme.success : theme.error}>{state.connected ? "● connected" : "○ offline"}</text>
        <box flexGrow={1} />
        <text fg={theme.textMuted}>{`${state.approvals.length} approvals  ${state.notifications.length} notices`}</text>
      </box>

      <box flexDirection="row" flexGrow={1} minHeight={0}>
        <box flexDirection="column" width={28} borderStyle="single" borderColor={theme.border}>
          <text fg={theme.textMuted}>Projects</text>
          <For each={state.projects}>
            {(p) => (
              <text fg={p.id === state.activeProjectID ? theme.primary : theme.text} onMouseDown={() => void selectProject(p.id)}>
                {` ${p.name}`}
              </text>
            )}
          </For>
          <text fg={theme.textMuted}>Sessions</text>
          <For each={state.conversations}>
            {(c) => (
              <text fg={c.id === state.activeConversationID ? theme.primary : theme.text} onMouseDown={() => void selectConversation(c.id)}>
                {` ${c.title || "Session"}`}
              </text>
            )}
          </For>
        </box>

        <box flexDirection="column" flexGrow={1} minHeight={0} borderStyle="single" borderColor={theme.border}>
          <TabBar tab={state.tab} />
          <Show when={state.tab === "session"}>
            <SessionView state={state} prompt={prompt()} setPrompt={setPrompt} send={send} />
          </Show>
          <Show when={state.tab === "changes"}>
            <ListView title="Run changes" items={state.runChanges} empty="No run changes" />
          </Show>
          <Show when={state.tab === "files"}>
            <ListView title="Files" items={state.files.map((f) => (f.dir ? `${f.name}/` : f.name))} empty="No files" />
          </Show>
          <Show when={state.tab === "routing"}>
            <KeyValue title="Routing" rows={state.routes.map((r) => [String(r.stageId ?? r.stage ?? ""), String(r.harnessId ?? "") + " " + String(r.modelId ?? "")])} empty="No route decisions" />
          </Show>
          <Show when={state.tab === "usage"}>
            <KeyValue
              title="Usage"
              rows={state.usage.map((u) => [String(u.stageId ?? ""), `in ${u.inputTokens ?? 0} out ${u.outputTokens ?? 0} cost ${u.costMicros ?? 0}`])}
              empty="No usage recorded"
            />
          </Show>
          <Show when={state.tab === "approvals"}>
            <Approvals state={state} resolve={resolveApproval} />
          </Show>
          <Show when={state.tab === "settings"}>
            <box flexDirection="column" paddingLeft={1} gap={1}>
              <text fg={theme.textMuted}>Connection</text>
              <text fg={theme.text}>{`URL: ${process.env.WAYSHARD_URL ?? "http://127.0.0.1:7420"}`}</text>
              <text fg={theme.textMuted}>Press Ctrl+K then "Settings" for pairing guidance. Terminal client uses WAYSHARD_URL/WAYSHARD_TOKEN.</text>
            </box>
          </Show>
        </box>
      </box>

      <box flexDirection="row" height={1} paddingLeft={1} backgroundColor={theme.backgroundPanel}>
        <text fg={theme.textMuted}>{`Ctrl+K palette  Tab tab  Ctrl+R refresh  ${dimensions().width}x${dimensions().height}`}</text>
        <box flexGrow={1} />
        <Show when={state.status}>
          <text fg={theme.info}>{state.status}</text>
        </Show>
        <Show when={state.error}>
          <text fg={theme.error}>{`  ${state.error}`}</text>
        </Show>
      </box>
    </box>
  )
}

function TabBar(props: { tab: Tab }) {
  const { theme } = useTheme()
  return (
    <box flexDirection="row" gap={1} paddingLeft={1} backgroundColor={theme.backgroundPanel} height={1}>
      <For each={TABS}>
        {(t) => (
          <text fg={props.tab === t ? theme.primary : theme.textMuted} attributes={props.tab === t ? TextAttributes.BOLD : undefined}>
            {t}
          </text>
        )}
      </For>
    </box>
  )
}

function SessionView(props: {
  state: TuiState
  prompt: string
  setPrompt: (v: string) => void
  send: () => void | Promise<void>
}) {
  const { theme } = useTheme()
  return (
    <box flexDirection="column" flexGrow={1} minHeight={0}>
      <scrollbox flexGrow={1} minHeight={0} paddingLeft={1} paddingRight={1}>
        <For each={props.state.messages}>
          {(m) => (
            <box flexDirection="column" marginBottom={1}>
              <text fg={m.role === "user" ? theme.accent : theme.success} attributes={TextAttributes.BOLD}>
                {m.role === "user" ? "You" : "Wayshard"}
              </text>
              <text fg={theme.text}>{m.body}</text>
            </box>
          )}
        </For>
        <Show when={props.state.run}>
          <box flexDirection="column" marginTop={1}>
            <text fg={theme.textMuted} attributes={TextAttributes.BOLD}>
              {`Run ${props.state.run!.id.slice(0, 8)} · ${props.state.run!.status}`}
            </text>
            <For each={props.state.stages}>
              {(s) => (
                <text fg={s.status === "failed" ? theme.error : s.status === "running" ? theme.info : theme.textMuted}>
                  {`  ${stageDisplay(s.kind)} · ${s.status}`}
                </text>
              )}
            </For>
          </box>
        </Show>
        <Show when={!props.state.messages.length && !props.state.run}>
          <text fg={theme.textMuted}>No messages yet. Describe a task below.</text>
        </Show>
      </scrollbox>
      <box flexDirection="row" borderStyle="single" borderColor={theme.border}>
        <text fg={theme.primary}>{"> "}</text>
        <input flexGrow={1} placeholder="Describe a task…" value={props.prompt} onInput={(v: string) => props.setPrompt(v)} onSubmit={() => void props.send()} />
      </box>
    </box>
  )
}

function ListView(props: { title: string; items: string[]; empty: string }) {
  const { theme } = useTheme()
  return (
    <scrollbox flexGrow={1} minHeight={0} paddingLeft={1}>
      <text fg={theme.textMuted} attributes={TextAttributes.BOLD}>
        {props.title}
      </text>
      <Show when={props.items.length} fallback={<text fg={theme.textMuted}>{props.empty}</text>}>
        <For each={props.items}>{(item) => <text fg={theme.text}>{`  ${item}`}</text>}</For>
      </Show>
    </scrollbox>
  )
}

function KeyValue(props: { title: string; rows: [string, string][]; empty: string }) {
  const { theme } = useTheme()
  return (
    <scrollbox flexGrow={1} minHeight={0} paddingLeft={1}>
      <text fg={theme.textMuted} attributes={TextAttributes.BOLD}>
        {props.title}
      </text>
      <Show when={props.rows.length} fallback={<text fg={theme.textMuted}>{props.empty}</text>}>
        <For each={props.rows}>
          {([k, v]) => (
            <text fg={theme.text}>
              {`  ${k}  `}
              <span style={{ fg: theme.textMuted }}>{v}</span>
            </text>
          )}
        </For>
      </Show>
    </scrollbox>
  )
}

function Approvals(props: { state: TuiState; resolve: (id: string, status: "allowed" | "denied") => void | Promise<void> }) {
  const { theme } = useTheme()
  return (
    <scrollbox flexGrow={1} minHeight={0} paddingLeft={1}>
      <text fg={theme.textMuted} attributes={TextAttributes.BOLD}>
        Approvals
      </text>
      <Show when={props.state.approvals.length} fallback={<text fg={theme.textMuted}>No pending approvals</text>}>
        <For each={props.state.approvals}>
          {(a) => (
            <box flexDirection="row">
              <text fg={theme.text}>{`${a.kind} ${a.resource} — `}</text>
              <text fg={theme.success} onMouseDown={() => void props.resolve(a.id, "allowed")}>
                [allow]
              </text>
              <text fg={theme.error} onMouseDown={() => void props.resolve(a.id, "denied")}>
                {" [deny]"}
              </text>
            </box>
          )}
        </For>
      </Show>
    </scrollbox>
  )
}

function App() {
  return (
    <ErrorBoundary fallback={(err) => {
      console.error("TUI_BOUNDARY_ERROR:", String(err), (err as Error)?.stack ?? "")
      return <box><text>Wayshard TUI failed to start. See the console for details.</text></box>
    }}>
    <ThemeProvider>
      <KVProvider>
        <ClipboardProvider>
          <ToastProvider>
            <DialogProvider>
              <Shell />
            </DialogProvider>
          </ToastProvider>
        </ClipboardProvider>
      </KVProvider>
    </ThemeProvider>
    </ErrorBoundary>
  )
}

export async function run() {
  try {
    await render(() => <App />)
  } catch (err) {
    console.error("wayshard-tui error:", err)
    throw err
  }
}
