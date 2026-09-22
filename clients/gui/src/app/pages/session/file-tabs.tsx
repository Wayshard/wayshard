// Wayshard session file tabs.
//
// Adapted from the imported OpenCode application session file tabs
// (third_party/opencode-v1.18.31/packages/app/src/pages/session/file-tabs.tsx):
// the session-scoped file-browser composition now lives here — a Changes/All tab
// bar over the file tree and the file editor/viewer. The Wayshard file tree and
// compare-and-set save (expectedHash) semantics are preserved; the repository
// remains authoritative for source state.
import { Show, createMemo, createResource, createSignal, type JSX } from "solid-js"
import { Button } from "@wayshard/ui/button"
import { useWayshard } from "../../../wayshard/state"
import { EmptyState } from "../../components/state-views"
import { FileTree } from "../../file-tree"

function Loading(props: { label?: string }): JSX.Element {
  return <div class="wh-loading">{props.label ?? "Loading…"}</div>
}

export function SessionFileTabs(): JSX.Element {
  const ws = useWayshard()
  const [mode, setMode] = createSignal<"changes" | "all">("changes")
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

  const visiblePaths = createMemo(() => {
    const all = tree() ?? []
    if (mode() === "all") return all
    const changedPaths = new Set(ws.state.runChanges)
    return all.filter((p) => changedPaths.has(p))
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
    <div data-slot="session-file-tabs" class="wh-panel flex min-h-0 flex-1 flex-col overflow-y-auto">
      <div class="wh-panel-header">
        <h2>Files</h2>
        <div class="flex items-center gap-2">
          <div class="wh-segmented" data-slot="file-tab-bar">
            <Button size="small" variant={mode() === "changes" ? "primary" : "secondary"} onClick={() => setMode("changes")}>
              Changes
            </Button>
            <Button size="small" variant={mode() === "all" ? "primary" : "secondary"} onClick={() => setMode("all")}>
              All
            </Button>
          </div>
          <Button size="small" variant="ghost" onClick={() => void refetch()}>
            Refresh
          </Button>
        </div>
      </div>
      <Show when={projectID()} fallback={<EmptyState title="No project selected" />}>
        <Show when={!tree.loading} fallback={<Loading />}>
          <div class="wh-files-layout">
            <div class="wh-files-tree">
              <Show
                when={visiblePaths().length}
                fallback={<EmptyState title={mode() === "changes" ? "No changed files" : "No files"} />}
              >
                <FileTree paths={visiblePaths()} selected={selected() ?? undefined} changed={changed()} onSelect={(p) => void open(p)} />
              </Show>
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
