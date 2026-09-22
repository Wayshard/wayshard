// Router tests: prove in-place SPA navigation semantics (push/replace/popstate)
// without a document reload, including query/hash preservation and the external
// link escape hatch. These exercise the real RouterAdapter contract via a fake
// adapter, so they are deterministic and DOM-free.
import { describe, expect, test } from "bun:test"
import { createRouter, matchRoute, resolveTarget, type Location, type RouterAdapter } from "./router"

function fakeAdapter(initial: Location = { pathname: "/", search: "", hash: "" }) {
  let current = initial
  let listeners: Array<() => void> = []
  const calls: Array<{ op: "push" | "replace" | "assign"; href: string }> = []
  const adapter: RouterAdapter = {
    read: () => current,
    push: (href) => {
      calls.push({ op: "push", href })
      const url = new URL(href, "http://localhost")
      current = { pathname: url.pathname, search: url.search, hash: url.hash }
    },
    replace: (href) => {
      calls.push({ op: "replace", href })
      const url = new URL(href, "http://localhost")
      current = { pathname: url.pathname, search: url.search, hash: url.hash }
    },
    assign: (href) => {
      calls.push({ op: "assign", href })
    },
    subscribe: (fn) => {
      listeners = [...listeners, fn]
      return () => {
        listeners = listeners.filter((l) => l !== fn)
      }
    },
  }
  return {
    adapter,
    calls,
    // emit simulates a browser popstate / back-forward event.
    emit(href: string) {
      const url = new URL(href, "http://localhost")
      current = { pathname: url.pathname, search: url.search, hash: url.hash }
      for (const fn of listeners) fn()
    },
  }
}

// tracked reads a Solid signal/reactor outside a component by re-running.
function read<T>(fn: () => T): T {
  return fn()
}

describe("matchRoute", () => {
  test("home", () => {
    expect(matchRoute("/").name).toBe("home")
    expect(matchRoute("").name).toBe("home")
  })
  test("new-session", () => {
    expect(matchRoute("/new-session").name).toBe("new-session")
  })
  test("session carries dir and id", () => {
    const m = matchRoute("/prj_1/session/cnv_2")
    expect(m.name).toBe("session")
    expect(m.params).toEqual({ dir: "prj_1", id: "cnv_2" })
  })
})

describe("resolveTarget", () => {
  const current: Location = { pathname: "/prj/session/c1", search: "?x=1", hash: "#bottom" }
  test("relative app path resolves to pathname+search+hash", () => {
    expect(resolveTarget("/new-session?a=1#b", current)).toEqual({ external: null, href: "/new-session?a=1#b" })
  })
  test("relative target inherits current directory", () => {
    expect(resolveTarget("sibling", current)).toEqual({ external: null, href: "/prj/session/sibling" })
  })
  test("absolute external URL is flagged external", () => {
    expect(resolveTarget("https://example.com/x", current)).toEqual({ external: "https://example.com/x", href: "" })
  })
})

describe("createRouter — in-place navigation", () => {
  test("navigate updates the reactive location without a document navigation", () => {
    const fake = fakeAdapter()
    const router = createRouter(fake.adapter)
    router.navigate("/prj/session/c1")
    expect(router.location().pathname).toBe("/prj/session/c1")
    expect(fake.calls[0].op).toBe("push")
    expect(fake.calls.some((c) => c.op === "assign")).toBe(false)
  })

  test("navigate('/') returns to home", () => {
    const fake = fakeAdapter({ pathname: "/prj/session/c1", search: "", hash: "" })
    const router = createRouter(fake.adapter)
    router.navigate("/")
    expect(matchRoute(router.location().pathname).name).toBe("home")
  })

  test("replace uses replaceState, not pushState", () => {
    const fake = fakeAdapter()
    const router = createRouter(fake.adapter)
    router.navigate("/a", { replace: true })
    expect(fake.calls).toEqual([{ op: "replace", href: "/a" }])
    expect(router.location().pathname).toBe("/a")
  })

  test("popstate synchronizes the location back from the window", () => {
    const fake = fakeAdapter()
    const router = createRouter(fake.adapter)
    router.navigate("/prj/session/c1")
    fake.emit("/other/session/c2?tab=changes#row-3")
    expect(router.location()).toEqual({ pathname: "/other/session/c2", search: "?tab=changes", hash: "#row-3" })
  })

  test("query and hash are preserved across navigation", () => {
    const fake = fakeAdapter()
    const router = createRouter(fake.adapter)
    router.navigate("/prj/session/c1?panel=files#top")
    expect(router.location().search).toBe("?panel=files")
    expect(router.location().hash).toBe("#top")
  })

  test("external URL never pushes history and assigns instead", () => {
    const fake = fakeAdapter()
    const router = createRouter(fake.adapter)
    router.navigate("https://example.com/")
    expect(fake.calls).toEqual([{ op: "assign", href: "https://example.com/" }])
  })

  test("dispose unsubscribes popstate", () => {
    const fake = fakeAdapter()
    const router = createRouter(fake.adapter)
    router.dispose()
    fake.emit("/after-dispose")
    expect(router.location().pathname).toBe("/")
  })
})

describe("createRouter — reactive consumers", () => {
  test("location signal drives matchRoute for consumers", () => {
    const fake = fakeAdapter()
    const router = createRouter(fake.adapter)
    const match = () => matchRoute(read(() => router.location().pathname))
    expect(match().name).toBe("home")
    router.navigate("/prj/session/c1")
    expect(match().name).toBe("session")
    router.navigate("/")
    expect(match().name).toBe("home")
  })
})
