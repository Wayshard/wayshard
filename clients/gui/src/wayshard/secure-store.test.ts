import { describe, expect, test } from "bun:test"
import { credentialStore } from "./secure-store"

describe("credentialStore", () => {
  test("browser clients never store the credential", async () => {
    const g = globalThis as { __TAURI_INTERNALS__?: unknown }
    delete g.__TAURI_INTERNALS__
    const store = credentialStore()
    expect(store.kind).toBe("browser")
    await store.save("secret")
    expect(await store.load()).toBe("")
    await store.clear()
  })

  test("native clients use the host keychain commands", async () => {
    const calls: Array<{ cmd: string; args?: Record<string, unknown> }> = []
    let value = ""
    const g = globalThis as {
      __TAURI_INTERNALS__?: { invoke: (cmd: string, args?: Record<string, unknown>) => Promise<unknown> }
    }
    g.__TAURI_INTERNALS__ = {
      invoke: async (cmd, args) => {
        calls.push({ cmd, args })
        if (cmd === "save_credential") value = String(args?.credential ?? "")
        if (cmd === "load_credential") return value
        return null
      },
    }
    try {
      const store = credentialStore()
      expect(store.kind).toBe("tauri")
      await store.save("cred-1")
      expect(await store.load()).toBe("cred-1")
      await store.clear()
      expect(calls.map((c) => c.cmd)).toEqual(["save_credential", "load_credential", "clear_credential"])
    } finally {
      delete g.__TAURI_INTERNALS__
    }
  })
})
