// Wayshard graphical terminal.
//
// Adapted from the imported OpenCode 2 graphical terminal
// (third_party/opencode-v1.18.31/packages/app/src/components/terminal.tsx):
// the ghostty-web renderer, FitAddon, onData/onResize wiring and loss handling
// are retained. The process is the Wayshard server-owned PTY; the client only
// renders and forwards input.
import { Show, createSignal, onCleanup } from "solid-js"
import type { FitAddon, Ghostty, Terminal as GhosttyTerminal } from "ghostty-web"
import { Button } from "@wayshard/ui/button"
import { Icon } from "@wayshard/ui/icon"
import { EmptyState, ErrorState } from "./session-shell"
import { useWayshard } from "../wayshard/state"

let shared: Promise<{ mod: typeof import("ghostty-web"); ghostty: Ghostty }> | undefined

function loadGhostty() {
  shared ??= import("ghostty-web").then(async (mod) => ({ mod, ghostty: await mod.Ghostty.load() }))
  return shared
}

type TerminalState = "idle" | "connecting" | "ready" | "lost"

export function Terminal(props: { projectId: string }) {
  const ws = useWayshard()
  const [state, setState] = createSignal<TerminalState>("idle")
  const [reason, setReason] = createSignal("")
  const [title, setTitle] = createSignal("")
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
  onCleanup(cleanup)

  async function start() {
    cleanup()
    setReason("")
    setState("connecting")
    try {
      const created = await ws.client().startTerminal(props.projectId)
      const { mod, ghostty } = await loadGhostty()
      term = new mod.Terminal({ ghostty, fontSize: 13, cursorBlink: true, scrollback: 10000, convertEol: false })
      fit = new mod.FitAddon()
      term.loadAddon(fit)
      if (container) term.open(container)
      term.onTitleChange((value: string) => setTitle(value))
      term.onData((data: string) => {
        if (socket?.readyState === WebSocket.OPEN) socket.send(data)
      })
      term.onResize((size: { cols: number; rows: number }) => {
        if (socket?.readyState === WebSocket.OPEN) socket.send(JSON.stringify({ wayshard: "resize", cols: size.cols, rows: size.rows }))
      })
      socket = ws.client().ptySocket(created.id)
      socket.binaryType = "arraybuffer"
      socket.onopen = () => {
        setState("ready")
        fit?.fit()
      }
      socket.onmessage = (ev) => {
        const data = typeof ev.data === "string" ? ev.data : new Uint8Array(ev.data as ArrayBuffer)
        term?.write(data as never)
      }
      socket.onclose = () => {
        setState("lost")
        setReason("server restart or process exit")
      }
      socket.onerror = () => {
        setState("lost")
        setReason("terminal socket error")
      }
    } catch (err) {
      setState("lost")
      setReason(String(err))
    }
  }

  return (
    <div class="wh-panel wh-terminal-panel">
      <div class="wh-panel-header">
        <h2>
          Terminal <Show when={title()}>{(t) => <span class="wh-muted">{t()}</span>}</Show>
        </h2>
        <Button size="small" variant="primary" icon="terminal" onClick={() => void start()}>
          {state() === "idle" ? "New terminal" : "New session"}
        </Button>
      </div>
      <p class="wh-muted">
        <Icon name="terminal" size="small" /> Server-owned PTY. The client never executes local shell commands.
      </p>
      <Show when={state() === "lost"}>
        <ErrorState title="Terminal lost" detail={`${reason()} — create a new terminal to reconnect.`} />
      </Show>
      <Show when={state() === "idle"}>
        <EmptyState title="No terminal attached" body="Create a server-owned terminal to begin." />
      </Show>
      <div ref={container} class="wh-terminal-renderer" data-state={state()} />
    </div>
  )
}
