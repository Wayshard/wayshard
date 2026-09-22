// Wayshard application router.
//
// Adapts the routing abstraction used by the imported OpenCode application
// (third_party/opencode-v1.18.31/packages/app/src/app.tsx, which uses
// @solidjs/router) into a Wayshard-owned reactive SPA router. It preserves the
// same hook names and shapes the adapted application files expect
// (useNavigate/useLocation/useParams) and the same route states
// (Home / New Session / Project Session), backed by the History API.
//
// Navigation is in-place: pushState/replaceState update a reactive location
// signal and popstate synchronizes it back from the window, so route changes do
// not recreate the document. (The inherited @solidjs/router package did not
// advance its reactive location inside the adapted Wayshard provider tree, so
// the abstraction is reimplemented here rather than carried as a broken
// dependency.) Route states and URL scheme are implementation-dependent per the
// port requirements.
//
// The core is a factory over a small RouterAdapter so push/replace/popstate
// semantics are deterministically testable without a DOM; the module singleton
// binds it to the real window (or an in-memory adapter when no window exists).
import { createMemo, createSignal } from "solid-js"

export interface Location {
  readonly pathname: string
  readonly search: string
  readonly hash: string
}

export interface NavigateOptions {
  replace?: boolean
}

export type RouteMatch =
  | { name: "home"; params: Record<string, string | undefined> }
  | { name: "new-session"; params: Record<string, string | undefined> }
  | { name: "session"; params: { dir: string; id?: string } }

export function matchRoute(pathname: string): RouteMatch {
  const parts = pathname.split("/").filter(Boolean)
  if (parts.length === 0) return { name: "home", params: {} }
  if (parts[0] === "new-session") return { name: "new-session", params: {} }
  if (parts.length >= 2 && parts[1] === "session") {
    return { name: "session", params: { dir: parts[0], id: parts[2] } }
  }
  return { name: "home", params: {} }
}

// RouterAdapter isolates the History API so route behavior can be tested
// without a browser. `assign` performs a real document navigation for external
// links; it must never be used for ordinary in-app navigation.
export interface RouterAdapter {
  read(): Location
  push(href: string): void
  replace(href: string): void
  assign(href: string): void
  subscribe(fn: () => void): () => void
}

export interface Router {
  location: () => Location
  navigate: (to: string, options?: NavigateOptions) => void
  sync: () => void
  dispose: () => void
}

const ABSOLUTE_URL = /^[a-zA-Z][a-zA-Z0-9+.-]*:\/\//

function sameLocation(a: Location, b: Location): boolean {
  return a.pathname === b.pathname && a.search === b.search && a.hash === b.hash
}

// resolveTarget turns a navigation target into either an in-app href
// (pathname+search+hash) or an external absolute URL that must use ordinary
// browser navigation.
export function resolveTarget(to: string, current: Location): { external: string | null; href: string } {
  if (ABSOLUTE_URL.test(to)) return { external: to, href: "" }
  const base = `http://localhost${current.pathname}${current.search}`
  const url = new URL(to, base)
  return { external: null, href: url.pathname + url.search + url.hash }
}

export function createRouter(adapter: RouterAdapter): Router {
  const [location, setLocation] = createSignal<Location>(adapter.read())

  function sync(): void {
    const next = adapter.read()
    if (!sameLocation(location(), next)) setLocation(next)
  }

  function navigate(to: string, options?: NavigateOptions): void {
    const { external, href } = resolveTarget(to, adapter.read())
    if (external !== null) {
      adapter.assign(external)
      return
    }
    if (options?.replace) adapter.replace(href)
    else adapter.push(href)
    sync()
  }

  const unsubscribe = adapter.subscribe(sync)

  return { location, navigate, sync, dispose: unsubscribe }
}

function browserAdapter(): RouterAdapter {
  return {
    read: () => ({
      pathname: window.location.pathname,
      search: window.location.search,
      hash: window.location.hash,
    }),
    push: (href) => window.history.pushState(null, "", href),
    replace: (href) => window.history.replaceState(null, "", href),
    assign: (href) => window.location.assign(href),
    subscribe: (fn) => {
      window.addEventListener("popstate", fn)
      return () => window.removeEventListener("popstate", fn)
    },
  }
}

// memoryAdapter backs non-window environments (tests, SSR). It preserves the
// same observable semantics without touching a global.
function memoryAdapter(initial: string = "/"): RouterAdapter {
  const parse = (href: string): Location => {
    const url = new URL(href, "http://localhost")
    return { pathname: url.pathname, search: url.search, hash: url.hash }
  }
  let current = parse(initial)
  let listeners: Array<() => void> = []
  return {
    read: () => current,
    push: (href) => {
      current = parse(href)
    },
    replace: (href) => {
      current = parse(href)
    },
    assign: (href) => {
      current = parse(href)
    },
    subscribe: (fn) => {
      listeners = [...listeners, fn]
      return () => {
        listeners = listeners.filter((l) => l !== fn)
      }
    },
  }
}

const router: Router = typeof window === "object" ? createRouter(browserAdapter()) : createRouter(memoryAdapter())

export function navigate(to: string, options?: NavigateOptions): void {
  router.navigate(to, options)
}

export function syncRouter(): void {
  router.sync()
}

export function useNavigate(): (to: string, options?: NavigateOptions) => void {
  return router.navigate
}

export function useLocation(): Location {
  return {
    get pathname() {
      return router.location().pathname
    },
    get search() {
      return router.location().search
    },
    get hash() {
      return router.location().hash
    },
  }
}

export function useParams<T extends Record<string, string | undefined> = Record<string, string | undefined>>(): T {
  return new Proxy({} as T, {
    get: (_target, key: string) =>
      (matchRoute(router.location().pathname).params as Record<string, string | undefined>)[key],
    has: () => true,
  })
}

export function useRouteMatch(): () => RouteMatch {
  return createMemo(() => matchRoute(router.location().pathname))
}

// RouterProvider is retained for API parity with the adapted app root; route
// state is a module-level signal, so provider re-renders cannot orphan consumers.
export function RouterProvider(props: { children?: unknown }) {
  return props.children as never
}
