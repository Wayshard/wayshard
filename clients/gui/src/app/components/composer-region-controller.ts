// Wayshard session composer region controller.
//
// Adapted from the imported OpenCode session composer region controller
// (third_party/opencode-v1.18.31/packages/app/src/pages/session/composer/session-composer-region-controller.ts):
// the behavior-bearing pieces are retained — dock/prompt/body refs, dock-height
// measurement through a resize observer, an animated dock progress feed,
// centered/full-width responsive logic and focus restoration to the prompt
// after a dock decision. OpenCode's session handoff
// and revert/todo/child-session concepts are omitted because Wayshard has no
// matching server semantics; the approval dock replaces the permission dock.
import { createMediaQuery } from "@solid-primitives/media"
import { createResizeObserver } from "@solid-primitives/resize-observer"
import { useSpring } from "@wayshard/ui/motion-spring"
import { createEffect } from "solid-js"
import { createStore } from "solid-js/store"
import type { SessionComposerController } from "./session-composer-state"

export interface SessionComposerRegionInput {
  state: SessionComposerController
  sessionKey: () => string
}

export function createSessionComposerRegionController(input: SessionComposerRegionInput) {
  const centered = createMediaQuery("(min-width: 768px)")
  const isDesktop = centered
  let dockRef: HTMLDivElement | undefined
  let promptRef: HTMLDivElement | undefined

  const [store, setStore] = createStore({
    height: 320,
    body: undefined as HTMLDivElement | undefined,
  })

  // Measure the dock body so the max-height transition matches its content.
  createEffect(() => {
    const el = store.body
    if (!el) return
    const update = () => setStore("height", el.getBoundingClientRect().height)
    createResizeObserver(el, update)
    update()
  })

  const open = () => input.state.dock() && !input.state.closing()
  const progress = useSpring(
    () => (open() ? 1 : 0),
    { visualDuration: 0.3, bounce: 0 },
    () => `${input.sessionKey()}\0${open()}`,
  )
  const value = () => Math.max(0, Math.min(1, progress()))

  function focusPrompt(): void {
    const target = promptRef?.querySelector<HTMLElement>(
      '[contenteditable="true"], textarea, input, [tabindex]:not([tabindex="-1"])',
    )
    target?.focus()
  }

  return {
    state: input.state,
    centered: () => centered(),
    isDesktop: () => isDesktop(),
    dock: () => open(),
    dockProgress: value,
    dockHeight: () => Math.max(78, store.height),
    showComposer: () => true,
    promptReady: () => true,
    focusPrompt,
    setDockRef: (el: HTMLDivElement) => {
      dockRef = el
    },
    setPromptRef: (el: HTMLDivElement) => {
      promptRef = el
    },
    setDockBodyRef: (el: HTMLDivElement) => setStore("body", el),
  }
}

export type SessionComposerRegionController = ReturnType<typeof createSessionComposerRegionController>
