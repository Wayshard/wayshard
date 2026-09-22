// Wayshard review view state.
//
// Adapted from the imported OpenCode review-tab scroll persistence
// (third_party/opencode-v1.18.31/packages/app/src/pages/session/review-tab.tsx,
// backed by the layout context view): the review scroll position and the open
// file set are remembered per session so switching sessions or reopening the
// Changes surface restores where the user was instead of resetting to the top.
import { createSignal } from "solid-js"

export interface ReviewScroll {
  x: number
  y: number
}

interface ReviewViewState {
  scroll: ReviewScroll
  open: string[]
}

const views = new Map<string, ReviewViewState>()

function read(key: string): ReviewViewState {
  return views.get(key) ?? { scroll: { x: 0, y: 0 }, open: [] }
}

export function createReviewView(key: () => string) {
  const initial = read(key())
  const [scroll, setScrollSignal] = createSignal<ReviewScroll>(initial.scroll)
  const [open, setOpenSignal] = createSignal<string[]>(initial.open)
  let current = key()

  function sync(): void {
    const next = key()
    if (next === current) return
    current = next
    const state = read(next)
    setScrollSignal(state.scroll)
    setOpenSignal(state.open)
  }

  function setScroll(value: ReviewScroll): void {
    setScrollSignal(value)
    views.set(current, { ...read(current), scroll: value })
  }

  function setOpen(value: string[]): void {
    setOpenSignal(value)
    views.set(current, { ...read(current), open: value })
  }

  return { scroll, setScroll, open, setOpen, sync }
}
