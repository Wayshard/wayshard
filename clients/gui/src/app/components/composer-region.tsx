// Wayshard session composer region.
//
// Adapted from the imported OpenCode session composer region
// (third_party/opencode-v1.18.31/packages/app/src/pages/session/composer/session-composer-region.tsx):
// the dock composition is retained — a bottom dock that hosts the prompt input
// plus request docks above it, with the same `data-component="session-prompt-dock"`
// marker. OpenCode's permission/question/revert/todo docks are replaced by the
// Wayshard approval dock (server-owned approvals); the prompt input remains the
// imported PromptInputV2 composer.
import { For, Show, type JSX } from "solid-js"
import { Button } from "@wayshard/ui/button"
import { useWayshard } from "../../wayshard/state"

export function ComposerRegion(props: { promptInput: JSX.Element }): JSX.Element {
  const ws = useWayshard()
  return (
    <div
      data-component="session-prompt-dock"
      class="w-full shrink-0 flex flex-col justify-center items-center pb-3 bg-v2-background-bg-base"
    >
      <div class="w-full px-3 md:max-w-200 md:mx-auto 2xl:max-w-[1000px]">
        <Show when={ws.state.approvals.length}>
          <div
            data-slot="approval-dock"
            class="mb-2 flex flex-col gap-1 rounded-md border border-v2-border-border-base bg-v2-background-bg-layer-01 p-2"
          >
            <For each={ws.state.approvals}>
              {(approval) => (
                <div class="flex items-center justify-between gap-2">
                  <span class="min-w-0 truncate text-sm text-v2-text-text-base">
                    {approval.kind} {approval.resource}
                  </span>
                  <div class="flex shrink-0 gap-1">
                    <Button size="small" variant="primary" onClick={() => void ws.resolveApproval(approval.id, "allowed")}>
                      Allow
                    </Button>
                    <Button size="small" variant="secondary" onClick={() => void ws.resolveApproval(approval.id, "denied")}>
                      Deny
                    </Button>
                  </div>
                </div>
              )}
            </For>
          </div>
        </Show>
        {props.promptInput}
      </div>
    </div>
  )
}
