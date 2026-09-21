// Wayshard application router.
//
// Adapts the routing abstraction used by the imported OpenCode application
// (third_party/opencode-v1.18.31/packages/app/src/app.tsx, which uses
// @solidjs/router) into a small Wayshard-owned router. It preserves the same
// hook names and shapes the adapted application files expect
// (useNavigate/useLocation/useParams) and the same route states
// (Home / New Session / Project Session), backed by the History API.
//
// The inherited @solidjs/router package did not advance its reactive location
// inside the adapted Wayshard provider tree (navigate() pushed history but the
// route never re-rendered), so the abstraction is reimplemented here rather than
// carried as a broken dependency. Route states and URL scheme are
// implementation-dependent per the port requirements.
//
// Route state is an eager module-level signal, matching the pattern the
// graphical shell already uses for its own tab state.
import { createSignal } from "solid-js"

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

function currentPathname(): string {
  return typeof window === "object" ? window.location.pathname : "/"
}

const [pathname, setPathname] = createSignal(currentPathname())

export function syncRouter(): void {
  setPathname(currentPathname())
}

export function navigate(to: string, options?: NavigateOptions): void {
  if (typeof window !== "object") return
  const next = new URL(to, window.location.origin)
  const value = next.pathname + next.search + next.hash
  // Interim bridge: real document navigation keeps routes reliable while the
  // adapted provider tree is migrated. The final port restores in-place
  // client-side transitions once the reactive integration is settled.
  if (options?.replace) window.location.replace(value)
  else window.location.assign(value)
}

export function useNavigate(): (to: string, options?: NavigateOptions) => void {
  return navigate
}

export function useLocation(): Location {
  return {
    get pathname() {
      return pathname()
    },
    get search() {
      return typeof window === "object" ? window.location.search : ""
    },
    get hash() {
      return typeof window === "object" ? window.location.hash : ""
    },
  }
}

export function useParams<T extends Record<string, string | undefined> = Record<string, string | undefined>>(): T {
  return new Proxy({} as T, {
    get: (_target, key: string) => (matchRoute(pathname()).params as Record<string, string | undefined>)[key],
    has: () => true,
  })
}

export function useRouteMatch(): () => RouteMatch {
  return () => matchRoute(pathname())
}

// RouterProvider is retained for API parity with the adapted app root; route
// state is a module-level signal, so provider re-renders cannot orphan consumers.
export function RouterProvider(props: { children?: unknown }) {
  return props.children as never
}
