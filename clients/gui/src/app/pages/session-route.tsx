// Wayshard session route (temporary bridge during the application-level port).
//
// Routes a Wayshard Conversation selected from Home into the session shell.
// The session composition is replaced by an adapted descendant of OpenCode's
// pages/session.tsx in a later stage of this port; until then this bridge keeps
// the session experience functional while the application root/home/layout are
// migrated.
import { createEffect } from "solid-js"
import { useParams } from "../router"
import { useGlobal } from "../context/global"
import { SessionShell } from "../session-shell"

export function SessionRoute() {
  const params = useParams<{ dir: string; id?: string }>()
  const global = useGlobal()

  createEffect(() => {
    const dir = params.dir
    const id = params.id
    if (dir && dir !== global.projects.selected()?.id) void global.projects.select(dir)
    if (dir && id) void global.sessions.open(dir, id)
  })

  return <SessionShell />
}
