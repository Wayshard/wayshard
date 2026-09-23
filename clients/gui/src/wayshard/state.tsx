// Wayshard graphical client state layer.
//
// This is Wayshard-owned domain state on top of @wayshard/sdk. It replaces the
// imported OpenCode server/session/provider context: the UI lineage is inherited
// from the OpenCode client foundation, but the domain model and the only backend
// are Wayshard.
import { createContext, createMemo, createSignal, onCleanup, useContext, type Accessor, type ParentProps } from "solid-js"
import { createStore } from "solid-js/store"
import { WayshardClient, type Approval, type Artifact, type Conversation, type Notification, type Project, type Run, type Stage } from "@wayshard/sdk"
import { credentialStore } from "./secure-store"

export interface ConnectionConfig {
  baseUrl: string
  token: string
}

const STORAGE_KEY = "wayshard.connection"

// Only the endpoint is persisted in web storage. The device credential lives in
// platform-secure storage (native clients) or the HttpOnly server session
// (browser clients) and is never written to localStorage.
export function loadConnection(): ConnectionConfig {
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    if (raw) {
      const parsed = JSON.parse(raw) as Partial<ConnectionConfig>
      if (parsed.baseUrl) return { baseUrl: parsed.baseUrl, token: "" }
    }
  } catch {
    // ignore
  }
  return { baseUrl: location.origin, token: "" }
}

export function saveConnection(cfg: ConnectionConfig) {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify({ baseUrl: cfg.baseUrl }))
  } catch {
    // ignore
  }
  const store = credentialStore()
  if (cfg.token) void store.save(cfg.token)
  else void store.clear()
}

export interface MessageView {
  id: string
  role: string
  body: string
}

export interface State {
  connection: ConnectionConfig
  connected: boolean
  connectionError: string
  // authRequired is set when the server rejects an authenticated request, so
  // the pairing gate shows for browser clients that rely on the session cookie
  // as well as native clients with a stale credential.
  authRequired: boolean
  lastEventSeq: number
  meta: { product: string; version: string } | null
  projects: Project[]
  activeProjectID: string | null
  conversations: Conversation[]
  activeConversationID: string | null
  messages: MessageView[]
  runsByConversation: Record<string, Run>
  activeRunID: string | null
  run: Run | null
  stages: Stage[]
  artifacts: Artifact[]
  runChanges: string[]
  approvals: Approval[]
  notifications: Notification[]
  busy: boolean
}

function initialState(): State {
  return {
    connection: loadConnection(),
    connected: false,
    connectionError: "",
    authRequired: false,
    lastEventSeq: 0,
    meta: null,
    projects: [],
    activeProjectID: null,
    conversations: [],
    activeConversationID: null,
    messages: [],
    runsByConversation: {},
    activeRunID: null,
    run: null,
    stages: [],
    artifacts: [],
    runChanges: [],
    approvals: [],
    notifications: [],
    busy: false,
  }
}

export interface StateAPI {
  state: State
  client: Accessor<WayshardClient>
  activeProject: Accessor<Project | undefined>
  activeConversation: Accessor<Conversation | undefined>
  activeRun: Accessor<Run | undefined>
  setConnection(cfg: ConnectionConfig): void
  hydrateCredentials(): Promise<void>
  refreshAll(): Promise<void>
  openProject(path: string): Promise<void>
  selectProject(id: string): Promise<void>
  selectConversation(id: string): Promise<void>
  newConversation(): Promise<void>
  send(text: string, opts?: { artifactOnly?: boolean; profile?: string }): Promise<void>
  cancelRun(): Promise<void>
  retryRun(): Promise<void>
  integrateRun(): Promise<void>
  resolveApproval(id: string, status: "allowed" | "denied"): Promise<void>
  readNotification(id: string): Promise<void>
  refreshRun(): Promise<void>
}

function createStateAPI(): StateAPI {
  const [state, setState] = createStore<State>(initialState())
  const client = createMemo(() => new WayshardClient(state.connection.baseUrl, state.connection.token))

  let socket: WebSocket | undefined
  let reconnectTimer: ReturnType<typeof setTimeout> | undefined

  const activeProject = createMemo(() => state.projects.find((p) => p.id === state.activeProjectID))
  const activeConversation = createMemo(() => state.conversations.find((c) => c.id === state.activeConversationID))
  const activeRun = createMemo(() => state.run ?? undefined)

  async function refreshProjects() {
    const projects = await client().projects()
    setState("projects", projects)
    if (!state.activeProjectID && projects.length) setState("activeProjectID", projects[0].id)
  }

  async function refreshConversations() {
    if (!state.activeProjectID) return
    const conversations = await client().conversations(state.activeProjectID)
    setState("conversations", conversations)
    if (!state.activeConversationID && conversations.length) setState("activeConversationID", conversations[0].id)
  }

  async function refreshMessages() {
    if (!state.activeConversationID) {
      setState("messages", [])
      return
    }
    const messages = await client().messages(state.activeConversationID)
    setState("messages", messages.map((m) => ({ id: m.id, role: m.role, body: m.body })))
    const run = state.runsByConversation[state.activeConversationID]
    if (run) {
      setState("activeRunID", run.id)
      await refreshRun()
    } else {
      setState("activeRunID", null)
      setState("run", null)
      setState("stages", [])
      setState("artifacts", [])
    }
  }

  async function refreshRun() {
    const id = state.activeRunID
    if (!id) return
    const [run, stages, artifacts] = await Promise.all([client().run(id), client().stages(id), client().artifacts(id)])
    setState("run", run)
    setState("stages", stages)
    setState("artifacts", artifacts)
    try {
      const changes = (await client().runChanges(id)) as { delta?: { files?: Array<{ path: string }> } }
      setState("runChanges", (changes.delta?.files ?? []).map((f) => f.path))
    } catch {
      setState("runChanges", [])
    }
    if (state.activeConversationID) setState("runsByConversation", state.activeConversationID, run)
  }

  async function refreshGlobal() {
    const [approvals, notifications] = await Promise.all([client().approvals(), client().notifications()])
    setState("approvals", approvals)
    setState("notifications", notifications)
  }

  async function refreshAll() {
    try {
      const meta = await client().meta()
      setState("meta", { product: meta.product, version: meta.version })
      await refreshProjects()
      await refreshConversations()
      await refreshMessages()
      await refreshGlobal()
      setState("connected", true)
      setState("authRequired", false)
      setState("connectionError", "")
      await connectEvents()
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err)
      setState("connected", false)
      setState("authRequired", message.startsWith("401"))
      setState("connectionError", message)
    }
  }

  async function connectEvents() {
    if (socket) socket.close()
    let url: string
    try {
      url = await client().eventURL(state.lastEventSeq)
    } catch (err) {
      setState("connectionError", String(err))
      return
    }
    try {
      socket = new WebSocket(url)
    } catch (err) {
      setState("connectionError", String(err))
      return
    }
    socket.onopen = () => setState("connected", true)
    socket.onclose = () => {
      setState("connected", false)
      reconnectTimer = setTimeout(() => void connectEvents(), 2000)
    }
    socket.onerror = () => setState("connectionError", "event stream error")
    socket.onmessage = (ev) => {
      try {
        const frame = JSON.parse(ev.data) as { seq?: number; type?: string }
        if (typeof frame.seq === "number") setState("lastEventSeq", frame.seq)
        void onEvent(frame.type ?? "")
      } catch {
        // ignore malformed frame
      }
    }
  }

  async function onEvent(type: string) {
    if (type.startsWith("approval.")) await refreshGlobal()
    else if (type.startsWith("notification.")) await refreshGlobal()
    else if (type.startsWith("run.") || type.startsWith("stage.") || type.startsWith("execution.")) {
      await refreshRun()
    } else if (type.startsWith("project.") || type.startsWith("integration.")) {
      await refreshRun()
    }
  }

  onCleanup(() => {
    socket?.close()
    if (reconnectTimer) clearTimeout(reconnectTimer)
  })

  return {
    state,
    client,
    activeProject,
    activeConversation,
    activeRun,
    setConnection(cfg) {
      // Browser clients authenticate with the HttpOnly server session and never
      // retain the device credential; native clients keep it in secure storage.
      const store = credentialStore()
      const effective = store.kind === "browser" ? { baseUrl: cfg.baseUrl, token: "" } : cfg
      saveConnection(effective)
      setState("connection", effective)
      setState("authRequired", false)
      void refreshAll()
    },
    async hydrateCredentials() {
      // Native clients load the device credential from platform-secure storage
      // once at startup; browser clients use the HttpOnly server session.
      const store = credentialStore()
      if (store.kind !== "tauri") return
      const token = await store.load()
      if (token && !state.connection.token) setState("connection", "token", token)
    },
    refreshAll,
    async openProject(path) {
      setState("busy", true)
      try {
        const p = await client().openProject(path)
        await refreshProjects()
        setState("activeProjectID", p.id)
        await refreshConversations()
        await refreshMessages()
      } finally {
        setState("busy", false)
      }
    },
    async selectProject(id) {
      setState("activeProjectID", id)
      setState("activeConversationID", null)
      setState("run", null)
      setState("stages", [])
      setState("artifacts", [])
      await refreshConversations()
      await refreshMessages()
    },
    async selectConversation(id) {
      setState("activeConversationID", id)
      await refreshMessages()
    },
    async newConversation() {
      if (!state.activeProjectID) return
      const c = await client().createConversation(state.activeProjectID)
      await refreshConversations()
      setState("activeConversationID", c.id)
      setState("messages", [])
      setState("run", null)
      setState("stages", [])
    },
    async send(text, opts) {
      if (!state.activeConversationID) return
      setState("busy", true)
      try {
        const res = await client().sendMessage(state.activeConversationID, text, opts)
        if (res?.run) {
          setState("run", res.run)
          setState("activeRunID", res.run.id)
          setState("runsByConversation", state.activeConversationID, res.run)
        }
        await refreshMessages()
      } finally {
        setState("busy", false)
      }
    },
    async cancelRun() {
      if (state.activeRunID) await client().cancel(state.activeRunID)
      await refreshRun()
    },
    async retryRun() {
      if (state.activeRunID) await client().retry(state.activeRunID)
      await refreshRun()
    },
    async integrateRun() {
      if (state.activeRunID) await client().integrate(state.activeRunID)
      await refreshRun()
    },
    async resolveApproval(id, status) {
      await client().resolveApproval(id, status)
      await refreshGlobal()
    },
    async readNotification(id) {
      await client().readNotification(id)
      await refreshGlobal()
    },
    refreshRun,
  }
}

const StateContext = createContext<StateAPI>()

export function StateProvider(props: ParentProps) {
  const api = createStateAPI()
  void api
    .hydrateCredentials()
    .then(() => api.refreshAll())
    .then(() => {
      // events are connected by the provider after initial load
    })
  return <StateContext.Provider value={api}>{props.children}</StateContext.Provider>
}

export function useWayshard(): StateAPI {
  const ctx = useContext(StateContext)
  if (!ctx) throw new Error("useWayshard must be used within StateProvider")
  return ctx
}
