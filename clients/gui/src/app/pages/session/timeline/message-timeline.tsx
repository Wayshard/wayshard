// Wayshard session message timeline.
//
// Adapted from the imported OpenCode application message timeline
// (third_party/opencode-v1.18.31/packages/app/src/pages/session/timeline/message-timeline.tsx):
// the timeline composition is retained — a message region rendered through the
// adapted session-ui turn component, followed by the run stage timeline. OpenCode
// message/part projections are replaced by the Wayshard message/run/stage model.
import { For, Show, createMemo, createSignal, type JSX } from "solid-js"
import { Icon } from "@wayshard/ui/icon"
import { Tag } from "@wayshard/ui/tag"
import { SessionTurn } from "@wayshard/gui/session-ui/components/session-turn"
import { useWayshard } from "../../../../wayshard/state"
import { EmptyState } from "../../../components/state-views"
import { runLabel, stageLabel } from "./model"

export function SessionTimeline(): JSX.Element {
  const ws = useWayshard()
  const userMessages = createMemo(() => ws.state.messages.filter((m) => m.role === "user"))
  const [expanded, setExpanded] = createSignal<string | null>(null)

  return (
    <div data-slot="message-timeline" class="wh-session-stream">
      <Show when={userMessages().length} fallback={<EmptyState title="No messages yet" body="Describe a task to start a run." />}>
        <For each={userMessages()}>{(m) => <SessionTurn sessionID={ws.state.activeConversationID!} messageID={m.id} />}</For>
      </Show>
      <Show when={ws.state.run}>
        <section class="wh-timeline" aria-label="Run timeline" data-slot="run-timeline">
          <div class="wh-timeline-header">
            <span>{runLabel(ws.state.run)}</span>
            <Tag>{ws.state.run?.status}</Tag>
          </div>
          <For each={ws.state.stages}>
            {(stage) => (
              <div class="wh-stage" data-status={stage.status}>
                <button class="wh-stage-row" onClick={() => setExpanded(expanded() === stage.id ? null : stage.id)}>
                  <Icon name="chevron-right" size="small" />
                  <span class="wh-stage-kind">{stageLabel(stage.kind)}</span>
                  <span class="wh-muted">{stage.status}</span>
                </button>
                <Show when={expanded() === stage.id}>
                  <div class="wh-stage-detail">
                    <div>Stage ID: {stage.id}</div>
                    <div>Ordinal: {stage.ordinal}</div>
                  </div>
                </Show>
              </div>
            )}
          </For>
        </section>
      </Show>
    </div>
  )
}
