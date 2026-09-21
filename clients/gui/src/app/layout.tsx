// Wayshard graphical application layout.
//
// Adapted from the imported OpenCode 2 graphical client layout page
// (third_party/opencode-v1.18.31/packages/app/src/pages/layout-new.tsx): the
// titlebar + main + toast composition is retained. The titlebar, content and
// toasts are Wayshard.
import type { JSX, ParentProps } from "solid-js"
import { Toast } from "@wayshard/ui/toast"

export function AppLayout(props: ParentProps<{ titlebar?: JSX.Element }>) {
  return (
    <div class="wh-layout">
      <div class="wh-layout-inner">
        {props.titlebar}
        <main class="wh-layout-main">{props.children}</main>
      </div>
      <Toast.Region />
    </div>
  )
}
