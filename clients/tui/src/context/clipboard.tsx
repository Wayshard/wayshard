// Adapted from the imported OpenCode TUI clipboard context. Wayshard does not
// run a local terminal clipboard service here; the server owns no clipboard, so
// this is a best-effort no-op adapter.
import { createContext, type JSX, useContext } from "solid-js"

export type ClipboardContent = Readonly<{ data: string; mime: string }>
export type ClipboardService = Readonly<{
  read?(): Promise<ClipboardContent | undefined>
  write?(text: string): Promise<void>
}>

const clipboard: ClipboardService = {
  async read() {
    return undefined
  },
  async write() {},
}

const ClipboardContext = createContext<ClipboardService>(clipboard)

export function ClipboardProvider(props: { value?: ClipboardService; children: JSX.Element }) {
  return <ClipboardContext.Provider value={props.value ?? clipboard}>{props.children}</ClipboardContext.Provider>
}

export function useClipboard() {
  return useContext(ClipboardContext)
}
