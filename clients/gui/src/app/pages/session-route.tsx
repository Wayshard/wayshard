// Wayshard session route.
//
// Routes a Wayshard Conversation selected from Home into the session shell and
// keeps the URL and the selected project/session in sync. The session
// composition itself is the adapted OpenCode-derived session page
// (../session-page.tsx); this module is the thin route adapter that binds the
// router params to Wayshard domain state.
import { createEffect } from "solid-js"
import { useParams } from "../router"
import { useGlobal } from "../context/global"
import { SessionPage } from "../session-page"

export function SessionRoute() {
  const params = useParams<{ dir: string; id?: string }>()
  const global = useGlobal()

  createEffect(() => {
    const dir = params.dir
    const id = params.id
    if (dir && dir !== global.projects.selected()?.id) void global.projects.select(dir)
    if (dir && id) void global.sessions.open(dir, id)
  })

  return <SessionPage />
}
