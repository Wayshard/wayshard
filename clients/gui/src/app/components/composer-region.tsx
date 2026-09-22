// Wayshard session composer region.
//
// Adapted from the imported OpenCode session composer region
// (third_party/opencode-v1.18.31/packages/app/src/pages/session/composer/session-composer-region.tsx):
// the dock composition is retained — a bottom `data-component="session-prompt-dock"`
// that hosts the prompt input plus the active request dock above it, with
// centered/full-width responsive logic, a measured max-height reveal for the
// dock, and focus restoration to the prompt after a decision. OpenCode's
// permission/question/revert/todo docks are replaced by the Wayshard approval
// dock (server-owned approvals); the prompt input remains the imported
// PromptInputV2 composer.
import { For, Show, createMemo, type JSX } from "solid-js"
import { Button } from "@wayshard/ui/button"
import { Tag } from "@wayshard/ui/tag"
import { useWayshard } from "../../wayshard/state"
import { createSessionComposerController } from "./session-composer-state"
import { createSessionComposerRegionController } from "./composer-region-controller"

export function ComposerRegion(props: { promptInput: JSX.Element }): JSX.Element {
  const ws = useWayshard()
  const state = createSessionComposerController()
  const controller = createSessionComposerRegionController({
    state,
    sessionKey: () => ws.state.activeConversationID ?? "global",
  })

  const approvals = createMemo(() => state.approvals())

  async function decide(id: string, status: "allowed" | "denied") {
    await state.decide(id, status)
    // Once the request dock empties, return focus to the prompt so the user can
    // keep typing without reaching for the mouse.
    if (!state.approvalRequest()) controller.focusPrompt()
  }

  return (
    <div
      ref={controller.setDockRef}
      data-component="session-prompt-dock"
      class="w-full shrink-0 flex flex-col justify-center items-center pb-3 bg-v2-background-bg-base pointer-events-none"
    >
      <div
        class="w-full px-3 pointer-events-auto"
        classList={{ "md:max-w-200 md:mx-auto 2xl:max-w-[1000px]": controller.centered() }}
      >
        <Show when={state.approvalRequest()} keyed>
          <div
            class="overflow-hidden"
            data-slot="approval-dock-reveal"
            classList={{ "pointer-events-none": controller.dockProgress() < 0.98 }}
            style={{ "max-height": `${controller.dockHeight() * controller.dockProgress()}px` }}
          >
            <div
              ref={controller.setDockBodyRef}
              data-slot="approval-dock"
              class="mb-2 flex flex-col gap-1 rounded-md border border-v2-border-border-base bg-v2-background-bg-layer-01 p-2"
            >
              <For each={approvals()}>
                {(approval) => (
                  <div class="flex items-center justify-between gap-2" data-approval-id={approval.id}>
                    <div class="min-w-0 flex flex-col">
                      <span class="truncate text-sm text-v2-text-text-base">
                        <Tag>{approval.kind}</Tag> {approval.resource}
                      </span>
                      <span class="wh-muted truncate text-xs">{approval.reason}</span>
                    </div>
                    <div class="flex shrink-0 gap-1">
                      <Button
                        size="small"
                        variant="primary"
                        disabled={state.responding()}
                        onClick={() => void decide(approval.id, "allowed")}
                      >
                        {state.responding() && state.approvalRequest()?.id === approval.id ? "Allowing…" : "Allow"}
                      </Button>
                      <Button
                        size="small"
                        variant="secondary"
                        disabled={state.responding()}
                        onClick={() => void decide(approval.id, "denied")}
                      >
                        Deny
                      </Button>
                    </div>
                  </div>
                )}
              </For>
            </div>
          </div>
        </Show>
        <div ref={controller.setPromptRef} class="relative z-[70]">
          {props.promptInput}
        </div>
      </div>
    </div>
  )
}
