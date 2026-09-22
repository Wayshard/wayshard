// Wayshard timeline view state.
//
// Adapted from the imported OpenCode timeline per-session cache
// (third_party/opencode-v1.18.31/packages/app/src/pages/session/timeline/message-timeline.tsx,
// which caches virtualizer measurements and expansion per session key): the
// timeline remembers its scroll position, bottom-follow intent, expanded stage
// rows and window size per session so switching sessions restores the previous
// view instead of resetting.
import { createSignal } from "solid-js"

export interface TimelineViewState {
  scroll: { x: number; y: number }
  atBottom: boolean
  expanded: string[]
  window: number
}

export const DEFAULT_WINDOW = 60

const views = new Map<string, TimelineViewState>()

function read(key: string): TimelineViewState {
  return views.get(key) ?? { scroll: { x: 0, y: 0 }, atBottom: true, expanded: [], window: DEFAULT_WINDOW }
}

export function createTimelineView(key: () => string) {
  const initial = read(key())
  const [scroll, setScrollSignal] = createSignal(initial.scroll)
  const [atBottom, setAtBottomSignal] = createSignal(initial.atBottom)
  const [expanded, setExpandedSignal] = createSignal<string[]>(initial.expanded)
  const [windowSize, setWindowSignal] = createSignal(initial.window)
  let current = key()

  function persist(): void {
    views.set(current, {
      scroll: scroll(),
      atBottom: atBottom(),
      expanded: expanded(),
      window: windowSize(),
    })
  }

  function sync(): void {
    const next = key()
    if (next === current) return
    current = next
    const state = read(next)
    setScrollSignal(state.scroll)
    setAtBottomSignal(state.atBottom)
    setExpandedSignal(state.expanded)
    setWindowSignal(state.window)
  }

  function setScroll(value: { x: number; y: number }): void {
    setScrollSignal(value)
    persist()
  }

  function setAtBottom(value: boolean): void {
    setAtBottomSignal(value)
    persist()
  }

  function toggleExpanded(id: string): void {
    setExpandedSignal((prev) => (prev.includes(id) ? prev.filter((x) => x !== id) : [...prev, id]))
    persist()
  }

  function growWindow(step: number): void {
    setWindowSignal((prev) => prev + step)
    persist()
  }

  return { scroll, atBottom, expanded, window: windowSize, sync, setScroll, setAtBottom, toggleExpanded, growWindow }
}
