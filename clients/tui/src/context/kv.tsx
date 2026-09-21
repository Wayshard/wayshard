// Wayshard TUI key-value store adapter. Adapted from the imported OpenCode TUI
// kv context; Wayshard keeps this in memory for the session.
import { createContext, useContext, type JSX } from "solid-js"
import { createStore } from "solid-js/store"

interface KV {
  get<T>(key: string, fallback: T): T
  set<T>(key: string, value: T): void
}

const defaultKV: KV = { get: (_k, f) => f, set: () => {} }
const KVContext = createContext<KV>(defaultKV)

export function KVProvider(props: { children: JSX.Element }) {
  const [store, setStore] = createStore<Record<string, unknown>>({})
  const value: KV = {
    get<T>(key: string, fallback: T): T {
      return (store[key] as T) ?? fallback
    },
    set<T>(key: string, v: T) {
      setStore(key, v)
    },
  }
  return <KVContext.Provider value={value}>{props.children}</KVContext.Provider>
}

export function useKV() {
  return useContext(KVContext)
}
