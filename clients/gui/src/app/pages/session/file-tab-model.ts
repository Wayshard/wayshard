// Wayshard file tab model.
//
// Adapted from the imported OpenCode file-tabs tab/scroll model
// (third_party/opencode-v1.18.31/packages/app/src/pages/session/file-tabs.tsx):
// the behavior-bearing pieces are the Changes/All mode, an ordered set of open
// file tabs with an active file, close/switch transitions that pick a sensible
// neighbor, and a per-file scroll position. Kept as pure reducers so the
// transition behavior is unit-testable without a DOM.
export type FileMode = "changes" | "all"

export interface ScrollPos {
  x: number
  y: number
}

export interface FileTabState {
  mode: FileMode
  tabs: string[]
  active: string | undefined
  scroll: Record<string, ScrollPos>
}

export function createFileTabState(mode: FileMode = "changes"): FileTabState {
  return { mode, tabs: [], active: undefined, scroll: {} }
}

export function setMode(state: FileTabState, mode: FileMode): FileTabState {
  return { ...state, mode }
}

// openFile adds the path as a tab if absent and makes it active; opening an
// already-open file switches to it without reordering.
export function openFile(state: FileTabState, path: string): FileTabState {
  const tabs = state.tabs.includes(path) ? state.tabs : [...state.tabs, path]
  return { ...state, tabs, active: path }
}

export function activateFile(state: FileTabState, path: string): FileTabState {
  if (!state.tabs.includes(path)) return state
  return { ...state, active: path }
}

// closeFile removes the tab. When the closed tab was active, the neighbor to
// the left becomes active, else the one to the right, matching the inherited
// close/switch behavior.
export function closeFile(state: FileTabState, path: string): FileTabState {
  const index = state.tabs.indexOf(path)
  if (index === -1) return state
  const tabs = state.tabs.filter((tab) => tab !== path)
  let active = state.active
  if (active === path) {
    const neighbor = tabs[Math.max(0, index - 1)] ?? tabs[0]
    active = neighbor
  }
  return { ...state, tabs, active }
}

export function setScroll(state: FileTabState, path: string, pos: ScrollPos): FileTabState {
  return { ...state, scroll: { ...state.scroll, [path]: pos } }
}
