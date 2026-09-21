import { describe, expect, test } from "bun:test"
import { buildData, stageDisplay } from "./adapter"

describe("wayshard -> session-ui adapter", () => {
  test("maps conversations to sessions and messages to parts", () => {
    const data = buildData({
      conversations: [{ id: "c1", projectId: "p1", title: "Task" }],
      activeConversationID: "c1",
      messages: [
        { id: "m1", role: "user", body: "do the thing" },
        { id: "m2", role: "assistant", body: "working" },
      ],
      runs: {},
    })
    expect(data.session.length).toBe(1)
    expect(data.session[0].id).toBe("c1")
    expect(data.message["c1"].length).toBe(2)
    expect(data.message["c1"][0].role).toBe("user")
    expect(data.part["m1"][0].type).toBe("text")
  })

  test("represents a run as a stage timeline tool part", () => {
    const data = buildData({
      conversations: [{ id: "c1", projectId: "p1", title: "Task" }],
      activeConversationID: "c1",
      messages: [{ id: "m1", role: "user", body: "go" }],
      runs: {},
      runView: {
        run: { id: "r1", taskId: "t1", projectId: "p1", conversationId: "c1", status: "running", profile: "auto" },
        stages: [
          { id: "s1", runId: "r1", kind: "plan", ordinal: 1, status: "succeeded" },
          { id: "s2", runId: "r1", kind: "execute", ordinal: 2, status: "running" },
        ],
      },
    })
    const parts = data.part["r1-run"]
    expect(parts.length).toBe(2)
    expect(parts[0].type).toBe("tool")
    expect(data.session_status["c1"].type).toBe("busy")
  })

  test("stage labels are human readable", () => {
    expect(stageDisplay("plan")).toBe("Planning")
    expect(stageDisplay("execute")).toBe("Executing")
    expect(stageDisplay("integrate")).toBe("Integrating")
  })
})
