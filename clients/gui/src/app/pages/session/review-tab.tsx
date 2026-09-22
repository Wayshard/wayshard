// Wayshard session review (Changes) tab.
//
// Adapted from the imported OpenCode application review tab
// (third_party/opencode-v1.18.31/packages/app/src/pages/session/review-tab.tsx):
// the adapted session-ui SessionReview component is the production review
// surface. The upstream scroll persistence is retained (user-interaction
// cancellation, requestAnimationFrame restore, per-session scroll/open state),
// and the Run vs Workspace provenance selection is a Wayshard addition. Wayshard
// semantics: Run changes = task-start snapshot -> run final; pre-existing user
// changes are labelled, never attributed to the agent.
//
// Deliberate semantic divergence: Wayshard's workspace API exposes the current
// working-tree state but not a HEAD baseline, so Workspace mode shows current
// content through the inherited File component rather than synthesizing a diff.
// Review comments are not a Wayshard domain feature and are omitted.
import { For, Show, createEffect, createResource, createSignal, onCleanup, type JSX } from "solid-js"
import { Button } from "@wayshard/ui/button"
import { Tag } from "@wayshard/ui/tag"
import { File as FilePreview } from "@wayshard/gui/session-ui/components/file"
import { SessionReview, type SessionReviewDiffStyle } from "@wayshard/gui/session-ui/components/session-review"
import { useWayshard } from "../../../wayshard/state"
import { EmptyState, ErrorState } from "../../components/state-views"
import { hydrateRunDiffs, runDeltaToDiffs, workspaceEntries, type ReviewDiff, type WorkspaceEntry } from "./review-adapter"
import { createReviewView } from "./review-view"

function Loading(props: { label?: string }): JSX.Element {
  return <div class="wh-loading">{props.label ?? "Loading…"}</div>
}

export function SessionReviewTab(): JSX.Element {
  const ws = useWayshard()
  const [mode, setMode] = createSignal<"run" | "workspace">("run")
  const [diffStyle, setDiffStyle] = createSignal<SessionReviewDiffStyle>("unified")
  const view = createReviewView(() => ws.state.activeConversationID ?? "global")

  // Scroll restore state, adapted from upstream review-tab.
  let scroll: HTMLDivElement | undefined
  let restoreFrame: number | undefined
  let userInteracted = false
  let restored: { x: number; y: number } | undefined

  createEffect(() => view.sync())

  const [runDiffs] = createResource(
    () => (mode() === "run" ? ws.state.activeRunID : null),
    async (runID) => {
      const data = (await ws.client().runChanges(runID)) as { delta?: { files?: Array<Record<string, unknown>> } }
      return hydrateRunDiffs(ws.client(), runID, runDeltaToDiffs(data?.delta))
    },
  )

  const [workspaceFiles] = createResource(
    () => (mode() === "workspace" ? ws.state.activeProjectID : null),
    async (projectID) => {
      const data = (await ws.client().workspaceChanges(projectID)) as { files?: Record<string, { kind?: string }> }
      return workspaceEntries(data?.files)
    },
  )

  const [selectedWorkspace, setSelectedWorkspace] = createSignal<string | undefined>()

  const handleInteraction = () => {
    userInteracted = true
    if (restoreFrame !== undefined) {
      cancelAnimationFrame(restoreFrame)
      restoreFrame = undefined
    }
  }

  const doRestore = () => {
    restoreFrame = undefined
    const el = scroll
    if (!el || userInteracted) return
    if (el.clientHeight === 0 || el.clientWidth === 0) return
    const s = view.scroll()
    if (!s || (s.x === 0 && s.y === 0)) return
    const maxY = Math.max(0, el.scrollHeight - el.clientHeight)
    const maxX = Math.max(0, el.scrollWidth - el.clientWidth)
    const targetY = Math.min(s.y, maxY)
    const targetX = Math.min(s.x, maxX)
    if (el.scrollTop === targetY && el.scrollLeft === targetX) return
    if (el.scrollTop !== targetY) el.scrollTop = targetY
    if (el.scrollLeft !== targetX) el.scrollLeft = targetX
    restored = { x: el.scrollLeft, y: el.scrollTop }
  }

  const queueRestore = () => {
    if (userInteracted || restoreFrame !== undefined) return
    restoreFrame = requestAnimationFrame(doRestore)
  }

  const handleScroll = (event: Event & { currentTarget: HTMLDivElement }) => {
    const el = event.currentTarget
    const prev = restored
    if (prev && el.scrollTop === prev.y && el.scrollLeft === prev.x) {
      restored = undefined
      return
    }
    restored = undefined
    handleInteraction()
    if (el.clientHeight === 0 || el.clientWidth === 0) return
    view.setScroll({ x: el.scrollLeft, y: el.scrollTop })
  }

  // A new session/run resets interaction tracking so its persisted scroll is
  // restored rather than suppressed by the previous session's scrolling.
  createEffect(() => {
    ws.state.activeConversationID
    ws.state.activeRunID
    userInteracted = false
  })

  createEffect(() => {
    runDiffs()
    diffStyle()
    queueRestore()
  })

  onCleanup(() => {
    if (restoreFrame !== undefined) cancelAnimationFrame(restoreFrame)
  })

  const diffs = () => (runDiffs() ?? []) as ReviewDiff[]
  const preExistingCount = () => diffs().filter((d) => d.preExisting && !d.agentModified).length

  return (
    <div data-slot="session-review-tab" class="flex min-h-0 flex-1 flex-col">
      <div class="wh-panel-header shrink-0">
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
      <Show when={mode() === "run"}>
        <Show when={ws.state.activeRunID} fallback={<EmptyState title="No run selected" body="Send a task to create a run." />}>
          <Show when={!runDiffs.loading} fallback={<Loading label="Loading run changes…" />}>
            <Show when={runDiffs.error} fallback={
              <SessionReview
                class="min-h-0 flex-1"
                diffs={diffs()}
                diffStyle={diffStyle()}
                onDiffStyleChange={setDiffStyle}
                open={view.open()}
                onOpenChange={view.setOpen}
                empty={<EmptyState title="No run changes" body="The run produced no source changes." />}
                title={
                  <span class="flex items-center gap-2">
                    Run changes
                    <Show when={preExistingCount()}>
                      <Tag>user baseline {preExistingCount()}</Tag>
                    </Show>
                  </span>
                }
                scrollRef={(el) => {
                  scroll = el
                  queueRestore()
                }}
                onScroll={handleScroll}
                onDiffRendered={queueRestore}
                readFile={async (path) => {
                  const runID = ws.state.activeRunID
                  if (!runID) return undefined
                  const file = await ws.client().runFile(runID, path, "run")
                  return { path, content: file.content, hash: file.hash, binary: file.binary }
                }}
              />
            }>
              <ErrorState title="Unable to load run changes" detail={String(runDiffs.error)} />
            </Show>
          </Show>
        </Show>
      </Show>
      <Show when={mode() === "workspace"}>
        <Show when={ws.state.activeProjectID} fallback={<EmptyState title="No project selected" />}>
          <Show when={!workspaceFiles.loading} fallback={<Loading label="Loading workspace changes…" />}>
            <WorkspaceChanges
              entries={workspaceFiles() ?? []}
              selected={selectedWorkspace()}
              onSelect={setSelectedWorkspace}
              projectID={ws.state.activeProjectID!}
            />
          </Show>
        </Show>
      </Show>
    </div>
  )
}

function WorkspaceChanges(props: {
  entries: WorkspaceEntry[]
  selected: string | undefined
  onSelect: (path: string) => void
  projectID: string
}): JSX.Element {
  const ws = useWayshard()
  const [file] = createResource(
    () => (props.selected ? { id: props.projectID, path: props.selected } : null),
    (args) => ws.client().readFile(args.id, args.path),
  )
  return (
    <div data-slot="session-review-workspace" class="flex min-h-0 flex-1">
      <div class="wh-files-tree shrink-0 overflow-y-auto">
        <Show when={props.entries.length} fallback={<EmptyState title="Workspace clean" />}>
          <ul class="wh-file-list">
            <For each={props.entries}>
              {(entry) => (
                <li>
                  <button class="wh-file-row" data-kind={entry.kind} onClick={() => props.onSelect(entry.path)}>
                    <Tag>{entry.kind}</Tag>
                    <span class="wh-truncate">{entry.path}</span>
                  </button>
                </li>
              )}
            </For>
          </ul>
        </Show>
      </div>
      <div class="wh-files-view min-w-0 flex-1 overflow-auto">
        <Show when={props.selected} fallback={<EmptyState title="Select a file" body="Choose a workspace file to view." />}>
          <Show when={!file.loading} fallback={<Loading />}>
            <Show when={file()} fallback={<ErrorState title="Unable to read file" />}>
              <Show
                when={!file()!.binary}
                fallback={<EmptyState title="Binary file" body="This file cannot be shown as text." />}
              >
                <FilePreview mode="text" file={{ name: props.selected!, contents: file()!.content }} />
              </Show>
            </Show>
          </Show>
        </Show>
      </div>
    </div>
  )
}
