// Wayshard timeline scroll helpers.
//
// Adapted from the imported OpenCode timeline scroll/virtualization behavior
// (third_party/opencode-v1.18.31/packages/app/src/pages/session/timeline/message-timeline.tsx,
// which anchors to end and follows appended rows, and preserves the scroll
// position when older rows are prepended). These are the pure, testable
// decisions; the component owns the DOM effects.
export interface ScrollMetrics {
  scrollTop: number
  clientHeight: number
  scrollHeight: number
}

export const BOTTOM_THRESHOLD = 48

// isAtBottom reports whether the viewport is close enough to the end that new
// rows should be followed (auto-scrolled into view).
export function isAtBottom(metrics: ScrollMetrics, threshold = BOTTOM_THRESHOLD): boolean {
  const distance = metrics.scrollHeight - metrics.scrollTop - metrics.clientHeight
  return distance <= threshold
}

// maxScrollTop clamps a restored position to the current content height.
export function clampScrollTop(scrollTop: number, metrics: ScrollMetrics): number {
  return Math.max(0, Math.min(scrollTop, Math.max(0, metrics.scrollHeight - metrics.clientHeight)))
}

// windowRows bounds the number of mounted rows for long sessions. The newest
// rows are always mounted; older rows are revealed explicitly. This is a
// bounded-DOM strategy rather than a fake virtualizer: no row is measured or
// repositioned, and every row is reachable.
export function windowRows<T>(rows: T[], limit: number): T[] {
  if (limit <= 0 || rows.length <= limit) return rows
  return rows.slice(rows.length - limit)
}

export function nextWindow(current: number, total: number, step: number): number {
  return Math.min(total, Math.max(current, 0) + step)
}
