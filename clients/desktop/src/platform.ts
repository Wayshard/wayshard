// Desktop/Android platform adapter. Tauri 2 is a shell: the product UI is the
// shared @wayshard/gui application. This module holds only platform-specific
// capabilities and degrades gracefully in a plain browser (Web).
type TauriGlobal = {
  core?: { invoke: (cmd: string, args?: Record<string, unknown>) => Promise<unknown> }
  event?: { listen: (event: string, cb: (e: unknown) => void) => Promise<() => void> }
}

function tauri(): TauriGlobal | undefined {
  return (globalThis as { __TAURI__?: TauriGlobal }).__TAURI__
}

export function isDesktop(): boolean {
  return !!tauri()?.core
}

export async function setWindowTitle(title: string): Promise<void> {
  const t = tauri()
  if (!t?.core) {
    document.title = title
    return
  }
  await t.core.invoke("set_title", { title })
}

export async function notify(title: string, body: string): Promise<void> {
  if (typeof Notification === "undefined") return
  if (Notification.permission === "granted") new Notification(title, { body })
}

export async function openExternal(url: string): Promise<void> {
  const t = tauri()
  if (t?.core) {
    await t.core.invoke("open_external", { url })
    return
  }
  window.open(url, "_blank", "noopener,noreferrer")
}
