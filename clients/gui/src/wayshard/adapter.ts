// Maps Wayshard domain data into the view-model consumed by the adapted
// OpenCode-derived session presentation layer. The presentation lineage is
// inherited; the data is entirely Wayshard.
import type { Conversation, Project, Run, Stage } from "@wayshard/sdk"
import type { Data } from "@wayshard/gui/session-ui/context"
import type { AssistantMessage, Message, Part, Session, SessionStatus, UserMessage } from "@wayshard/gui/session-ui/sdk/model"

export interface RunView {
  run: Run
  stages: Stage[]
}

const stageLabel: Record<string, string> = {
  plan: "Planning",
  explore: "Exploring",
  execute: "Executing",
  validate: "Validating",
  review: "Reviewing",
  repair: "Repairing",
  replan: "Replanning",
  integrate: "Integrating",
}

export function stageDisplay(kind: string): string {
  return stageLabel[kind] ?? kind.charAt(0).toUpperCase() + kind.slice(1)
}

function stageStatusText(stage: Stage): string {
  return `${stageDisplay(stage.kind)} · ${stage.status}`
}

// buildData converts the Wayshard conversation stream into the session-ui Data
// shape so the inherited message/turn components can render it.
export function buildData(args: {
  project?: Project
  conversations: Conversation[]
  activeConversationID: string | null
  messages: { id: string; role: string; body: string }[]
  runs: Record<string, Run>
  runView?: RunView
}): Data {
  const sessions: Session[] = args.conversations.map((c) => ({ id: c.id, title: c.title || "Session" }))
  const message: Data["message"] = {}
  const part: Data["part"] = {}
  const session_status: Data["session_status"] = {}
  const session_diff: Data["session_diff"] = {}

  for (const c of args.conversations) {
    message[c.id] = []
    part[c.id] = []
    session_diff[c.id] = []
  }

  if (args.activeConversationID) {
    const list: Message[] = []
    for (const m of args.messages) {
      if (m.role === "user") {
        const um: UserMessage = { id: m.id, role: "user", time: { created: 0 }, text: m.body } as UserMessage
        list.push(um)
        part[m.id] = [{ id: `${m.id}-text`, type: "text", text: m.body } as Part]
      } else {
        const am: AssistantMessage = {
          id: m.id,
          role: "assistant",
          time: { completed: 1 },
          text: m.body,
        } as AssistantMessage
        list.push(am)
        part[m.id] = [{ id: `${m.id}-text`, type: "text", text: m.body } as Part]
      }
    }
    // Represent the active run as a final assistant turn with one tool part per
    // stage so the inherited timeline renders the Wayshard stage progression.
    const rv = args.runView
    if (rv) {
      const runID = `${rv.run.id}-run`
      const am: AssistantMessage = { id: runID, role: "assistant", parentID: list.at(-1)?.id, time: {}, text: "" } as AssistantMessage
      list.push(am)
      part[runID] = rv.stages.map((s) => ({
        id: s.id,
        type: "tool",
        tool: "stage",
        state: { status: s.status, title: stageStatusText(s), input: { stage: s.kind, ordinal: s.ordinal }, metadata: {} },
      }) as Part)
      session_status[args.activeConversationID] = {
        type: rv.run.status === "running" ? "busy" : "idle",
        status: rv.run.status,
      } as SessionStatus
    }
    message[args.activeConversationID] = list
  }

  return { session: sessions, session_status, session_diff, message, part } as Data
}
