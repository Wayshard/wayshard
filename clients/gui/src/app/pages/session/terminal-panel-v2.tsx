// Wayshard session terminal panel.
//
// Adapted from the imported OpenCode application terminal panel
// (third_party/opencode-v1.18.31/packages/app/src/pages/session/terminal-panel-v2.tsx):
// the behavior-bearing panel lifecycle is retained — a terminal tab strip with
// active selection, create/close/open actions, focus on selection, one
// `terminal-wrapper-<id>` per server PTY, and honest loss/recovery presentation.
// The backend remains the Wayshard server-owned PTY rendered through the
// imported ghostty-web presentation; the client never spawns a shell.
//
// Deliberate semantic divergence: OpenCode's terminal panel is a docked split
// with a vertical resize handle over an in-memory terminal context. Wayshard's
// Terminal is a primary work surface (a tab of the session content region) and
// its PTY stream is not server-replayed, so the split-resize handle and the
// client-side screen serialization/handoff are omitted rather than faked.
// Inactive terminals stay mounted (hidden) so their scrollback is preserved for
// the session; the server PTY list is the source of truth.
import { For, Show, batch, createEffect, on, type JSX } from "solid-js"
import { createStore } from "solid-js/store"
import { Button } from "@wayshard/ui/button"
import { Icon } from "@wayshard/ui/icon"
import { useWayshard } from "../../../wayshard/state"
import { EmptyState } from "../../components/state-views"
import { TerminalView } from "../../terminal"
import { focusTerminalById, terminalTabLabel } from "./terminal-helpers"

interface TerminalTab {
  id: string
  ordinal: number
  title: string
}

function Loading(props: { label?: string }): JSX.Element {
  return <div class="wh-loading">{props.label ?? "Loading…"}</div>
}

export function SessionTerminalPanel(): JSX.Element {
  const ws = useWayshard()
  const [store, setStore] = createStore({
    tabs: [] as TerminalTab[],
    activeId: undefined as string | undefined,
    loading: false,
    error: "",
    autoCreated: false,
  })
  const projectID = () => ws.state.activeProjectID

  function nextOrdinal(): number {
    return store.tabs.reduce((max, tab) => Math.max(max, tab.ordinal), 0) + 1
  }

  async function createTerminal(): Promise<void> {
    const id = projectID()
    if (!id) return
    setStore("loading", true)
    setStore("error", "")
    try {
      const created = await ws.client().startTerminal(id)
      batch(() => {
        setStore("tabs", (tabs) => [...tabs, { id: created.id, ordinal: nextOrdinal(), title: "" }])
        setStore("activeId", created.id)
      })
    } catch (err) {
      setStore("error", String(err))
    } finally {
      setStore("loading", false)
    }
  }

  async function closeTerminal(tabId: string): Promise<void> {
    const id = projectID()
    if (!id) return
    try {
      await ws.client().closeTerminal(id, tabId)
    } catch {
      // The PTY may already have exited; removing the tab is still correct.
    }
    const index = store.tabs.findIndex((tab) => tab.id === tabId)
    const tabs = store.tabs.filter((tab) => tab.id !== tabId)
    batch(() => {
      setStore("tabs", tabs)
      if (store.activeId === tabId) {
        const neighbor = tabs[Math.max(0, Math.min(index - 1, tabs.length - 1))]
        setStore("activeId", neighbor?.id)
      }
    })
  }

  function selectTerminal(tabId: string): void {
    setStore("activeId", tabId)
    requestAnimationFrame(() => focusTerminalById(tabId))
  }

  function setTabTitle(tabId: string, title: string): void {
    setStore("tabs", (tab) => tab.id === tabId, "title", title)
  }

  // Load the project's server-owned PTYs when the panel opens; auto-create one
  // when the panel has none, mirroring upstream's open-triggered creation.
  createEffect(
    on(
      () => projectID(),
      async (id) => {
        if (!id) {
          batch(() => {
            setStore("tabs", [])
            setStore("activeId", undefined)
          })
          return
        }
        setStore("error", "")
        try {
          const list = await ws.client().terminals(id)
          batch(() => {
            setStore(
              "tabs",
              list.map((t, index) => ({ id: t.id, ordinal: index + 1, title: "" })),
            )
            setStore("activeId", list[0]?.id)
            setStore("autoCreated", false)
          })
          if (list.length === 0 && !store.autoCreated) {
            setStore("autoCreated", true)
            await createTerminal()
          }
        } catch (err) {
          setStore("error", String(err))
        }
      },
    ),
  )

  // Keep focus on the active terminal when the selection changes.
  createEffect(
    on(
      () => store.activeId,
      (id) => {
        if (id) requestAnimationFrame(() => focusTerminalById(id))
      },
    ),
  )

  return (
    <div data-slot="session-terminal-panel" class="flex min-h-0 flex-1 flex-col overflow-hidden">
      <Show
        when={projectID()}
        fallback={<EmptyState title="No project selected" body="Open a project to use a terminal." />}
      >
        <div
          data-slot="terminal-tab-strip"
          class="flex h-10 shrink-0 items-center gap-1 overflow-x-auto border-b border-v2-border-border-base px-2"
        >
          <For each={store.tabs}>
            {(tab) => (
              <div
                data-slot="terminal-tab"
                data-active={store.activeId === tab.id}
                class="group flex shrink-0 items-center rounded-md"
                classList={{ "bg-v2-background-bg-layer-01": store.activeId === tab.id }}
              >
                <button
                  type="button"
                  class="max-w-40 truncate px-2 py-1 text-sm"
                  title={terminalTabLabel({ title: tab.title, number: tab.ordinal })}
                  onClick={() => selectTerminal(tab.id)}
                >
                  {terminalTabLabel({ title: tab.title, number: tab.ordinal })}
                </button>
                <button
                  type="button"
                  aria-label="Close terminal"
                  data-slot="terminal-tab-close"
                  class="px-1 text-v2-text-text-weak opacity-0 group-hover:opacity-100"
                  onClick={() => void closeTerminal(tab.id)}
                >
                  <Icon name="close-small" size="small" />
                </button>
              </div>
            )}
          </For>
          <Button
            size="small"
            variant="ghost"
            icon="plus-small"
            aria-label="New terminal"
            disabled={store.loading}
            onClick={() => void createTerminal()}
          />
        </div>
        <Show when={store.error}>
          <div class="wh-muted px-2 py-1 text-xs">{store.error}</div>
        </Show>
        <Show when={!store.loading || store.tabs.length} fallback={<Loading label="Starting terminal…" />}>
          <Show
            when={store.tabs.length}
            fallback={
              <div class="flex flex-1 items-center justify-center">
                <Button size="small" variant="primary" icon="terminal" onClick={() => void createTerminal()}>
                  New terminal
                </Button>
              </div>
            }
          >
            <div class="relative min-h-0 flex-1">
              <For each={store.tabs}>
                {(tab) => (
                  <div
                    id={`terminal-wrapper-${tab.id}`}
                    class="absolute inset-0"
                    hidden={store.activeId !== tab.id}
                  >
                    <TerminalView
                      ptyId={tab.id}
                      active={store.activeId === tab.id}
                      onTitleChange={(title) => setTabTitle(tab.id, title)}
                    />
                  </div>
                )}
              </For>
            </div>
          </Show>
        </Show>
      </Show>
    </div>
  )
}
