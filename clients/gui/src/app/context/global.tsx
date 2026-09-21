// Wayshard application data context.
//
// Adapted from the imported OpenCode application context family
// (third_party/opencode-v1.18.31/packages/app/src/context/global.tsx and
// context/server-sync.tsx): the application-level provider shape is retained
// (projects, sessions, selection, server health, open/new-session actions)
// while the data source is the Wayshard server via @wayshard/sdk. OpenCode's
// multi-server/workspace model is replaced by Wayshard's single server and
// Project -> Conversation model.
import { createMemo, createSignal, type Accessor } from "solid-js"
import { createSimpleContext } from "@wayshard/ui/context/helper"
import type { Conversation, Project } from "@wayshard/sdk"
import { useWayshard } from "../../wayshard/state"

export interface SessionEntry {
  id: string
  projectID: string
  title: string
}

function init() {
  const ws = useWayshard()
  const [sessions, setSessions] = createSignal<Record<string, Conversation[]>>({})
  const [loading, setLoading] = createSignal<Record<string, boolean>>({})

  const projects = createMemo<Project[]>(() => ws.state.projects)

  function project(id: string | null): Project | undefined {
    return id ? projects().find((p) => p.id === id) : undefined
  }

  function list(projectID: string): Conversation[] {
    return sessions()[projectID] ?? []
  }

  async function load(projectID: string): Promise<Conversation[]> {
    setLoading((prev) => ({ ...prev, [projectID]: true }))
    try {
      const list = await ws.client().conversations(projectID)
      setSessions((prev) => ({ ...prev, [projectID]: list }))
      return list
    } finally {
      setLoading((prev) => ({ ...prev, [projectID]: false }))
    }
  }

  function search(projectID: string, query: string): Conversation[] {
    const q = query.trim().toLowerCase()
    if (!q) return list(projectID)
    return list(projectID).filter((c) => (c.title || "").toLowerCase().includes(q))
  }

  async function openProject(path: string): Promise<Project> {
    const project = await ws.client().openProject(path)
    await ws.refreshAll()
    await load(project.id)
    return project
  }

  async function newSession(projectID: string): Promise<Conversation> {
    const c = await ws.client().createConversation(projectID)
    await load(projectID)
    return c
  }

  return {
    server: {
      connected: () => ws.state.connected,
      error: () => ws.state.connectionError,
    },
    projects: {
      list: projects,
      get: project,
      selected: () => project(ws.state.activeProjectID),
      select: (id: string) => ws.selectProject(id),
      open: openProject,
      recent: createMemo(() => projects().slice(0, 8)),
    },
    sessions: {
      list,
      loading: (projectID: string) => loading()[projectID] ?? false,
      load,
      search,
      open: (projectID: string, conversationID: string) => ws.selectConversation(conversationID),
      new: newSession,
      count: (projectID: string) => list(projectID).length,
    },
    client: ws.client,
  }
}

export const { use: useGlobal, provider: GlobalProvider } = createSimpleContext({ name: "Global", init })
