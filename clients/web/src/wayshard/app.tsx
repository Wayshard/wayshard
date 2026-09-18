import { createEffect, createSignal, For, onCleanup, onMount, Show } from "solid-js";
import {
  WayshardClient,
  type Approval,
  type Artifact,
  type Conversation,
  type Notification,
  type Project,
  type Run,
  type Stage,
} from "@wayshard/sdk";

const client = new WayshardClient(typeof location !== "undefined" ? location.origin : "http://127.0.0.1:7420");

type Tab = "session" | "changes" | "files" | "terminal";
type Overlay =
  | "none"
  | "usage"
  | "details"
  | "routing"
  | "context"
  | "artifacts"
  | "knowledge"
  | "recovery"
  | "settings"
  | "notifications"
  | "approvals";

const stageLabel: Record<string, string> = {
  plan: "Planning",
  explore: "Exploring",
  execute: "Executing",
  validate: "Validating",
  review: "Reviewing",
  repair: "Repairing",
  replan: "Replanning",
  integrate: "Integrating",
  assess: "Assessing",
};

export function App() {
  const [projects, setProjects] = createSignal<Project[]>([]);
  const [project, setProject] = createSignal<Project | null>(null);
  const [convs, setConvs] = createSignal<Conversation[]>([]);
  const [convId, setConvId] = createSignal("");
  const [messages, setMessages] = createSignal<Array<{ role: string; body: string }>>([]);
  const [tab, setTab] = createSignal<Tab>("session");
  const [overlay, setOverlay] = createSignal<Overlay>("none");
  const [text, setText] = createSignal("");
  const [path, setPath] = createSignal("");
  const [run, setRun] = createSignal<Run | null>(null);
  const [stages, setStages] = createSignal<Stage[]>([]);
  const [arts, setArts] = createSignal<Artifact[]>([]);
  const [notes, setNotes] = createSignal<Notification[]>([]);
  const [approvals, setApprovals] = createSignal<Approval[]>([]);
  const [err, setErr] = createSignal("");
  const [profile, setProfile] = createSignal("auto");
  const [artifactOnly, setArtifactOnly] = createSignal(false);
  const [changeView, setChangeView] = createSignal<"run" | "workspace">("run");
  const [changes, setChanges] = createSignal<unknown>(null);
  const [files, setFiles] = createSignal<Array<{ name: string; dir: boolean }>>([]);
  const [filePath, setFilePath] = createSignal("");
  const [fileBody, setFileBody] = createSignal("");
  const [fileHash, setFileHash] = createSignal("");
  const [fileConflict, setFileConflict] = createSignal("");
  const [termId, setTermId] = createSignal("");
  const [termLog, setTermLog] = createSignal("Server-owned PTY. Disconnect does not kill the process.\n");
  const [termLost, setTermLost] = createSignal("");
  const [detail, setDetail] = createSignal<unknown>(null);
  const [connected, setConnected] = createSignal(true);
  const [expanded, setExpanded] = createSignal<Record<string, boolean>>({});

  async function refreshProjects() {
    try {
      setProjects(await client.projects());
      setNotes(await client.notifications());
      setApprovals(await client.approvals());
      setConnected(true);
    } catch (e) {
      setConnected(false);
      setErr(String(e));
    }
  }

  onMount(() => {
    refreshProjects();
    const t = setInterval(refreshProjects, 4000);
    onCleanup(() => clearInterval(t));
  });

  async function selectProject(p: Project) {
    setProject(p);
    const list = await client.conversations(p.id);
    setConvs(list);
    if (list[0]) await selectConv(list[0].id);
    setFiles(await client.listFiles(p.id, ""));
  }

  async function selectConv(id: string) {
    setConvId(id);
    setMessages(await client.messages(id));
  }

  async function openProject() {
    const p = await client.openProject(path());
    await refreshProjects();
    await selectProject(p);
  }

  async function newSession() {
    const p = project();
    if (!p) return;
    const c = await client.createConversation(p.id, "Session");
    setConvs(await client.conversations(p.id));
    await selectConv(c.id);
  }

  async function send() {
    if (!convId()) return;
    try {
      const r = await client.sendMessage(convId(), text(), {
        artifactOnly: artifactOnly(),
        profile: profile(),
        idempotencyKey: crypto.randomUUID(),
      });
      setRun(r.run);
      setText("");
      poll(r.run.id);
    } catch (e) {
      setErr(String(e));
    }
  }

  async function poll(id: string) {
    const cur = await client.run(id);
    setRun(cur);
    setStages(await client.stages(id));
    setArts(await client.artifacts(id));
    setMessages(await client.messages(convId()));
    if (!["complete", "failed", "cancelled"].includes(cur.status)) {
      setTimeout(() => poll(id), 600);
    }
  }

  createEffect(() => {
    const p = project();
    const r = run();
    const v = changeView();
    if (!p) return;
    if (v === "workspace") client.workspaceChanges(p.id).then(setChanges).catch(setErr);
    else if (r) client.runChanges(r.id).then(setChanges).catch(setErr);
  });

  async function openFile(name: string) {
    const p = project();
    if (!p) return;
    const rel = filePath() ? filePath() + "/" + name : name;
    const f = files().find((x) => x.name === name);
    if (f?.dir) {
      setFilePath(rel);
      setFiles(await client.listFiles(p.id, rel));
      return;
    }
    const got = await client.readFile(p.id, rel);
    if (got.binary) {
      setFileBody("(binary file)");
      setFileHash(got.hash);
      return;
    }
    setFilePath(rel);
    setFileBody(got.content);
    setFileHash(got.hash);
    setFileConflict("");
  }

  async function saveFile() {
    const p = project();
    if (!p) return;
    try {
      await client.writeFile(p.id, filePath(), fileBody(), fileHash());
      setFileConflict("");
    } catch (e) {
      setFileConflict(String(e));
    }
  }

  async function attachTerm() {
    const p = project();
    if (!p) return;
    const t = await client.startTerminal(p.id);
    setTermId(t.id);
    const ws = client.ptySocket(t.id);
    ws.binaryType = "arraybuffer";
    ws.onmessage = (ev) => {
      const text = typeof ev.data === "string" ? ev.data : new TextDecoder().decode(ev.data as ArrayBuffer);
      setTermLog((l) => l + text);
    };
    ws.onclose = () => {
      /* PTY stays alive server-side */
    };
    ws.onerror = () => setTermLost("Unable to attach. After a server restart the PTY is gone.");
  }

  async function loadOverlay(o: Overlay) {
    setOverlay(o);
    const r = run();
    const p = project();
    try {
      if (o === "details" && r) setDetail(await client.details(r.id));
      if (o === "routing" && r) setDetail(await client.routes(r.id));
      if (o === "context" && r) setDetail(await client.context(r.id));
      if (o === "usage" && r) setDetail(await client.usage(r.id));
      if (o === "artifacts" && r) setDetail(await client.artifacts(r.id));
      if (o === "knowledge" && p) setDetail(await client.knowledge(p.id));
      if (o === "recovery") setDetail(await client.sandbox());
      if (o === "settings") setDetail({ settings: await client.settings(), storage: await client.storage(), harnesses: await client.harnesses() });
    } catch (e) {
      setErr(String(e));
    }
  }

  const attention = () => notes().filter((n) => n.attention).length + approvals().filter((a) => a.status === "pending").length;

  return (
    <>
      <header>
        <strong>Wayshard</strong>
        <span class="muted">{connected() ? "connected" : "disconnected — stale"}</span>
        <span class="grow" />
        <button onClick={() => loadOverlay("approvals")}>Approvals {approvals().length || ""}</button>
        <button onClick={() => loadOverlay("notifications")}>Attention {attention() || ""}</button>
        <button onClick={() => loadOverlay("settings")}>Settings</button>
      </header>
      <Show when={err()}>
        <div class="banner">{err()}</div>
      </Show>
      <div class="layout">
        <aside>
          <div class="muted">Projects</div>
          <For each={projects()}>
            {(p) => (
              <div class="item" classList={{ active: project()?.id === p.id }} onClick={() => selectProject(p)}>
                {p.name}
                <div class="muted">
                  {p.status}
                  {p.status !== "available" ? " · Locate / Remove" : ""}
                </div>
              </div>
            )}
          </For>
          <div class="row" style="margin-top:12px">
            <input value={path()} onInput={(e) => setPath(e.currentTarget.value)} placeholder="Open folder on this server" />
            <button onClick={openProject}>Open</button>
          </div>
          <Show when={project()}>
            <div class="muted" style="margin-top:12px">
              Sessions
            </div>
            <For each={convs()}>
              {(c) => (
                <div class="item" classList={{ active: convId() === c.id }} onClick={() => selectConv(c.id)}>
                  {c.title || c.id.slice(0, 8)}
                </div>
              )}
            </For>
            <button onClick={newSession}>New session</button>
          </Show>
        </aside>
        <main>
          <div class="tabs">
            <For each={["session", "changes", "files", "terminal"] as const}>
              {(t) => (
                <button aria-current={tab() === t} onClick={() => setTab(t)}>
                  {t}
                </button>
              )}
            </For>
            <span class="grow" />
            <button onClick={() => loadOverlay("details")}>Run details</button>
          </div>

          <Show when={tab() === "session"}>
            <div class="timeline">
              <For each={messages()}>
                {(m) => (
                  <div class="msg">
                    <div class="muted">{m.role}</div>
                    <pre>{m.body}</pre>
                  </div>
                )}
              </For>
              <Show when={run()}>
                {(r) => (
                  <div>
                    <div class="muted">
                      {r().status}
                      {r().blockedReason ? ` · ${r().blockedReason}` : ""}
                      {r().degradedRouting ? " · degraded routing" : ""}
                    </div>
                    <For each={stages()}>
                      {(st) => (
                        <div class="stage" onClick={() => setExpanded({ ...expanded(), [st.id]: !expanded()[st.id] })}>
                          {stageLabel[st.kind] || st.kind} · {st.status}
                          <Show when={expanded()[st.id]}>
                            <pre class="muted">{JSON.stringify(st, null, 2)}</pre>
                          </Show>
                        </div>
                      )}
                    </For>
                    <Show when={arts().length}>
                      <For each={arts()}>
                        {(a) => (
                          <details>
                            <summary>{a.kind}</summary>
                            <pre>{a.json}</pre>
                          </details>
                        )}
                      </For>
                    </Show>
                    <div class="row">
                      <button onClick={() => client.cancel(r().id)}>Cancel</button>
                      <button onClick={() => client.retry(r().id)}>Retry</button>
                      <Show when={r().status === "ready_to_integrate" || r().status === "integration_blocked"}>
                        <button class="primary" onClick={() => client.integrate(r().id)}>
                          Integrate
                        </button>
                      </Show>
                    </div>
                  </div>
                )}
              </Show>
            </div>
            <div class="composer">
              <textarea value={text()} onInput={(e) => setText(e.currentTarget.value)} placeholder="Describe the work…" />
              <div class="row">
                <select value={profile()} onChange={(e) => setProfile(e.currentTarget.value)}>
                  <option value="auto">Routing: Auto</option>
                  <option value="balanced">Balanced</option>
                  <option value="quality">Quality</option>
                  <option value="economy">Economy</option>
                  <option value="speed">Speed</option>
                </select>
                <label class="muted">
                  <input type="checkbox" checked={artifactOnly()} onChange={(e) => setArtifactOnly(e.currentTarget.checked)} /> artifact-only
                </label>
                <button class="primary" onClick={send}>
                  Send
                </button>
              </div>
            </div>
          </Show>

          <Show when={tab() === "changes"}>
            <div class="timeline">
              <div class="tabs">
                <button aria-current={changeView() === "run"} onClick={() => setChangeView("run")}>
                  Run
                </button>
                <button aria-current={changeView() === "workspace"} onClick={() => setChangeView("workspace")}>
                  Workspace
                </button>
              </div>
              <p class="muted">
                Run is snapshot → final. Workspace is the current source tree, including your pre-existing edits.
              </p>
              <pre>{JSON.stringify(changes(), null, 2)}</pre>
            </div>
          </Show>

          <Show when={tab() === "files"}>
            <div class="timeline">
              <div class="row">
                <button
                  onClick={async () => {
                    const p = project();
                    if (!p) return;
                    setFilePath("");
                    setFiles(await client.listFiles(p.id, ""));
                  }}
                >
                  /
                </button>
                <span class="muted">{filePath() || "."}</span>
              </div>
              <For each={files()}>
                {(f) => (
                  <div class="item" onClick={() => openFile(f.name)}>
                    {f.dir ? "▸ " : ""}
                    {f.name}
                  </div>
                )}
              </For>
              <Show when={fileBody()}>
                <textarea value={fileBody()} onInput={(e) => setFileBody(e.currentTarget.value)} />
                <button onClick={saveFile}>Save</button>
                <Show when={fileConflict()}>
                  <div class="banner">Stale hash — reload or compare. {fileConflict()}</div>
                </Show>
              </Show>
            </div>
          </Show>

          <Show when={tab() === "terminal"}>
            <div class="timeline">
              <p class="muted">Source workspace PTY. Client refresh does not terminate it. Server restart reports loss honestly.</p>
              <button onClick={attachTerm}>New terminal</button>
              <Show when={termLost()}>
                <div class="banner">{termLost()}</div>
              </Show>
              <pre>{termLog()}</pre>
              <div class="muted">id {termId()}</div>
            </div>
          </Show>
        </main>
      </div>

      <Show when={overlay() !== "none"}>
        <div class="overlay" onClick={() => setOverlay("none")}>
          <div class="sheet" onClick={(e) => e.stopPropagation()}>
            <div class="row">
              <strong>{overlay()}</strong>
              <span class="grow" />
              <button onClick={() => setOverlay("none")}>Close</button>
            </div>
            <Show when={overlay() === "notifications"}>
              <For each={notes()}>
                {(n) => (
                  <div class="item">
                    <div>
                      {n.title} {n.attention ? "· attention" : ""}
                    </div>
                    <div class="muted">{n.body}</div>
                    <button onClick={() => client.readNotification(n.id)}>Mark read</button>
                  </div>
                )}
              </For>
            </Show>
            <Show when={overlay() === "approvals"}>
              <For each={approvals()}>
                {(a) => (
                  <div class="item">
                    <div>
                      {a.kind} · {a.resource}
                    </div>
                    <div class="muted">{a.reason}</div>
                    <button onClick={() => client.resolveApproval(a.id, "allowed")}>Allow</button>
                    <button onClick={() => client.resolveApproval(a.id, "denied")}>Deny</button>
                  </div>
                )}
              </For>
            </Show>
            <pre>{JSON.stringify(detail(), null, 2)}</pre>
            <div class="row">
              <button onClick={() => loadOverlay("usage")}>Usage</button>
              <button onClick={() => loadOverlay("routing")}>Routing</button>
              <button onClick={() => loadOverlay("context")}>Context</button>
              <button onClick={() => loadOverlay("artifacts")}>Artifacts</button>
              <button onClick={() => loadOverlay("knowledge")}>Knowledge</button>
              <button onClick={() => loadOverlay("recovery")}>Recovery</button>
            </div>
          </div>
        </div>
      </Show>
    </>
  );
}
