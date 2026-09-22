// Wayshard view surfaces. Each surface is a real layout with loading, empty and
// error states, actions and no normal dependence on raw JSON (a debug inspector
// is opt-in).
import { For, Match, Show, Switch, createMemo, createResource, createSignal, onCleanup, type JSX } from "solid-js"
import { Button } from "@wayshard/ui/button"
import { Icon } from "@wayshard/ui/icon"
import { Tag } from "@wayshard/ui/tag"
import { Spinner } from "@wayshard/ui/spinner"
import { TextField } from "@wayshard/ui/text-field"
import { EmptyState, ErrorState } from "./components/state-views"
import { File as DiffFile } from "@wayshard/gui/session-ui/components/file"
import { FileTree } from "./file-tree"
import { Terminal } from "./terminal"
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

function isPlainObject(v: unknown): v is Record<string, unknown> {
  return typeof v === "object" && v !== null && !Array.isArray(v)
}

function labelize(key: string): string {
  return key
    .replace(/([a-z])([A-Z])/g, "$1 $2")
    .replace(/[_-]+/g, " ")
    .replace(/^./, (c) => c.toUpperCase())
}

function Scalar(props: { value: unknown }) {
  const v = props.value
  if (v === null || v === undefined || v === "") return <span class="wh-muted">—</span>
  if (typeof v === "boolean") return <Tag>{v ? "yes" : "no"}</Tag>
  if (Array.isArray(v)) return <span>{v.map((x) => String(x)).join(", ") || "—"}</span>
  return <span>{String(v)}</span>
}

// Structured renders an object as labeled sections, lists and tables rather than
// a raw JSON dump. Raw data remains behind the explicit debug inspector.
export function Structured(props: { value: unknown; depth?: number }) {
  const depth = () => props.depth ?? 0
  return (
    <Show when={isPlainObject(props.value)} fallback={<pre class="wh-json">{JSON.stringify(props.value, null, 2)}</pre>}>
      <div class="wh-structured">
        <For each={Object.entries(props.value as Record<string, unknown>)}>
          {([key, value]) => (
            <Show when={value !== undefined && value !== null && !(Array.isArray(value) && value.length === 0)}>
              <div class="wh-structured-section" data-depth={depth()}>
                <div class="wh-structured-label">{labelize(key)}</div>
                <Show
                  when={Array.isArray(value) && value.length > 0 && isPlainObject(value[0])}
                  fallback={
                    <Show when={isPlainObject(value)} fallback={<div class="wh-structured-value"><Scalar value={value} /></div>}>
                      <Structured value={value} depth={depth() + 1} />
                    </Show>
                  }
                >
                  <Table
                    columns={Array.from(new Set((value as Record<string, unknown>[]).flatMap((row) => Object.keys(row)))).slice(0, 6)}
                    rows={(value as Record<string, unknown>[]).slice(0, 200).map((row) =>
                      Array.from(new Set((value as Record<string, unknown>[]).flatMap((r) => Object.keys(r))))
                        .slice(0, 6)
                        .map((c) => String(row[c] ?? "")),
                    )}
                  />
                </Show>
              </div>
            </Show>
          )}
        </For>
      </div>
    </Show>
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
        <FilePeek path={selected()!} mode={mode()} runID={runID()} />
      </Show>
    </div>
  )
}

function FilePeek(props: { path: string; mode: "run" | "workspace"; runID: string | null }) {
  const ws = useWayshard()
  const [before] = createResource(
    () => (props.mode === "run" && props.runID ? { runID: props.runID, path: props.path } : null),
    (args) => ws.client().runFile(args.runID, args.path, "snapshot"),
  )
  const [after] = createResource(
    () => (props.mode === "run" && props.runID ? { runID: props.runID, path: props.path } : null),
    (args) => ws.client().runFile(args.runID, args.path, "run"),
  )
  const [workspace] = createResource(
    () => (props.mode === "workspace" && ws.state.activeProjectID ? { id: ws.state.activeProjectID, path: props.path } : null),
    (args) => ws.client().readFile(args.id, args.path),
  )

  return (
    <div class="wh-peek">
      <div class="wh-peek-header">
        <span class="wh-truncate">{props.path}</span>
        <Show when={props.mode === "run"}>
          <Tag>run start → run final</Tag>
        </Show>
        <Show when={props.mode === "workspace"}>
          <Tag>current workspace</Tag>
        </Show>
      </div>
      <Show when={props.mode === "run"}>
        <Show when={!before.loading && !after.loading} fallback={<Loading label="Loading diff…" />}>
          <Show
            when={before() || after()}
            fallback={<EmptyState title="No diff available" />}
          >
            <Show
              when={!(before()?.binary || after()?.binary)}
              fallback={<EmptyState title="Binary change" body="This file cannot be shown as text." />}
            >
              <div class="wh-diff">
                <DiffFile
                  mode="diff"
                  before={{ name: props.path, contents: before()?.content ?? "" }}
                  after={{ name: props.path, contents: after()?.content ?? "" }}
                />
              </div>
            </Show>
          </Show>
        </Show>
      </Show>
      <Show when={props.mode === "workspace"}>
        <Show when={!workspace.loading} fallback={<Loading />}>
          <Show when={workspace()} fallback={<ErrorState title="Unable to read file" />}>
            <Show
              when={!workspace()!.binary}
              fallback={<EmptyState title="Binary file" body="This file cannot be shown as text." />}
            >
              <pre class="wh-file-view">{workspace()!.content}</pre>
            </Show>
          </Show>
        </Show>
      </Show>
    </div>
  )
}

/* ----------------------------------- Files --------------------------------- */

export function FilesView() {
  const ws = useWayshard()
  const [selected, setSelected] = createSignal<string | null>(null)
  const [content, setContent] = createSignal("")
  const [hash, setHash] = createSignal("")
  const [binary, setBinary] = createSignal(false)
  const [status, setStatus] = createSignal("")
  const projectID = () => ws.state.activeProjectID

  const [tree, { refetch }] = createResource(
    () => projectID(),
    async (id) => {
      const out: string[] = []
      async function walk(dir: string, depth: number) {
        if (depth > 6 || out.length > 2000) return
        const entries = await ws.client().listFiles(id, dir)
        for (const e of entries) {
          const p = dir ? `${dir}/${e.name}` : e.name
          if (e.dir) await walk(p, depth + 1)
          else out.push(p)
        }
      }
      await walk("", 0)
      return out.sort()
    },
  )

  const changed = createMemo<Record<string, string>>(() => {
    const map: Record<string, string> = {}
    for (const path of ws.state.runChanges) map[path] = "changed"
    return map
  })

  async function open(p: string) {
    if (!projectID()) return
    try {
      const file = await ws.client().readFile(projectID()!, p)
      setSelected(p)
      setContent(file.content)
      setHash(file.hash)
      setBinary(file.binary)
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
        <Button size="small" variant="ghost" onClick={() => void refetch()}>
          Refresh
        </Button>
      </div>
      <Show when={projectID()} fallback={<EmptyState title="No project selected" />}>
        <Show when={!tree.loading} fallback={<Loading />}>
          <div class="wh-files-layout">
            <div class="wh-files-tree">
              <FileTree paths={tree() ?? []} selected={selected() ?? undefined} changed={changed()} onSelect={(p) => void open(p)} />
            </div>
            <div class="wh-files-view">
              <Show when={selected()} fallback={<EmptyState title="Select a file" body="Choose a file from the tree to view or edit." />}>
                <div class="wh-peek-header">
                  <span class="wh-truncate">{selected()}</span>
                  <Button size="small" variant="primary" onClick={() => void save()} disabled={binary()}>
                    Save
                  </Button>
                </div>
                <Show when={!binary()} fallback={<EmptyState title="Binary file" body="This file cannot be edited as text." />}>
                  <textarea class="wh-editor" value={content()} onInput={(e) => setContent(e.currentTarget.value)} />
                </Show>
                <Show when={status()}>
                  <div class="wh-muted">{status()}</div>
                </Show>
              </Show>
            </div>
          </div>
        </Show>
      </Show>
    </div>
  )
}

/* --------------------------------- Terminal -------------------------------- */

export function TerminalView() {
  const ws = useWayshard()
  return (
    <Show when={ws.state.activeProjectID} fallback={<EmptyState title="No project selected" />}>
      <Terminal projectId={ws.state.activeProjectID!} />
    </Show>
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
        <Structured value={context()} />
      </Show>
      <DebugInspector value={context() ?? null} />
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
          <Structured value={knowledge()} />
        </Show>
      </Show>
      <DebugInspector value={knowledge() ?? null} />
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
    <Panel title="Recovery & diagnostics" hint="Sandbox, provider networking, storage pressure and server health.">
      <h3>Sandbox capability</h3>
      <Show when={!sandbox.loading} fallback={<Loading />}>
        <Structured value={sandbox()} />
      </Show>
      <h3>Storage</h3>
      <Show when={!storage.loading} fallback={<Loading />}>
        <Structured value={storage()} />
      </Show>
      <DebugInspector value={{ sandbox: sandbox() ?? null, storage: storage() ?? null }} />
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
