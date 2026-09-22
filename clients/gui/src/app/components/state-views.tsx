// Shared Wayshard empty/error state presentation used by the adapted session
// page and the domain view surfaces.
import { Show, type JSX } from "solid-js"

export function EmptyState(props: { title: string; body?: string; action?: JSX.Element }): JSX.Element {
  return (
    <div class="wh-empty-state">
      <div class="wh-empty-title">{props.title}</div>
      <Show when={props.body}>
        <div class="wh-muted">{props.body}</div>
      </Show>
      <Show when={props.action}>{props.action}</Show>
    </div>
  )
}

export function ErrorState(props: { title: string; detail?: string }): JSX.Element {
  return (
    <div class="wh-empty-state" data-state="error">
      <div class="wh-empty-title">{props.title}</div>
      <Show when={props.detail}>
        <pre class="wh-error-detail">{props.detail}</pre>
      </Show>
    </div>
  )
}
