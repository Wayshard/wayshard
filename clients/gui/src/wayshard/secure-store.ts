// Platform-secure device-credential storage.
//
// Native clients (Desktop and Android, Tauri) store the device credential in
// platform-secure storage through the host keychain. Browser clients never store
// the credential: they authenticate with the server's HttpOnly session cookie
// and never place a bearer token in web storage.
export type CredentialStoreKind = "tauri" | "browser"

export interface CredentialStore {
  kind: CredentialStoreKind
  load(): Promise<string>
  save(credential: string): Promise<void>
  clear(): Promise<void>
}

type TauriInvoke = (cmd: string, args?: Record<string, unknown>) => Promise<unknown>

// tauriInvoke returns the Tauri v2 invoke bridge when running inside a Tauri
// webview, without a static import that would fail in a plain browser.
function tauriInvoke(): TauriInvoke | null {
  const g = globalThis as { __TAURI_INTERNALS__?: { invoke?: TauriInvoke } }
  const fn = g.__TAURI_INTERNALS__?.invoke
  return typeof fn === "function" ? fn : null
}

export function credentialStore(): CredentialStore {
  const invoke = tauriInvoke()
  if (invoke) {
    return {
      kind: "tauri",
      async load() {
        try {
          const v = await invoke("load_credential")
          return typeof v === "string" ? v : ""
        } catch {
          return ""
        }
      },
      async save(credential: string) {
        await invoke("save_credential", { credential })
      },
      async clear() {
        try {
          await invoke("clear_credential")
        } catch {
          // best effort
        }
      },
    }
  }
  return {
    kind: "browser",
    async load() {
      return ""
    },
    async save() {
      // Browser clients use the HttpOnly server session; nothing is stored.
    },
    async clear() {
      // Browser clients use the HttpOnly server session; nothing is stored.
    },
  }
}
