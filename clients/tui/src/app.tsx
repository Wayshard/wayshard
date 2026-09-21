// Wayshard TUI — adapted from the imported OpenCode 2 terminal UI foundation
// (@opentui/solid + the imported theme system). The domain, data path and
// backend are Wayshard (@wayshard/sdk). This is a real interactive terminal
// application, not a readline prompt.
import { render, useKeyboard, useRenderer, useTerminalDimensions } from "@opentui/solid"
import { For, Show, createMemo, createSignal, onCleanup, onMount } from "solid-js"
import { createStore } from "solid-js/store"
import { RGBA, TextAttributes } from "@opentui/core"
import { WayshardClient, type Approval, type Conversation, type Notification, type Project, type Run, type Stage } from "@wayshard/sdk"
import { loadTheme } from "./theme"
import { filterCommands, stageDisplay } from "./model"

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
  focus: "sidebar" | "prompt"
  palette: boolean
  paletteQuery: string
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
    focus: "prompt",
    palette: false,
    paletteQuery: "",
    status: "",
  }
}

function createClient(): WayshardClient {
  const baseUrl = process.env.WAYSHARD_URL ?? "http://127.0.0.1:7420"
  const token = process.env.WAYSHARD_TOKEN ?? ""
  return new WayshardClient(baseUrl, token)
}

function App() {
  const theme = loadTheme()
  const client = createClient()
  const renderer = useRenderer()
  const dimensions = useTerminalDimensions()
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
        if (cid) {
          const messages = await client.messages(cid)
          setState("messages", messages.map((m) => ({ id: m.id, role: m.role, body: m.body })))
        }
      }
      if (state.run) {
        const [run, stages] = await Promise.all([client.run(state.run.id), client.stages(state.run.id)])
        setState("run", run)
        setState("stages", stages)
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
      setState("status", "run started")
      await refresh()
    } catch (err) {
      setState("status", `send failed: ${String(err)}`)
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
    await refresh()
  }

  const paletteCommands = createMemo(() => {
    const cmds = [
      { id: "new-session", title: "New session", run: () => void newConversation() },
      { id: "cancel-run", title: "Cancel run", run: () => state.run && void client.cancel(state.run.id).then(refresh) },
      { id: "retry-run", title: "Retry run", run: () => state.run && void client.retry(state.run.id).then(refresh) },
      { id: "integrate-run", title: "Integrate run", run: () => state.run && void client.integrate(state.run.id).then(refresh) },
      { id: "refresh", title: "Refresh", run: () => void refresh() },
    ]
    return filterCommands(cmds, state.paletteQuery)
  })

  useKeyboard((key) => {
    if (state.palette) {
      if (key.name === "escape") setState("palette", false)
      if (key.name === "enter" && paletteCommands().length) {
        void paletteCommands()[0].run()
        setState("palette", false)
      }
      return
    }
    if (key.ctrl && key.name === "k") {
      setState("palette", true)
      setState("paletteQuery", "")
      return
    }
    if (key.name === "tab") {
      setState("focus", state.focus === "prompt" ? "sidebar" : "prompt")
      return
    }
    if (key.name === "r" && key.ctrl) void refresh()
  })

  const title = () => `Wayshard${state.connected ? "" : " (disconnected)"}`
  const activeProject = () => state.projects.find((p) => p.id === state.activeProjectID)

  return (
    <box flexDirection="column" width="100%" height="100%" backgroundColor={theme.background}>
      {/* header */}
      <box flexDirection="row" paddingLeft={1} paddingRight={1} backgroundColor={theme.backgroundPanel} height={1}>
        <text fg={theme.primary} attributes={TextAttributes.BOLD}>
          {title()}
        </text>
        <text fg={theme.textMuted}>{`  ${activeProject()?.name ?? ""}  `}</text>
        <text fg={state.connected ? theme.success : theme.error}>{state.connected ? "● connected" : "○ offline"}</text>
        <box flexGrow={1} />
        <text fg={theme.textMuted}>{`${state.approvals.length} approvals  ${state.notifications.length} notices`}</text>
      </box>

      {/* body */}
      <box flexDirection="row" flexGrow={1} minHeight={0}>
        {/* sidebar */}
        <box
          flexDirection="column"
          width={28}
          borderStyle="single"
          borderColor={state.focus === "sidebar" ? theme.borderActive : theme.border}
        >
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

        {/* main */}
        <box flexDirection="column" flexGrow={1} minHeight={0} borderStyle="single" borderColor={theme.border}>
          <scrollbox flexGrow={1} minHeight={0} paddingLeft={1} paddingRight={1}>
            <For each={state.messages}>
              {(m) => (
                <box flexDirection="column" marginBottom={1}>
                  <text fg={m.role === "user" ? theme.accent : theme.success} attributes={TextAttributes.BOLD}>
                    {m.role === "user" ? "You" : "Wayshard"}
                  </text>
                  <text fg={theme.text}>{m.body}</text>
                </box>
              )}
            </For>
            <Show when={state.run}>
              <box flexDirection="column" marginTop={1}>
                <text fg={theme.textMuted} attributes={TextAttributes.BOLD}>
                  {`Run ${state.run!.id.slice(0, 8)} · ${state.run!.status}`}
                </text>
                <For each={state.stages}>
                  {(s) => (
                    <text fg={s.status === "failed" ? theme.error : s.status === "running" ? theme.info : theme.textMuted}>
                      {`  ${stageDisplay(s.kind)} · ${s.status}`}
                    </text>
                  )}
                </For>
              </box>
            </Show>
            <Show when={!state.messages.length && !state.run}>
              <text fg={theme.textMuted}>No messages yet. Describe a task below.</text>
            </Show>
          </scrollbox>

          <Show when={state.approvals.length}>
            <box flexDirection="column" borderStyle="single" borderColor={theme.warning} paddingLeft={1}>
              <text fg={theme.warning} attributes={TextAttributes.BOLD}>
                Approvals
              </text>
              <For each={state.approvals}>
                {(a) => (
                  <box flexDirection="row">
                    <text fg={theme.text}>{`${a.kind} ${a.resource} — `}</text>
                    <text fg={theme.success} onMouseDown={() => void resolveApproval(a.id, "allowed")}>
                      [allow]
                    </text>
                    <text fg={theme.error} onMouseDown={() => void resolveApproval(a.id, "denied")}>
                      {" [deny]"}
                    </text>
                  </box>
                )}
              </For>
            </box>
          </Show>

          <box flexDirection="row" borderStyle="single" borderColor={state.focus === "prompt" ? theme.borderActive : theme.border}>
            <text fg={theme.primary}>{"> "}</text>
            <input
              flexGrow={1}
              placeholder="Describe a task…"
              value={prompt()}
              onInput={(v: string) => setPrompt(v)}
              onSubmit={() => void send()}
            />
          </box>
        </box>
      </box>

      {/* status bar */}
      <box flexDirection="row" height={1} paddingLeft={1} backgroundColor={theme.backgroundPanel}>
        <text fg={theme.textMuted}>
          {`Ctrl+K palette  Tab focus  Ctrl+R refresh  ${dimensions().width}x${dimensions().height}`}
        </text>
        <box flexGrow={1} />
        <Show when={state.status}>
          <text fg={theme.info}>{state.status}</text>
        </Show>
        <Show when={state.error}>
          <text fg={theme.error}>{`  ${state.error}`}</text>
        </Show>
      </box>

      {/* command palette */}
      <Show when={state.palette}>
        <box
          position="absolute"
          top={Math.max(2, Math.floor(dimensions().height / 4))}
          left={Math.max(2, Math.floor(dimensions().width / 4))}
          width={Math.max(40, Math.floor(dimensions().width / 2))}
          flexDirection="column"
          borderStyle="double"
          borderColor={theme.borderActive}
          backgroundColor={theme.backgroundMenu}
          paddingLeft={1}
          paddingRight={1}
        >
          <text fg={theme.primary} attributes={TextAttributes.BOLD}>
            Command palette
          </text>
          <input
            placeholder="Type a command…"
            value={state.paletteQuery}
            onInput={(v: string) => setState("paletteQuery", v)}
            onSubmit={() => {
              if (paletteCommands().length) void paletteCommands()[0].run()
              setState("palette", false)
            }}
          />
          <For each={paletteCommands()}>
            {(c) => (
              <text fg={theme.text} onMouseDown={() => (void c.run(), setState("palette", false))}>
                {` ${c.title}`}
              </text>
            )}
          </For>
        </box>
      </Show>
    </box>
  )
}

export async function run() {
  await render(() => <App />)
}
