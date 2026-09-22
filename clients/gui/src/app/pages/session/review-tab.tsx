// Wayshard session review (Changes) tab.
//
// Adapted from the imported OpenCode application review tab
// (third_party/opencode-v1.18.31/packages/app/src/pages/session/review-tab.tsx):
// the session-scoped review composition now lives here (Run vs Workspace
// selection, changed-file list, and the diff/preview viewport), rendered through
// the adapted session-ui diff component. Wayshard semantics are preserved: Run
// changes = task-start snapshot → run final; Workspace changes = current source
// workspace state; pre-existing user changes are labelled, never attributed to
// the agent.
import { For, Show, createMemo, createResource, createSignal, type JSX } from "solid-js"
import { Button } from "@wayshard/ui/button"
import { Tag } from "@wayshard/ui/tag"
import { File as DiffFile } from "@wayshard/gui/session-ui/components/file"
import { useWayshard } from "../../../wayshard/state"
import { EmptyState, ErrorState } from "../../components/state-views"

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

function Loading(props: { label?: string }): JSX.Element {
  return <div class="wh-loading">{props.label ?? "Loading…"}</div>
}

export function SessionReviewTab(): JSX.Element {
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
    <div data-slot="session-review" class="wh-panel flex min-h-0 flex-1 flex-col overflow-y-auto">
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

function FilePeek(props: { path: string; mode: "run" | "workspace"; runID: string | null }): JSX.Element {
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
          <Show when={before() || after()} fallback={<EmptyState title="No diff available" />}>
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
