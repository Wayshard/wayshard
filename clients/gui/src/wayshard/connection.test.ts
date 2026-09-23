import { describe, expect, test } from "bun:test"
import { loadConnection, saveConnection } from "./state"

// The device credential must never be written to web storage. Only the endpoint
// is persisted; native clients use platform-secure storage and browser clients
// use the HttpOnly server session.
describe("connection persistence", () => {
  test("saveConnection persists only the endpoint", () => {
    const mem = new Map<string, string>()
    const g = globalThis as {
      localStorage?: unknown
      location?: unknown
    }
    const origLS = g.localStorage
    const origLoc = g.location
    g.localStorage = {
      getItem: (k: string) => mem.get(k) ?? null,
      setItem: (k: string, v: string) => {
        mem.set(k, v)
      },
      removeItem: (k: string) => {
        mem.delete(k)
      },
    }
    g.location = { origin: "http://127.0.0.1:7420" }
    try {
      saveConnection({ baseUrl: "http://127.0.0.1:7420", token: "SECRET-TOKEN" })
      const raw = mem.get("wayshard.connection") ?? ""
      expect(raw).toContain("baseUrl")
      expect(raw).not.toContain("SECRET-TOKEN")
      expect(raw).not.toContain("token")
      const loaded = loadConnection()
      expect(loaded.baseUrl).toBe("http://127.0.0.1:7420")
      expect(loaded.token).toBe("")
    } finally {
      g.localStorage = origLS
      g.location = origLoc
    }
  })
})
