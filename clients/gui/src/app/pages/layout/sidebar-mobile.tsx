// Wayshard mobile/narrow sidebar.
//
// Adapted from the imported OpenCode application narrow-width navigation
// (third_party/opencode-v1.18.31/packages/app/src/pages/layout.tsx, which mounts
// the sidebar as a mobile drawer): the drawer composition is retained — an
// overlay backdrop plus an off-canvas panel hosting the sidebar content, closed
// by backdrop click or after navigation. The Wayshard sidebar content is shared
// with the desktop rail/panel.
import { Show, type JSX } from "solid-js"

export function SidebarMobile(props: { open: boolean; onClose: () => void; children: JSX.Element }): JSX.Element {
  return (
    <Show when={props.open}>
      <div
        data-component="sidebar-mobile-backdrop"
        class="fixed inset-0 z-30 bg-black/50"
        aria-hidden="true"
        onClick={props.onClose}
      />
      <div
        data-component="sidebar-mobile"
        class="fixed inset-y-0 left-0 z-40 w-[min(82vw,300px)] shadow-[0_0_24px_rgba(0,0,0,0.6)]"
      >
        {props.children}
      </div>
    </Show>
  )
}
