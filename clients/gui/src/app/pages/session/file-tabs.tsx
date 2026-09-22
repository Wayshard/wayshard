// Wayshard session file tabs.
//
// Adapted from the imported OpenCode application session file tabs
// (third_party/opencode-v1.18.31/packages/app/src/pages/session/file-tabs.tsx):
// the behavior-bearing file-browser composition is retained — a Changes/All tab
// bar over the file tree, an ordered set of open file tabs with an active file,
// close/switch transitions that pick a neighbor, a per-file scroll position, and
// the file rendered through the inherited session-ui File component. Wayshard
// adds compare-and-set editing (expectedHash) on top of the inherited read-only
// viewer; the repository remains authoritative for source state.
import { For, Show, createEffect, createMemo, createResource, createSignal, type JSX } from "solid-js"
import { createStore } from "solid-js/store"
import { createMediaQuery } from "@solid-primitives/media"
import { Button } from "@wayshard/ui/button"
import { Icon } from "@wayshard/ui/icon"
import { Tag } from "@wayshard/ui/tag"
import { File as FileViewer } from "@wayshard/gui/session-ui/components/file"
import { useWayshard } from "../../../wayshard/state"
import { EmptyState } from "../../components/state-views"
import { FileTree } from "../../file-tree"
import {
  activateFile,
  closeFile,
  createFileTabState,
  openFile,
  setMode,
  setScroll,
  type FileTabState,
} from "./file-tab-model"

function Loading(props: { label?: string }): JSX.Element {
  return <div class="wh-loading">{props.label ?? "Loading…"}</div>
}

interface FileData {
  content: string
  hash: string
  binary: boolean
}

export function SessionFileTabs(): JSX.Element {
  const ws = useWayshard()
  const isDesktop = createMediaQuery("(min-width: 768px)")
  const [tabState, setTabState] = createStore<FileTabState>(createFileTabState("changes"))
  const [files, setFiles] = createStore<Record<string, FileData>>({})
  const [draft, setDraft] = createSignal("")
  const [editing, setEditing] = createSignal(false)
  const [status, setStatus] = createSignal("")
  const [showTree, setShowTree] = createSignal(true)
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

  const visiblePaths = createMemo(() => {
    const all = tree() ?? []
    if (tabState.mode === "all") return all
    const changedPaths = new Set(ws.state.runChanges)
    return all.filter((p) => changedPaths.has(p))
  })

  const active = () => tabState.active
  const activeData = () => (active() ? files[active()!] : undefined)

  async function load(path: string): Promise<void> {
    const id = projectID()
    if (!id) return
    if (files[path]) return
    try {
      const file = await ws.client().readFile(id, path)
      setFiles(path, { content: file.content, hash: file.hash, binary: file.binary })
    } catch (err) {
      setStatus(String(err))
    }
  }

  function open(path: string): void {
    setTabState((state) => openFile(state, path))
    setEditing(false)
    setStatus("")
    if (!isDesktop()) setShowTree(false)
    void load(path)
  }

  function activate(path: string): void {
    setTabState((state) => activateFile(state, path))
    setEditing(false)
    setStatus("")
    void load(path)
  }

  function close(path: string): void {
    setTabState((state) => closeFile(state, path))
    setEditing(false)
  }

  function beginEdit(): void {
    const data = activeData()
    if (!data || data.binary) return
    setDraft(data.content)
    setEditing(true)
    setStatus("")
  }

  async function save(): Promise<void> {
    const id = projectID()
    const path = active()
    const data = activeData()
    if (!id || !path || !data) return
    try {
      await ws.client().writeFile(id, path, draft(), data.hash)
      setStatus("Saved")
      setEditing(false)
      setFiles(path, "content", draft())
      void refetch()
    } catch (err) {
      setStatus(`Save rejected (stale hash or conflict): ${String(err)}`)
    }
  }

  // Restore the active file's scroll position when the active file changes.
  let viewport: HTMLDivElement | undefined
  createEffect(() => {
    const path = active()
    const el = viewport
    if (!path || !el) return
    const pos = tabState.scroll[path]
    if (!pos) return
    requestAnimationFrame(() => {
      el.scrollTop = pos.y
      el.scrollLeft = pos.x
    })
  })

  function handleScroll(event: Event & { currentTarget: HTMLDivElement }): void {
    const path = active()
    if (!path) return
    setTabState((state) => setScroll(state, path, { x: event.currentTarget.scrollLeft, y: event.currentTarget.scrollTop }))
  }

  const showTreeColumn = () => isDesktop() || showTree()

  return (
    <div data-slot="session-file-tabs" class="wh-panel flex min-h-0 flex-1 flex-col overflow-hidden">
      <div class="wh-panel-header shrink-0">
        <h2>Files</h2>
        <div class="flex items-center gap-2">
          <div class="wh-segmented" data-slot="file-tab-bar">
            <Button size="small" variant={tabState.mode === "changes" ? "primary" : "secondary"} onClick={() => setTabState((s) => setMode(s, "changes"))}>
              Changes
            </Button>
            <Button size="small" variant={tabState.mode === "all" ? "primary" : "secondary"} onClick={() => setTabState((s) => setMode(s, "all"))}>
              All
            </Button>
          </div>
          <Button size="small" variant="ghost" onClick={() => void refetch()}>
            Refresh
          </Button>
        </div>
      </div>
      <Show when={projectID()} fallback={<EmptyState title="No project selected" />}>
        <Show when={tabState.tabs.length}>
          <div data-slot="file-open-tabs" class="flex h-9 shrink-0 items-center gap-1 overflow-x-auto border-b border-v2-border-border-base px-2">
            <For each={tabState.tabs}>
              {(path) => (
                <div class="group flex shrink-0 items-center rounded-md" classList={{ "bg-v2-background-bg-layer-01": active() === path }}>
                  <button type="button" class="max-w-48 truncate px-2 py-1 text-sm" title={path} onClick={() => activate(path)}>
                    {path.split("/").pop()}
                  </button>
                  <button type="button" aria-label="Close file" class="px-1 opacity-0 group-hover:opacity-100" onClick={() => close(path)}>
                    <Icon name="close-small" size="small" />
                  </button>
                </div>
              )}
            </For>
          </div>
        </Show>
        <Show when={!tree.loading} fallback={<Loading />}>
          <div class="wh-files-layout min-h-0 flex-1">
            <Show when={showTreeColumn()}>
              <div class="wh-files-tree shrink-0 overflow-y-auto">
                <Show
                  when={visiblePaths().length}
                  fallback={<EmptyState title={tabState.mode === "changes" ? "No changed files" : "No files"} />}
                >
                  <FileTree paths={visiblePaths()} selected={active()} changed={changed()} onSelect={open} />
                </Show>
              </div>
            </Show>
            <div class="wh-files-view flex min-w-0 flex-1 flex-col">
              <Show
                when={active()}
                fallback={<EmptyState title="Select a file" body="Choose a file from the tree to view or edit." />}
              >
                <div class="wh-peek-header shrink-0">
                  <Show when={!isDesktop() && !showTree()}>
                    <Button size="small" variant="ghost" icon="chevron-right" onClick={() => setShowTree(true)}>
                      Files
                    </Button>
                  </Show>
                  <span class="wh-truncate">{active()}</span>
                  <Show when={activeData()?.binary}>
                    <Tag>binary</Tag>
                  </Show>
                  <Show when={!activeData()?.binary}>
                    <Show
                      when={editing()}
                      fallback={
                        <Button size="small" variant="secondary" onClick={beginEdit}>
                          Edit
                        </Button>
                      }
                    >
                      <Button size="small" variant="primary" onClick={() => void save()}>
                        Save
                      </Button>
                    </Show>
                  </Show>
                </div>
                <Show when={status()}>
                  <div class="wh-muted shrink-0 px-2 text-xs">{status()}</div>
                </Show>
                <div ref={viewport} class="min-h-0 flex-1 overflow-auto" onScroll={handleScroll}>
                  <Show when={activeData()} fallback={<Loading />}>
                    <Show when={!activeData()!.binary} fallback={<EmptyState title="Binary file" body="This file cannot be edited as text." />}>
                      <Show
                        when={editing()}
                        fallback={<FileViewer mode="text" file={{ name: active()!, contents: activeData()!.content }} />}
                      >
                        <textarea
                          class="wh-editor"
                          data-slot="file-editor"
                          value={draft()}
                          onInput={(e) => setDraft(e.currentTarget.value)}
                        />
                      </Show>
                    </Show>
                  </Show>
                </div>
              </Show>
            </div>
          </div>
        </Show>
      </Show>
    </div>
  )
}
