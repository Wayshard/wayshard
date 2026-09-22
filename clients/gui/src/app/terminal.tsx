// Wayshard graphical terminal renderer.
//
// Adapted from the imported OpenCode 2 graphical terminal
// (third_party/opencode-v1.18.31/packages/app/src/components/terminal.tsx):
// the ghostty-web renderer, FitAddon, onData/onResize wiring, focus surface
// (`data-component="terminal"`) and connect/loss handling are retained. The
// process is the Wayshard server-owned PTY; the client only renders and forwards
// input and never spawns a shell.
//
// This component binds to one already-created server PTY id. PTY lifecycle
// (create/list/close/active) is owned by the adapted terminal panel
// (./pages/session/terminal-panel-v2.tsx), mirroring the upstream split between
// the terminal context and the renderer component.
import { Show, createEffect, createSignal, onCleanup, onMount } from "solid-js"
import type { FitAddon, Ghostty, Terminal as GhosttyTerminal } from "ghostty-web"
import { Button } from "@wayshard/ui/button"
import { Icon } from "@wayshard/ui/icon"
import { useWayshard } from "../wayshard/state"

let shared: Promise<{ mod: typeof import("ghostty-web"); ghostty: Ghostty }> | undefined

function loadGhostty() {
  shared ??= import("ghostty-web").then(async (mod) => ({ mod, ghostty: await mod.Ghostty.load() }))
  return shared
}

type TerminalState = "connecting" | "ready" | "lost"

export interface TerminalViewProps {
  ptyId: string
  active?: boolean
  onConnect?: () => void
  onConnectError?: (reason: string) => void
  onTitleChange?: (title: string) => void
}

export function TerminalView(props: TerminalViewProps) {
  const ws = useWayshard()
  const [state, setState] = createSignal<TerminalState>("connecting")
  const [reason, setReason] = createSignal("")
  let container: HTMLDivElement | undefined
  let socket: WebSocket | undefined
  let term: GhosttyTerminal | undefined
  let fit: FitAddon | undefined

  function cleanup() {
    socket?.close()
    socket = undefined
    term?.dispose()
    term = undefined
    fit = undefined
  }

  // Attach opens a WebSocket to the existing server-owned PTY. It never creates
  // or owns the process; a detach (socket close) leaves the PTY running.
  async function attach() {
    cleanup()
    setReason("")
    setState("connecting")
    try {
      const { mod, ghostty } = await loadGhostty()
      term = new mod.Terminal({ ghostty, fontSize: 13, cursorBlink: true, scrollback: 10000, convertEol: false })
      fit = new mod.FitAddon()
      term.loadAddon(fit)
      if (container) term.open(container)
      term.onTitleChange((value: string) => props.onTitleChange?.(value))
      term.onData((data: string) => {
        if (socket?.readyState === WebSocket.OPEN) socket.send(data)
      })
      term.onResize((size: { cols: number; rows: number }) => {
        if (socket?.readyState === WebSocket.OPEN) {
          socket.send(JSON.stringify({ wayshard: "resize", cols: size.cols, rows: size.rows }))
        }
      })
      socket = ws.client().ptySocket(props.ptyId)
      socket.binaryType = "arraybuffer"
      socket.onopen = () => {
        setState("ready")
        fit?.fit()
        props.onConnect?.()
      }
      socket.onmessage = (ev) => {
        const data = typeof ev.data === "string" ? ev.data : new Uint8Array(ev.data as ArrayBuffer)
        term?.write(data as never)
      }
      socket.onclose = () => {
        setState("lost")
        setReason("server restart or process exit")
        props.onConnectError?.("server restart or process exit")
      }
      socket.onerror = () => {
        setState("lost")
        setReason("terminal socket error")
        props.onConnectError?.("terminal socket error")
      }
    } catch (err) {
      setState("lost")
      setReason(String(err))
      props.onConnectError?.(String(err))
    }
  }

  onMount(() => {
    void attach()
    const resize = () => {
      if (props.active === false) return
      if (state() !== "ready") return
      fit?.fit()
    }
    window.addEventListener("resize", resize)
    onCleanup(() => window.removeEventListener("resize", resize))
  })
  onCleanup(cleanup)

  // Refit when this terminal becomes the active view or its container resizes.
  createEffect(() => {
    if (props.active === false) return
    if (state() !== "ready") return
    requestAnimationFrame(() => fit?.fit())
  })

  return (
    <div class="wh-terminal-view">
      <Show when={state() === "lost"}>
        <div class="wh-terminal-lost" data-slot="terminal-lost">
          <span>
            <Icon name="warning" size="small" /> Terminal lost: {reason()}. The server-owned PTY ended or the server
            restarted.
          </span>
          <Button size="small" variant="secondary" onClick={() => void attach()}>
            Reconnect
          </Button>
        </div>
      </Show>
      <div ref={container} class="wh-terminal-renderer" data-component="terminal" data-state={state()} />
    </div>
  )
}
