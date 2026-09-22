// Wayshard session message timeline.
//
// Adapted from the imported OpenCode application message timeline
// (third_party/opencode-v1.18.31/packages/app/src/pages/session/timeline/message-timeline.tsx):
// the behavior-bearing interaction model is retained — a reconciled row
// projection with stable keys, bottom-follow when the user is at the end,
// scroll-position preservation when they have scrolled away, a jump-to-latest
// affordance, per-session scroll/expansion state and a bounded mounted window
// for long sessions. Wayshard run stages are rows in the same projection rather
// than a custom block appended beneath the messages. OpenCode message/part
// projections are replaced by the Wayshard message/run/stage model.
//
// Virtualization note: true windowed virtualization from @tanstack/solid-virtual
// is not available as a Wayshard dependency and the imported SessionTurn
// boundary does not virtualize, so this uses an explicit bounded mounted window
// (newest rows, with "show earlier") instead of faking a virtualizer. Every row
// remains reachable and no row is measured or repositioned.
import { For, Match, Show, Switch, createEffect, createMemo, on, type JSX } from "solid-js"
import { Button } from "@wayshard/ui/button"
import { Icon } from "@wayshard/ui/icon"
import { Tag } from "@wayshard/ui/tag"
import { SessionTurn } from "@wayshard/gui/session-ui/components/session-turn"
import { useWayshard } from "../../../../wayshard/state"
import { EmptyState } from "../../../components/state-views"
import {
  activeMessageKey,
  buildTimelineRows,
  reconcileRows,
  runLabel,
  stageLabel,
  type TimelineRow,
} from "./model"
import { clampScrollTop, isAtBottom, nextWindow, windowRows, type ScrollMetrics } from "./scroll"
import { createTimelineView } from "./view"

const WINDOW_STEP = 60

export function SessionTimeline(): JSX.Element {
  const ws = useWayshard()
  const view = createTimelineView(() => ws.state.activeConversationID ?? "global")
  let scroller: HTMLDivElement | undefined

  createEffect(() => view.sync())

  const rows = createMemo<TimelineRow[]>(
    (previous) =>
      reconcileRows(
        previous,
        buildTimelineRows({ messages: ws.state.messages, run: ws.state.run, stages: ws.state.stages }),
      ),
    [],
  )
  const activeKey = createMemo(() => activeMessageKey(rows()))
  const visible = createMemo(() => windowRows(rows(), view.window()))

  function metrics(): ScrollMetrics | undefined {
    const el = scroller
    if (!el) return undefined
    return { scrollTop: el.scrollTop, clientHeight: el.clientHeight, scrollHeight: el.scrollHeight }
  }

  function scrollToBottom(): void {
    const el = scroller
    if (!el) return
    el.scrollTop = el.scrollHeight
  }

  // reveal/scroll-to-message: bring a row with a known key into view.
  function scrollToKey(key: string): void {
    const el = scroller
    if (!el) return
    const target = el.querySelector<HTMLElement>(`[data-timeline-key="${CSS.escape(key)}"]`)
    target?.scrollIntoView({ block: "start" })
  }

  function handleScroll(): void {
    const m = metrics()
    if (!m) return
    view.setAtBottom(isAtBottom(m))
    view.setScroll({ x: 0, y: m.scrollTop })
  }

  // Bottom-follow: when rows are appended and the user was at the end, keep the
  // newest row in view. When they have scrolled away, do nothing so their
  // position is preserved.
  createEffect(
    on(
      () => rows().length,
      () => {
        if (view.atBottom()) requestAnimationFrame(scrollToBottom)
      },
    ),
  )

  // Session switch: restore the remembered position, or follow the end.
  createEffect(
    on(
      () => ws.state.activeConversationID,
      () => {
        requestAnimationFrame(() => {
          const m = metrics()
          if (!m) return
          if (view.atBottom()) {
            scrollToBottom()
            return
          }
          const el = scroller
          if (el) el.scrollTop = clampScrollTop(view.scroll().y, m)
        })
      },
    ),
  )

  return (
    <div
      ref={scroller}
      data-slot="message-timeline"
      class="wh-session-stream"
      onScroll={handleScroll}
    >
      <Show when={rows().length > visible().length}>
        <div class="wh-timeline-window">
          <Button size="small" variant="ghost" onClick={() => view.growWindow(WINDOW_STEP)}>
            Show earlier messages ({rows().length - visible().length})
          </Button>
        </div>
      </Show>
      <Show
        when={visible().length}
        fallback={<EmptyState title="No messages yet" body="Describe a task to start a run." />}
      >
        <For each={visible()}>
          {(row) => (
            <TimelineRowView
              row={row}
              sessionID={ws.state.activeConversationID ?? undefined}
              activeKey={activeKey()}
              expanded={view.expanded()}
              onToggle={view.toggleExpanded}
            />
          )}
        </For>
      </Show>
      <Show when={rows().length && !view.atBottom()}>
        <button type="button" class="wh-timeline-jump" onClick={scrollToBottom}>
          <Icon name="arrow-down-to-line" size="small" /> Jump to latest
        </button>
      </Show>
    </div>
  )
}

function TimelineRowView(props: {
  row: TimelineRow
  sessionID?: string
  activeKey?: string
  expanded: string[]
  onToggle: (id: string) => void
}): JSX.Element {
  return (
    <div
      data-timeline-key={props.row.key}
      data-timeline-row={props.row.tag}
      data-active={props.row.key === props.activeKey ? "" : undefined}
    >
      <Switch>
        <Match when={props.row.tag === "message"}>
          <Show when={props.sessionID}>
            <SessionTurn sessionID={props.sessionID!} messageID={(props.row as { id: string }).id} />
          </Show>
        </Match>
        <Match when={props.row.tag === "run"}>
          <section class="wh-timeline" aria-label="Run timeline" data-slot="run-timeline">
            <div class="wh-timeline-header">
              <span>{runLabel((props.row as { run: { id: string; status: string } }).run)}</span>
              <Tag>{(props.row as { run: { status: string } }).run.status}</Tag>
            </div>
          </section>
        </Match>
        <Match when={props.row.tag === "stage"}>
          {(() => {
            const stage = (props.row as { stage: { id: string; kind: string; status: string; ordinal: number } }).stage
            const open = () => props.expanded.includes(stage.id)
            return (
              <div class="wh-stage" data-status={stage.status}>
                <button class="wh-stage-row" onClick={() => props.onToggle(stage.id)}>
                  <Icon name="chevron-right" size="small" />
                  <span class="wh-stage-kind">{stageLabel(stage.kind)}</span>
                  <span class="wh-muted">{stage.status}</span>
                </button>
                <Show when={open()}>
                  <div class="wh-stage-detail">
                    <div>Stage ID: {stage.id}</div>
                    <div>Ordinal: {stage.ordinal}</div>
                  </div>
                </Show>
              </div>
            )
          })()}
        </Match>
      </Switch>
    </div>
  )
}
