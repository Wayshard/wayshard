// Wayshard application titlebar.
//
// Adapted from the imported OpenCode application titlebar family
// (third_party/opencode-v1.18.31/packages/app/src/components/titlebar.tsx and
// components/titlebar-tab-nav.tsx): the composition is retained — a brand/
// project region, a horizontally scrollable tab strip of work surfaces, and a
// trailing actions region — using the same `data-slot` markers
// (`titlebar-tab-strip`, `titlebar-tab-item`). OpenCode's session-tab model and
// desktop window controls are replaced by the Wayshard primary work surfaces and
// the Wayshard command palette.
import { For, Show } from "solid-js"
import { Button } from "@wayshard/ui/button"
import { Icon } from "@wayshard/ui/icon"
import { Tag } from "@wayshard/ui/tag"
import { useWayshard } from "../../wayshard/state"
import { PRIMARY_TABS, activeTab, setActiveTab } from "../navigation"

export function Titlebar(props: { onMore: () => void; onPalette: () => void }): import("solid-js").JSX.Element {
  const ws = useWayshard()
  return (
    <header
      data-component="titlebar"
      class="flex shrink-0 items-center gap-3 border-b border-v2-border-border-base bg-v2-background-bg-base px-3 py-1.5"
    >
      <div class="flex min-w-0 shrink-0 items-center gap-2">
        <span class="truncate text-sm font-medium text-v2-text-text-base">Wayshard</span>
        <span class="truncate text-sm text-v2-text-text-muted">{ws.activeProject()?.name ?? ""}</span>
      </div>
      <nav
        data-slot="titlebar-tab-strip"
        class="flex min-w-0 flex-1 items-center gap-1 overflow-x-auto"
        role="tablist"
      >
        <For each={PRIMARY_TABS}>
          {(tab) => (
            <button
              type="button"
              data-slot="titlebar-tab-item"
              data-active={activeTab() === tab.key}
              class="flex shrink-0 items-center gap-1.5 rounded-md px-2.5 py-1 text-sm text-v2-text-text-base hover:bg-v2-background-bg-layer-01 data-[active=true]:bg-v2-background-bg-layer-02"
              role="tab"
              aria-selected={activeTab() === tab.key}
              onClick={() => setActiveTab(tab.key)}
            >
              <Icon name={tab.icon as never} size="small" />
              <span data-slot="tab-title">{tab.label}</span>
            </button>
          )}
        </For>
        <button
          type="button"
          data-slot="titlebar-tab-item"
          class="flex shrink-0 items-center gap-1.5 rounded-md px-2.5 py-1 text-sm text-v2-text-text-base hover:bg-v2-background-bg-layer-01"
          onClick={props.onMore}
        >
          <Icon name="bullet-list" size="small" />
          <span data-slot="tab-title">More</span>
        </button>
      </nav>
      <div class="flex shrink-0 items-center gap-2">
        <Show when={ws.state.run}>
          <Tag>{ws.state.run!.status}</Tag>
        </Show>
        <Button size="small" variant="ghost" icon="console" onClick={props.onPalette}>
          ⌘K
        </Button>
      </div>
    </header>
  )
}
