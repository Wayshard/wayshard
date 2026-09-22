// Wayshard session composer state.
//
// Adapted from the imported OpenCode session composer controller
// (third_party/opencode-v1.18.31/packages/app/src/pages/session/composer/session-composer-state.ts):
// the behavior-bearing structure is retained — a per-session request dock with
// an open/closing/opening transition state machine, a "responding" lock so a
// decision is applied exactly once, and a blocked/request view derived from
// server-owned domain state. OpenCode's permission/question/todo request trees
// are replaced by the Wayshard approval list (server-owned, independently
// revocable) and the server-owned run lifecycle.
import { createEffect, createMemo, createSignal, on, onCleanup } from "solid-js"
import { createStore } from "solid-js/store"
import { useWayshard } from "../../wayshard/state"

export interface ComposerPromptState {
  profile: () => string
  setProfile: (value: string) => void
  artifactOnly: () => boolean
  setArtifactOnly: (value: boolean) => void
}

// Routing profile and artifact-only are legitimate Wayshard composer controls.
// They live here so the composer region/controls share one prompt state shape
// instead of OpenCode's provider/model selection.
export function createComposerPromptState(): ComposerPromptState {
  const [profile, setProfile] = createSignal("auto")
  const [artifactOnly, setArtifactOnly] = createSignal(false)
  return { profile, setProfile, artifactOnly, setArtifactOnly }
}

export type ComposerDockState = "hide" | "open" | "close"

// Pure dock transition decision (adapted from OpenCode's `todoState`), kept
// separate from the component so the open/close boundary is unit-testable.
export function composerDockState(input: {
  count: number
  sessionChanged: boolean
  wasOpen: boolean
}): ComposerDockState {
  if (input.sessionChanged) return input.count > 0 ? "open" : "hide"
  if (input.count > 0) return "open"
  return input.wasOpen ? "close" : "hide"
}

export function createSessionComposerController(options?: { closeMs?: number }) {
  const ws = useWayshard()
  const closeMs = () => Math.max(0, options?.closeMs ?? 400)

  // Approvals relevant to the active session: the active run's approvals when a
  // run is selected, otherwise every pending approval (server-owned, so a
  // decision may be made from any client).
  const approvals = createMemo(() => {
    const runID = ws.state.activeRunID
    return runID ? ws.state.approvals.filter((a) => a.runId === runID) : ws.state.approvals
  })
  const approvalRequest = createMemo(() => approvals()[0])

  const [store, setStore] = createStore({
    sessionID: ws.state.activeConversationID,
    responding: undefined as string | undefined,
    dock: approvals().length > 0,
    closing: false,
    opening: false,
  })

  let timer: ReturnType<typeof setTimeout> | undefined
  let raf: ReturnType<typeof requestAnimationFrame> | undefined

  const clear = () => {
    if (timer !== undefined) clearTimeout(timer)
    if (raf !== undefined) cancelAnimationFrame(raf)
    timer = undefined
    raf = undefined
  }

  createEffect(
    on(
      () => [ws.state.activeConversationID, approvals().length] as const,
      ([id, count], previous) => {
        clear()
        const decision = composerDockState({
          count,
          sessionChanged: !previous || previous[0] !== id,
          wasOpen: store.dock,
        })
        if (decision === "hide") {
          setStore({ sessionID: id, dock: false, closing: false, opening: false })
          return
        }
        if (decision === "open") {
          const hidden = !store.dock || store.closing
          setStore({ sessionID: id, dock: true, closing: false })
          if (hidden) {
            setStore("opening", true)
            raf = requestAnimationFrame(() => {
              setStore("opening", false)
              raf = undefined
            })
          }
          return
        }
        setStore({ sessionID: id, dock: true, opening: false, closing: true })
        if (!timer) {
          timer = setTimeout(() => {
            setStore({ dock: false, closing: false })
            timer = undefined
          }, closeMs())
        }
      },
    ),
  )

  onCleanup(clear)

  const responding = createMemo(() => {
    const req = approvalRequest()
    if (!req) return false
    return store.responding === req.id
  })

  // A decision is applied once. The responding lock is held until the server
  // resolves the approval; the approval leaves the pending list on resolution.
  async function decide(id: string, status: "allowed" | "denied"): Promise<void> {
    if (store.responding === id) return
    setStore("responding", id)
    try {
      await ws.resolveApproval(id, status)
    } finally {
      setStore("responding", (current) => (current === id ? undefined : current))
    }
  }

  return {
    approvals,
    approvalRequest,
    responding,
    decide,
    dock: () => store.dock,
    closing: () => store.closing,
    opening: () => store.opening,
    // Wayshard keeps the composer usable while an approval is pending; the
    // approval dock sits above the prompt rather than replacing it.
    blocked: () => false,
    request: {
      busy: () => ws.state.busy,
      run: () => ws.state.run,
      running: () => ws.state.run?.status === "running" || ws.state.run?.status === "planning",
      retryable: () => ws.state.run?.status === "failed" || ws.state.run?.status === "blocked",
    },
  }
}

export type SessionComposerController = ReturnType<typeof createSessionComposerController>
