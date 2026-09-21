// Wayshard view surfaces. Each surface is a real layout with loading, empty and
// error states, actions and no normal dependence on raw JSON (a debug inspector
// is opt-in).
import { For, Match, Show, Switch, createMemo, createResource, createSignal, onCleanup, type JSX } from "solid-js"
import { Button } from "@wayshard/ui/button"
import { Icon } from "@wayshard/ui/icon"
import { Tag } from "@wayshard/ui/tag"
import { Spinner } from "@wayshard/ui/spinner"
import { TextField } from "@wayshard/ui/text-field"
import { EmptyState, ErrorState } from "./App"
import { useWayshard } from "../wayshard/state"

export type AdvancedSurfaceKey =
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

export function AdvancedSurface(props: { view: AdvancedSurfaceKey }) {
  return (
    <Switch>
      <Match when={props.view === "usage"}>
        <UsageView />
      </Match>
      <Match when={props.view === "routing"}>
        <RoutingView />
      </Match>
      <Match when={props.view === "context"}>
        <ContextView />
      </Match>
      <Match when={props.view === "artifacts"}>
        <ArtifactsView />
      </Match>
      <Match when={props.view === "knowledge"}>
        <KnowledgeView />
      </Match>
      <Match when={props.view === "harnesses"}>
        <HarnessesView />
      </Match>
      <Match when={props.view === "recovery"}>
        <RecoveryView />
      </Match>
      <Match when={props.view === "approvals"}>
        <ApprovalsView />
      </Match>
      <Match when={props.view === "notifications"}>
        <NotificationsView />
      </Match>
      <Match when={props.view === "settings"}>
        <SettingsView />
      </Match>
    </Switch>
  )
}

function Loading(props: { label?: string }) {
  return (
    <div class="wh-loading">
      <Spinner />
      <span class="wh-muted">{props.label ?? "Loading…"}</span>
    </div>
  )
}

function DebugInspector(props: { value: unknown }) {
  const [open, setOpen] = createSignal(false)
  return (
    <div class="wh-debug">
      <Button size="small" variant="ghost" onClick={() => setOpen(!open())}>
        {open() ? "Hide raw data" : "Inspect raw data"}
      </Button>
      <Show when={open()}>
        <pre class="wh-json">{JSON.stringify(props.value, null, 2)}</pre>
      </Show>
    </div>
  )
}

/* ---------------------------------- Changes --------------------------------- */

type RunDelta = {
  files?: Array<{
    path: string
    kind: string
    agentModified: boolean
    preExisting: boolean
    before?: { missing?: boolean; size?: number }
    after?: { missing?: boolean; size?: number }
  }>
}

export function ChangesView() {
  const ws = useWayshard()
  const [mode, setMode] = createSignal<"run" | "workspace">("run")
  const [selected, setSelected] = createSignal<string | null>(null)
  const projectID = () => ws.state.activeProjectID
  const runID = () => ws.state.activeRunID

  const [workspaceChanges] = createResource(
    () => (mode() === "workspace" ? projectID() : null),
    (id) => ws.client().workspaceChanges(id!),
  )
  const [runChanges] = createResource(
    () => (mode() === "run" && runID() ? runID() : null),
    (id) => ws.client().runChanges(id!),
  )

  const runFiles = createMemo<RunDelta["files"]>(() => {
    const data = runChanges() as { delta?: RunDelta } | undefined
    return data?.delta?.files ?? []
  })

  const workspaceFiles = createMemo<Array<{ path: string; meta?: unknown }>>(() => {
    const data = workspaceChanges() as { files?: Record<string, unknown> } | undefined
    const files = data?.files ?? {}
    return Object.keys(files).map((path) => ({ path, meta: files[path] }))
  })

  return (
    <div class="wh-panel">
      <div class="wh-panel-header">
        <h2>Changes</h2>
        <div class="wh-segmented">
          <Button size="small" variant={mode() === "run" ? "primary" : "secondary"} onClick={() => setMode("run")}>
            Run changes
          </Button>
          <Button size="small" variant={mode() === "workspace" ? "primary" : "secondary"} onClick={() => setMode("workspace")}>
            Workspace changes
          </Button>
        </div>
      </div>
      <p class="wh-muted">
        {mode() === "run"
          ? "Changes produced by this run (task-start snapshot → run workspace). User changes are never attributed to the agent."
          : "Current source workspace state. Use a Git client for staging and commits."}
      </p>
      <Show when={mode() === "run"}>
        <Show when={runID()} fallback={<EmptyState title="No run selected" body="Send a task to create a run." />}>
          <Show when={!runChanges.loading} fallback={<Loading />}>
            <Show when={runFiles()!.length} fallback={<EmptyState title="No run changes" body="The run produced no source changes." />}>
              <ul class="wh-file-list">
                <For each={runFiles()}>
                  {(f) => (
                    <li>
                      <button class="wh-file-row" data-kind={f.kind} onClick={() => setSelected(f.path)}>
                        <Tag>{f.kind}</Tag>
                        <span class="wh-truncate">{f.path}</span>
                        <Show when={f.preExisting}>
                          <Tag>user baseline</Tag>
                        </Show>
                      </button>
                    </li>
                  )}
                </For>
              </ul>
            </Show>
          </Show>
        </Show>
      </Show>
      <Show when={mode() === "workspace"}>
        <Show when={projectID()} fallback={<EmptyState title="No project selected" />}>
          <Show when={!workspaceChanges.loading} fallback={<Loading />}>
            <Show when={workspaceFiles()!.length} fallback={<EmptyState title="Workspace clean" />}>
              <ul class="wh-file-list">
                <For each={workspaceFiles()}>
                  {(f) => (
                    <li>
                      <button class="wh-file-row" onClick={() => setSelected(f.path)}>
                        <span class="wh-truncate">{f.path}</span>
                      </button>
                    </li>
                  )}
                </For>
              </ul>
            </Show>
          </Show>
        </Show>
      </Show>
      <Show when={selected() && projectID()}>
        <FilePeek path={selected()!} />
      </Show>
    </div>
  )
}

function FilePeek(props: { path: string }) {
  const ws = useWayshard()
  const [file] = createResource(
    () => ({ id: ws.state.activeProjectID!, path: props.path }),
    (args) => ws.client().readFile(args.id, args.path),
  )
  return (
    <div class="wh-peek">
      <div class="wh-peek-header">{props.path}</div>
      <Show when={!file.loading} fallback={<Loading />}>
        <Show when={file()} fallback={<ErrorState title="Unable to read file" />}>
          <Show when={(file() as { binary?: boolean }).binary} fallback={<pre class="wh-file-view">{(file() as { content: string }).content}</pre>}>
            <EmptyState title="Binary file" body="This file cannot be shown as text." />
          </Show>
        </Show>
      </Show>
    </div>
  )
}

/* ----------------------------------- Files --------------------------------- */

export function FilesView() {
  const ws = useWayshard()
  const [path, setPath] = createSignal("")
  const [selected, setSelected] = createSignal<string | null>(null)
  const [content, setContent] = createSignal("")
  const [hash, setHash] = createSignal("")
  const [status, setStatus] = createSignal("")
  const projectID = () => ws.state.activeProjectID

  const [entries, { refetch }] = createResource(
    () => (projectID() ? { id: projectID()!, path: path() } : null),
    (args) => ws.client().listFiles(args.id, args.path),
  )

  async function open(p: string) {
    if (!projectID()) return
    try {
      const file = await ws.client().readFile(projectID()!, p)
      setSelected(p)
      setContent(file.content)
      setHash(file.hash)
      setStatus("")
    } catch (err) {
      setStatus(String(err))
    }
  }

  async function save() {
    if (!projectID() || !selected()) return
    try {
      await ws.client().writeFile(projectID()!, selected()!, content(), hash())
      setStatus("Saved")
      void refetch()
    } catch (err) {
      setStatus(`Save rejected (stale hash or conflict): ${String(err)}`)
    }
  }

  return (
    <div class="wh-panel">
      <div class="wh-panel-header">
        <h2>Files</h2>
        <div class="wh-breadcrumb">
          <Button size="small" variant="ghost" onClick={() => setPath("")}>
            root
          </Button>
          <span class="wh-muted">/{path()}</span>
        </div>
      </div>
      <Show when={projectID()} fallback={<EmptyState title="No project selected" />}>
        <Show when={!entries.loading} fallback={<Loading />}>
          <ul class="wh-file-list">
            <For each={entries() ?? []}>
              {(e) => (
                <li>
                  <button
                    class="wh-file-row"
                    onClick={() => (e.dir ? setPath(path() ? `${path()}/${e.name}` : e.name) : void open(path() ? `${path()}/${e.name}` : e.name))}
                  >
                    <Icon name={e.dir ? "bullet-list" : "console"} size="small" />
                    <span class="wh-truncate">{e.name}</span>
                  </button>
                </li>
              )}
            </For>
          </ul>
        </Show>
      </Show>
      <Show when={selected()}>
        <div class="wh-peek">
          <div class="wh-peek-header">
            <span>{selected()}</span>
            <Button size="small" variant="primary" onClick={() => void save()}>
              Save
            </Button>
          </div>
          <textarea class="wh-editor" value={content()} onInput={(e) => setContent(e.currentTarget.value)} />
          <Show when={status()}>
            <div class="wh-muted">{status()}</div>
          </Show>
        </div>
      </Show>
    </div>
  )
}

/* --------------------------------- Terminal -------------------------------- */

export function TerminalView() {
  const ws = useWayshard()
  const [terminalID, setTerminalID] = createSignal<string | null>(null)
  const [output, setOutput] = createSignal("")
  const [input, setInput] = createSignal("")
  const [lost, setLost] = createSignal(false)
  let socket: WebSocket | undefined

  onCleanup(() => socket?.close())

  async function start() {
    if (!ws.state.activeProjectID) return
    const t = await ws.client().startTerminal(ws.state.activeProjectID)
    setTerminalID(t.id)
    setLost(false)
    socket = ws.client().ptySocket(t.id)
    socket.onmessage = (ev) => setOutput((o) => o + String(ev.data))
    socket.onclose = () => setLost(true)
    socket.onerror = () => setLost(true)
  }

  function send() {
    if (!socket || socket.readyState !== WebSocket.OPEN) return
    socket.send(input())
    setInput("")
  }

  return (
    <div class="wh-panel">
      <div class="wh-panel-header">
        <h2>Terminal</h2>
        <Button size="small" variant="primary" onClick={() => void start()} disabled={!ws.state.activeProjectID}>
          New terminal
        </Button>
      </div>
      <p class="wh-muted">Terminals are server-owned PTYs. The client never executes local shell commands.</p>
      <Show when={terminalID()} fallback={<EmptyState title="No terminal attached" body="Create a server-owned terminal." />}>
        <Show when={lost()}>
          <ErrorState title="Terminal lost" detail="The server restarted or the PTY closed. Create a new terminal." />
        </Show>
        <pre class="wh-terminal">{output()}</pre>
        <div class="wh-composer-actions">
          <TextField value={input()} onInput={(e: InputEvent) => setInput((e.currentTarget as HTMLInputElement).value)} onKeyDown={(e: KeyboardEvent) => e.key === "Enter" && send()} />
          <Button size="small" onClick={send}>
            Send
          </Button>
        </div>
      </Show>
    </div>
  )
}

/* --------------------------- Run-scoped surfaces --------------------------- */

function useRunResource<T>(fn: (client: ReturnType<typeof useWayshard>["client"] extends () => infer C ? C : never, runID: string) => Promise<T>) {
  const ws = useWayshard()
  return createResource(
    () => ws.state.activeRunID,
    (runID) => fn(ws.client() as never, runID),
  )
}

export function UsageView() {
  const ws = useWayshard()
  const [usage] = useRunResource((client, runID) => client.usage(runID))
  return (
    <Panel title="Usage" hint="Token, cache and cost accounting per stage.">
      <Show when={ws.state.activeRunID} fallback={<EmptyState title="No run selected" />}>
        <Show when={!usage.loading} fallback={<Loading />}>
          <Show when={(usage() ?? []).length} fallback={<EmptyState title="No usage recorded" />}>
            <Table
              columns={["stage", "harness", "model", "input", "output", "cache", "cost"]}
              rows={(usage() as Array<Record<string, unknown>>).map((u) => [
                String(u.stageId ?? u.stage ?? ""),
                String(u.harnessId ?? u.harness ?? ""),
                String(u.modelId ?? u.model ?? ""),
                String(u.inputTokens ?? ""),
                String(u.outputTokens ?? ""),
                String(u.cacheReadTokens ?? u.cache ?? ""),
                String(u.costMicros ?? u.cost ?? ""),
              ])}
            />
          </Show>
        </Show>
      </Show>
      <DebugInspector value={usage() ?? null} />
    </Panel>
  )
}

export function RoutingView() {
  const ws = useWayshard()
  const [routes] = useRunResource((client, runID) => client.routes(runID))
  const [harnesses] = createResource(() => ws.client().harnesses())
  return (
    <Panel title="Routing" hint="Harness/model selection, health and fallback.">
      <Show when={ws.state.activeRunID} fallback={<EmptyState title="No run selected" />}>
        <Show when={!routes.loading} fallback={<Loading />}>
          <Show when={(routes() ?? []).length} fallback={<EmptyState title="No route decisions yet" />}>
            <Table
              columns={["stage", "harness", "model", "reason", "degraded"]}
              rows={(routes() as Array<Record<string, unknown>>).map((r) => [
                String(r.stageId ?? r.stage ?? ""),
                String(r.harnessId ?? ""),
                String(r.modelId ?? ""),
                String(r.reason ?? ""),
                String(r.degraded ?? false),
              ])}
            />
          </Show>
        </Show>
      </Show>
      <h3>Harness inventory</h3>
      <Show when={!harnesses.loading} fallback={<Loading />}>
        <Table
          columns={["definition", "executable", "bridge", "acp", "health", "transport"]}
          rows={((harnesses() ?? []) as Array<Record<string, unknown>>).map((h) => [
            String(h.definitionId ?? ""),
            String(h.executable ?? ""),
            String(h.bridgeExecutable ?? ""),
            String(h.acpStatus ?? ""),
            String(h.health ?? ""),
            String(h.providerTransport ?? ""),
          ])}
        />
      </Show>
      <DebugInspector value={{ routes: routes() ?? null, harnesses: harnesses() ?? null }} />
    </Panel>
  )
}

export function ContextView() {
  const [context] = useRunResource((client, runID) => client.context(runID))
  return (
    <Panel title="Context" hint="The context bundle delivered to the planning stage.">
      <Show when={context()} fallback={<EmptyState title="No context manifest available" />}>
        <pre class="wh-json">{JSON.stringify(context(), null, 2)}</pre>
      </Show>
    </Panel>
  )
}

export function ArtifactsView() {
  const ws = useWayshard()
  const [selected, setSelected] = createSignal<string | null>(null)
  const artifacts = () => ws.state.artifacts
  return (
    <Panel title="Artifacts" hint="Structured stage outputs, validated server-side.">
      <Show when={artifacts().length} fallback={<EmptyState title="No artifacts" />}>
        <ul class="wh-file-list">
          <For each={artifacts()}>
            {(a) => (
              <li>
                <button class="wh-file-row" data-valid={a.valid} onClick={() => setSelected(a.id)}>
                  <Tag>{a.kind}</Tag>
                  <span>{a.valid ? "valid" : "invalid"}</span>
                </button>
              </li>
            )}
          </For>
        </ul>
        <Show when={selected()}>
          <pre class="wh-json">{JSON.stringify(artifacts().find((a) => a.id === selected()), null, 2)}</pre>
        </Show>
      </Show>
    </Panel>
  )
}

export function KnowledgeView() {
  const ws = useWayshard()
  const [knowledge] = createResource(
    () => ws.state.activeProjectID,
    (id) => ws.client().knowledge(id!),
  )
  return (
    <Panel title="Project knowledge" hint="Repository-owned instruction and specification documents.">
      <Show when={ws.state.activeProjectID} fallback={<EmptyState title="No project selected" />}>
        <Show when={!knowledge.loading} fallback={<Loading />}>
          <pre class="wh-json">{JSON.stringify(knowledge(), null, 2)}</pre>
        </Show>
      </Show>
    </Panel>
  )
}

export function HarnessesView() {
  const ws = useWayshard()
  const [installations] = createResource(() => ws.client().harnesses())
  const [definitions] = createResource(() => ws.client().get<{ definitions: Array<Record<string, unknown>>; diagnostics: unknown[] }>("/v1/harness-definitions"))
  return (
    <Panel title="Harnesses" hint="Definitions are edited through the user TOML catalog; there is no editor here.">
      <h3>Installations</h3>
      <Show when={!installations.loading} fallback={<Loading />}>
        <Table
          columns={["definition", "source", "executable", "bridge", "version", "acp", "auth", "transport", "blocking"]}
          rows={((installations() ?? []) as Array<Record<string, unknown>>).map((h) => [
            String(h.definitionId ?? ""),
            String(h.definitionSource ?? ""),
            String(h.executable ?? ""),
            String(h.bridgeExecutable ?? ""),
            String(h.version ?? ""),
            String(h.acpStatus ?? ""),
            String(h.authStatus ?? ""),
            String(h.providerTransport ?? ""),
            String(h.blockingReason ?? ""),
          ])}
        />
      </Show>
      <h3>Effective catalog</h3>
      <Show when={!definitions.loading} fallback={<Loading />}>
        <Table
          columns={["id", "source", "enabled", "acp", "executables", "bridges"]}
          rows={(definitions()?.definitions ?? []).map((d) => [
            String(d.id ?? ""),
            String(d.source ?? ""),
            String(d.enabled ?? ""),
            String(d.acp ?? ""),
            String((d.executables as string[])?.join(", ") ?? ""),
            String((d.bridges as string[])?.join(", ") ?? ""),
          ])}
        />
      </Show>
    </Panel>
  )
}

export function RecoveryView() {
  const ws = useWayshard()
  const [storage] = createResource(() => ws.client().storage())
  const [sandbox] = createResource(() => ws.client().sandbox())
  return (
    <Panel title="Recovery & diagnostics" hint="Storage pressure, sandbox capability and server status.">
      <h3>Sandbox</h3>
      <Show when={!sandbox.loading} fallback={<Loading />}>
        <pre class="wh-json">{JSON.stringify(sandbox(), null, 2)}</pre>
      </Show>
      <h3>Storage</h3>
      <Show when={!storage.loading} fallback={<Loading />}>
        <pre class="wh-json">{JSON.stringify(storage(), null, 2)}</pre>
      </Show>
    </Panel>
  )
}

export function ApprovalsView() {
  const ws = useWayshard()
  return (
    <Panel title="Approvals" hint="Durable server-owned approvals. Approving is policy, not a sandbox bypass.">
      <Show when={ws.state.approvals.length} fallback={<EmptyState title="No pending approvals" />}>
        <ul class="wh-file-list">
          <For each={ws.state.approvals}>
            {(a) => (
              <li class="wh-approval">
                <div>
                  <div>
                    <Tag>{a.kind}</Tag> {a.resource}
                  </div>
                  <div class="wh-muted">{a.reason}</div>
                  <div class="wh-muted">run {a.runId}</div>
                </div>
                <div class="wh-approval-actions">
                  <Button size="small" variant="primary" onClick={() => void ws.resolveApproval(a.id, "allowed")}>
                    Allow
                  </Button>
                  <Button size="small" variant="secondary" onClick={() => void ws.resolveApproval(a.id, "denied")}>
                    Deny
                  </Button>
                </div>
              </li>
            )}
          </For>
        </ul>
      </Show>
    </Panel>
  )
}

export function NotificationsView() {
  const ws = useWayshard()
  return (
    <Panel title="Notifications" hint="Run, approval and recovery attention.">
      <Show when={ws.state.notifications.length} fallback={<EmptyState title="No notifications" />}>
        <ul class="wh-file-list">
          <For each={ws.state.notifications}>
            {(n) => (
              <li class="wh-approval">
                <div>
                  <div>
                    <Show when={n.attention}>
                      <Tag>attention</Tag>
                    </Show>{" "}
                    {n.title}
                  </div>
                  <div class="wh-muted">{n.body}</div>
                </div>
                <Button size="small" variant="ghost" onClick={() => void ws.readNotification(n.id)}>
                  Mark read
                </Button>
              </li>
            )}
          </For>
        </ul>
      </Show>
    </Panel>
  )
}

export function SettingsView() {
  const ws = useWayshard()
  const [baseUrl, setBaseUrl] = createSignal(ws.state.connection.baseUrl)
  const [token, setToken] = createSignal(ws.state.connection.token)
  return (
    <Panel title="Settings" hint="Client connection and appearance. Security invariants are not configurable here.">
      <h3>Server connection</h3>
      <div class="wh-form">
        <label>
          Server URL
          <TextField value={baseUrl()} onInput={(e) => setBaseUrl(e.currentTarget.value)} />
        </label>
        <label>
          Device token
          <TextField value={token()} onInput={(e) => setToken(e.currentTarget.value)} type="password" />
        </label>
        <Button size="small" variant="primary" onClick={() => ws.setConnection({ baseUrl: baseUrl(), token: token() })}>
          Save & reconnect
        </Button>
      </div>
      <h3>Appearance</h3>
      <p class="wh-muted">Theme and typography are inherited from the Wayshard design system.</p>
    </Panel>
  )
}

/* ---------------------------------- shared --------------------------------- */

function Panel(props: { title: string; hint?: string; children: JSX.Element }) {
  return (
    <div class="wh-panel">
      <div class="wh-panel-header">
        <h2>{props.title}</h2>
      </div>
      <Show when={props.hint}>
        <p class="wh-muted">{props.hint}</p>
      </Show>
      {props.children}
    </div>
  )
}

function Table(props: { columns: string[]; rows: string[][] }) {
  return (
    <table class="wh-table">
      <thead>
        <tr>
          <For each={props.columns}>{(c) => <th>{c}</th>}</For>
        </tr>
      </thead>
      <tbody>
        <For each={props.rows}>
          {(row) => (
            <tr>
              <For each={row}>{(cell) => <td class="wh-truncate">{cell}</td>}</For>
            </tr>
          )}
        </For>
      </tbody>
    </table>
  )
}
