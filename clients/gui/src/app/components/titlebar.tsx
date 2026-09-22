// Wayshard application titlebar.
//
// Adapted from the imported OpenCode application titlebar
// (third_party/opencode-v1.18.31/packages/app/src/components/titlebar.tsx and
// components/titlebar-tab-nav.tsx): the composition is retained — a leading
// region with the narrow-width (`xl:hidden`) sidebar menu toggle, a scrollable
// work-surface tab strip with `data-slot` markers, and trailing actions. OpenCode
// window controls and session-tab model are replaced by the Wayshard primary
// surfaces and command palette.
import { For, Show, type JSX } from "solid-js"
import { Button } from "@wayshard/ui/button"
import { Icon } from "@wayshard/ui/icon"
import { IconButton } from "@wayshard/ui/icon-button"
import { Tag } from "@wayshard/ui/tag"
import { Dialog } from "@wayshard/ui/dialog"
import { useDialog } from "@wayshard/ui/context/dialog"
import { useWayshard } from "../../wayshard/state"
import { useLayout } from "../context/layout"
import { CommandPalette } from "../command-palette"
import { AdvancedSurface, type AdvancedSurfaceKey } from "../views"
import { ADVANCED_SURFACES, PRIMARY_TABS, activeTab, setActiveTab } from "../navigation"

export function Titlebar(): JSX.Element {
  const ws = useWayshard()
  const layout = useLayout()
  const dialog = useDialog()

  function openPalette() {
    dialog.show(() => <CommandPalette />)
  }

  function openAdvanced(key: AdvancedSurfaceKey) {
    dialog.show(() => (
      <Dialog title={ADVANCED_SURFACES.find((s) => s.key === key)?.label ?? key} size="x-large">
        <div class="wh-dialog-surface">
          <AdvancedSurface view={key} />
        </div>
      </Dialog>
    ))
  }

  function openMore() {
    dialog.show(() => (
      <Dialog title="More surfaces" size="large">
        <div class="wh-more-grid">
          <For each={ADVANCED_SURFACES}>
            {(s) => (
              <button class="wh-more-item" onClick={() => openAdvanced(s.key)}>
                {s.label}
              </button>
            )}
          </For>
        </div>
      </Dialog>
    ))
  }

  return (
    <header
      data-component="titlebar"
      class="grid h-10 shrink-0 grid-cols-[auto_minmax(0,1fr)_auto] items-center gap-2 border-b border-v2-border-border-base bg-v2-background-bg-base px-2"
    >
      <div class="flex min-w-0 items-center gap-1">
        <div class="flex w-[48px] shrink-0 items-center justify-center xl:hidden">
          <IconButton
            icon="bullet-list"
            variant="ghost"
            class="rounded-md"
            data-component="mobile-nav-toggle"
            onClick={layout.mobileSidebar.toggle}
            aria-label="Toggle navigation"
            aria-expanded={layout.mobileSidebar.opened()}
          />
        </div>
        <span class="hidden truncate text-sm font-medium text-v2-text-text-base sm:inline">Wayshard</span>
        <span class="truncate text-sm text-v2-text-text-muted">{ws.activeProject()?.name ?? ""}</span>
      </div>
      <nav data-slot="titlebar-tab-strip" class="flex min-w-0 items-center gap-1 overflow-x-auto" role="tablist">
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
          onClick={openMore}
        >
          <Icon name="bullet-list" size="small" />
          <span data-slot="tab-title">More</span>
        </button>
      </nav>
      <div class="flex shrink-0 items-center gap-2">
        <Show when={ws.state.run}>
          <Tag>{ws.state.run!.status}</Tag>
        </Show>
        <Button size="small" variant="ghost" icon="console" onClick={openPalette}>
          ⌘K
        </Button>
      </div>
    </header>
  )
}
