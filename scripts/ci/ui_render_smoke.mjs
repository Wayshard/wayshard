#!/usr/bin/env node
// Rendered smoke test for the production Wayshard graphical client.
//
// Drives the installed Chrome over the DevTools Protocol (no browser download,
// no npm dependency) against the real production Web build, with the Wayshard
// HTTP API and WebSocket boundary mocked in-page. It asserts *rendered*
// behaviour of the adapted OpenCode-derived application: theme tokens resolve
// and the canvas is legible, Home renders, navigation is in-place (proved with a
// per-document marker that survives route changes only when the document is not
// recreated), the New Session flow submits, and the remediated session surfaces
// (Changes/Review, Files, Terminal, Composer/Approval, Timeline) render and
// interact. The narrow layout is re-checked at 390px and desktop at 1280/900.
//
// Usage: node scripts/ci/ui_render_smoke.mjs
//   UI_SMOKE_DIST   directory of the built Web client (default clients/web/dist)
//   UI_SMOKE_SHOTS  optional directory to write PNG artifacts
//   CHROME_PATH     explicit Chrome/Chromium executable
//
// Exits 0 on success, 1 on assertion failure, 2 if no Chrome is available
// (skipped, not failed, so non-desktop CI legs can no-op).
import { createServer } from "node:http";
import { spawn, spawnSync } from "node:child_process";
import { existsSync, mkdtempSync } from "node:fs";
import { readFile, mkdir, writeFile, rm } from "node:fs/promises";
import { extname, join, resolve, normalize } from "node:path";
import { tmpdir } from "node:os";
import { fileURLToPath } from "node:url";

const ROOT = resolve(fileURLToPath(import.meta.url), "..", "..", "..");
const DIST = process.env.UI_SMOKE_DIST || join(ROOT, "clients", "web", "dist");
const SHOTS = process.env.UI_SMOKE_SHOTS || "";

const failures = [];
const notes = [];
function check(name, ok, detail) {
  if (ok) notes.push(`ok   ${name}`);
  else failures.push(`${name}${detail ? ` — ${detail}` : ""}`);
}

function which(bin) {
  const r = spawnSync("sh", ["-c", `command -v ${bin}`], { encoding: "utf8" });
  return r.status === 0 ? r.stdout.trim() : "";
}
function findChrome() {
  const explicit = process.env.CHROME_PATH;
  if (explicit) return existsSync(explicit) ? explicit : "";
  for (const c of ["google-chrome", "google-chrome-stable", "chromium", "chromium-browser"]) {
    const p = which(c);
    if (p) return p;
  }
  for (const p of ["/usr/bin/google-chrome", "/usr/bin/chromium"]) if (existsSync(p)) return p;
  return "";
}

const MIME = { ".html": "text/html", ".js": "text/javascript", ".mjs": "text/javascript", ".css": "text/css", ".json": "application/json", ".svg": "image/svg+xml", ".woff2": "font/woff2", ".png": "image/png", ".ico": "image/x-icon", ".wasm": "application/wasm" };
function startServer() {
  const server = createServer(async (req, res) => {
    try {
      const url = new URL(req.url, "http://localhost");
      const rel = normalize(decodeURIComponent(url.pathname)).replace(/^(\.\.[/\\])+/, "");
      const candidate = join(DIST, rel === "/" ? "index.html" : rel);
      let file = candidate;
      if (!candidate.startsWith(DIST)) { res.writeHead(403); return res.end(); }
      if (!existsSync(file)) {
        // SPA fallback: unknown extensionless routes serve the app shell.
        if (extname(rel)) { res.writeHead(404); return res.end("not found"); }
        file = join(DIST, "index.html");
      }
      const body = await readFile(file);
      res.writeHead(200, { "content-type": MIME[extname(file)] || "application/octet-stream" });
      res.end(body);
    } catch {
      res.writeHead(404); res.end("not found");
    }
  });
  return new Promise((r) => server.listen(0, "127.0.0.1", () => r(server)));
}

// Mock the Wayshard API + WebSocket boundary in-page. No visual/CSS override.
// The mock is stateful: approvals resolve, terminals are created/closed, file
// writes are captured, and stage status advances on demand.
const SEED = `(() => {
  try { if (location.search.includes("noauth")) localStorage.removeItem("wayshard.connection"); else localStorage.setItem("wayshard.connection", JSON.stringify({ baseUrl: location.origin, token: "smoke-token" })); } catch {}
  window.__wayshardDocId = (globalThis.crypto && crypto.randomUUID) ? crypto.randomUUID() : String(Math.random());
  const project = { id: "prj_smoke", name: "wayshard", path: "/home/dev/wayshard", sourceKind: "git", status: "ready" };
  const conv1 = { id: "cnv_1", projectId: project.id, title: "Smoke session" };
  const conv2 = { id: "cnv_new", projectId: project.id, title: "New session" };
  const messages = [];
  for (let i = 0; i < 60; i++) {
    messages.push({ id: "u" + i, role: "user", body: "Task " + i + ": keep the timeline usable with a long fixture." });
    messages.push({ id: "a" + i, role: "assistant", body: "Handled task " + i + "." });
  }
  const run = { id: "run_smoke", taskId: "task_1", projectId: project.id, conversationId: conv1.id, status: "running", profile: "auto" };
  const stages = () => [
    { id: "st_1", runId: run.id, kind: "plan", ordinal: 1, status: "complete" },
    { id: "st_2", runId: run.id, kind: "execute", ordinal: 2, status: state.advanceStages ? "complete" : "running" },
  ];
  const filesByPath = {
    "": [{ name: "src", dir: true }, { name: "README.md", dir: false }],
    "src": [{ name: "a.ts", dir: false }, { name: "b.ts", dir: false }, { name: "nested", dir: true }],
    "src/nested": [{ name: "deep.ts", dir: false }],
  };
  const runFile = (path, side) => {
    if (path === "src/a.ts") return { path, content: side === "snapshot" ? "export const a = 1\\n" : "export const a = 2\\n", hash: "h", binary: false, missing: false };
    if (path === "src/b.ts") return { path, content: side === "snapshot" ? "" : "brand new\\n", hash: "h", binary: false, missing: side === "snapshot" };
    if (path === "README.md") return { path, content: "", hash: "", binary: false, missing: true };
    return { path, content: "line\\n", hash: "h", binary: false, missing: false };
  };
  const state = { approvals: [{ id: "a1", runId: run.id, kind: "tool_network", resource: "registry.npmjs.org:443", reason: "build dep", status: "pending" }], terminals: [], calls: [], advanceStages: false };
  const J = (b) => Promise.resolve(new Response(JSON.stringify(b), { status: 200, headers: { "content-type": "application/json" } }));
  const route = (p, search, method, body) => {
    const q = new URLSearchParams(search);
    if (p === "/v1/meta") return { product: "Wayshard", version: "0.0.0-smoke", commit: "smoke", api: 1, compatibility: 1 };
    if (p === "/v1/projects") return [project];
    if (p === "/v1/projects/" + project.id + "/conversations") return method === "POST" ? conv2 : [conv1, conv2];
    if (p === "/v1/conversations/" + conv1.id + "/messages" || p === "/v1/conversations/" + conv2.id + "/messages") {
      return method === "POST" ? { run } : messages;
    }
    if (p === "/v1/runs/" + run.id) return run;
    if (p === "/v1/runs/" + run.id + "/stages") return stages();
    if (p === "/v1/runs/" + run.id + "/artifacts") return [];
    if (p === "/v1/runs/" + run.id + "/changes") return { kind: "run", delta: { files: [
      { path: "src/a.ts", kind: "modified", agentModified: true, preExisting: false },
      { path: "src/b.ts", kind: "added", agentModified: true, preExisting: true },
      { path: "README.md", kind: "deleted", agentModified: true },
    ] } };
    if (p === "/v1/runs/" + run.id + "/file") return runFile(q.get("path") || "", q.get("side") || "run");
    if (p === "/v1/projects/" + project.id + "/changes") return { kind: "workspace", files: { "src/a.ts": { kind: "modified" } } };
    if (p === "/v1/projects/" + project.id + "/files") return filesByPath[q.get("path") || ""] || [];
    if (p === "/v1/projects/" + project.id + "/file") {
      if (method === "PUT") return { ok: true };
      const path = q.get("path") || "";
      if (path.endsWith(".png")) return { path, content: "", hash: "b1", binary: true };
      return { path, content: "# Wayshard\\nline two\\n", hash: "h1", binary: false };
    }
    if (p === "/v1/projects/" + project.id + "/terminals") {
      if (method === "POST") { const id = "pty_" + (state.terminals.length + 1); state.terminals.push(id); return { id, projectId: project.id, alive: true }; }
      return state.terminals.map((id) => ({ id, projectId: project.id, alive: true }));
    }
    if (p.indexOf("/v1/projects/" + project.id + "/terminals/") === 0 && method === "DELETE") {
      const id = p.split("/").pop();
      state.terminals = state.terminals.filter((t) => t !== id);
      return { closed: true };
    }
    if (p === "/v1/approvals") return state.approvals;
    if (p === "/v1/approvals/a1/resolve") { state.approvals = []; return {}; }
    if (p === "/v1/notifications") return [{ id: "n1", kind: "run.complete", title: "Run complete", body: "integrated", attention: false }];
    if (p === "/v1/projects/" + project.id + "/knowledge") return { documents: [{ path: "AGENTS.md", kind: "instructions", authority: "implementation" }], conflicts: [] };
    if (p === "/v1/harnesses") return [{ definitionId: "opencode", executable: "/usr/bin/opencode", bridgeExecutable: "", acpStatus: "routable", health: "healthy", providerTransport: "http_proxy" }];
    if (p === "/v1/harness-definitions") return { definitions: [{ id: "opencode", source: "shipped", enabled: true, acp: "native", executables: ["opencode"], bridges: [] }], diagnostics: [] };
    if (p === "/v1/storage") return { databaseBytes: 1, objectsBytes: 2, workspacesBytes: 3, lowDisk: false };
    if (p === "/v1/sandbox") return { confinement: "landlock", network: "none", providerNetwork: { available: true }, loopback: true };
    return {};
  };
  const real = window.fetch ? window.fetch.bind(window) : null;
  window.fetch = (input, init) => {
    const url = typeof input === "string" ? input : (input && input.url) || "";
    const method = (init && init.method) || (input && input.method) || "GET";
    try {
      const u = new URL(url, location.origin);
      if (u.pathname.startsWith("/v1/")) {
        let body;
        try { body = init && init.body ? JSON.parse(init.body) : undefined; } catch {}
        state.calls.push({ method, path: u.pathname, search: u.search, body });
        return J(route(u.pathname, u.search, method, body));
      }
    } catch {}
    return real ? real(input, init) : Promise.reject(new Error("no fetch"));
  };
  class FakeWS {
    constructor(url) {
      this.url = String(url); this.readyState = 0; this.sent = [];
      FakeWS.all.push(this);
      setTimeout(() => { this.readyState = 1; if (this.onopen) this.onopen({}); }, 10);
    }
    send(d) { this.sent.push(d); }
    close() { this.readyState = 3; if (this.onclose) this.onclose({}); }
    emit(data) { if (this.onmessage) this.onmessage({ data }); }
  }
  FakeWS.all = [];
  window.WebSocket = FakeWS;
  window.__wayshardTest = state;
  window.__wayshardWS = FakeWS;
})();`;

class CDP {
  constructor(ws) { this.ws = ws; this.id = 0; this.pending = new Map(); this.handlers = new Map(); ws.onmessage = (e) => this.onMessage(e.data); }
  onMessage(data) {
    const msg = JSON.parse(data);
    if (msg.id && this.pending.has(msg.id)) {
      const { resolve, reject } = this.pending.get(msg.id); this.pending.delete(msg.id);
      msg.error ? reject(new Error(JSON.stringify(msg.error))) : resolve(msg.result);
    } else if (msg.method && this.handlers.has(msg.method)) {
      for (const h of this.handlers.get(msg.method)) h(msg.params, msg.sessionId);
    }
  }
  on(method, fn) { if (!this.handlers.has(method)) this.handlers.set(method, []); this.handlers.get(method).push(fn); }
  send(method, params = {}, sessionId) {
    const id = ++this.id;
    return new Promise((resolve, reject) => {
      this.pending.set(id, { resolve, reject });
      this.ws.send(JSON.stringify({ id, method, params, ...(sessionId ? { sessionId } : {}) }));
    });
  }
}

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

async function openPage(cdp, url, viewport, shots, name) {
  const { targetId } = await cdp.send("Target.createTarget", { url: "about:blank" });
  const { sessionId } = await cdp.send("Target.attachToTarget", { targetId, flatten: true });
  await cdp.send("Page.enable", {}, sessionId);
  await cdp.send("Runtime.enable", {}, sessionId);
  await cdp.send("Page.addScriptToEvaluateOnNewDocument", { source: SEED }, sessionId);
  await cdp.send("Emulation.setDeviceMetricsOverride", { width: viewport.width, height: viewport.height, deviceScaleFactor: 1, mobile: false }, sessionId);
  const loaded = new Promise((r) => cdp.on("Page.loadEventFired", (p, s) => { if (s === sessionId) r(); }));
  await cdp.send("Page.navigate", { url }, sessionId);
  await Promise.race([loaded, sleep(6000)]);
  await sleep(1200);
  return {
    sessionId,
    async evaluate(expression) {
      const r = await cdp.send("Runtime.evaluate", { expression, awaitPromise: true, returnByValue: true }, sessionId);
      if (r.exceptionDetails) throw new Error(r.exceptionDetails.exception?.description || "eval failed");
      return r.result.value;
    },
    async shot(file) {
      if (!shots) return;
      const r = await cdp.send("Page.captureScreenshot", { format: "png" }, sessionId);
      await writeFile(join(shots, file), Buffer.from(r.data, "base64"));
    },
    async close() { await cdp.send("Target.closeTarget", { targetId }).catch(() => {}); },
  };
}

const HELPERS = `
  const vis = (e) => { if (!e) return false; const b = e.getBoundingClientRect(); const s = getComputedStyle(e); return b.width > 0 && b.height > 0 && s.visibility !== "hidden" && s.display !== "none"; };
  const sleep = (ms) => new Promise(r => setTimeout(r, ms));
  const byText = (sel, t) => [...document.querySelectorAll(sel)].find(e => e.textContent.trim().toLowerCase().includes(t.toLowerCase()));
  const tab = (label) => [...document.querySelectorAll('[data-slot="titlebar-tab-item"]')].find(b => b.textContent.trim() === label);
  const clickTab = async (label) => { const t = tab(label); if (t) t.click(); await sleep(500); };
`;

async function main() {
  const chrome = findChrome();
  if (!chrome) { console.error("ui_render_smoke: no Chrome/Chromium found; skipping (set CHROME_PATH to run)"); process.exit(2); }
  if (!existsSync(join(DIST, "index.html"))) { console.error(`ui_render_smoke: no built client at ${DIST} (run the web build first)`); process.exit(2); }
  if (SHOTS) await mkdir(SHOTS, { recursive: true });

  const server = await startServer();
  const port = server.address().port;
  const base = `http://127.0.0.1:${port}`;
  const userDir = mkdtempSync(join(tmpdir(), "wayshard-ui-smoke-"));
  const proc = spawn(chrome, ["--headless=new", "--no-sandbox", "--disable-gpu", "--hide-scrollbars", "--remote-debugging-port=0", `--user-data-dir=${userDir}`, "about:blank"], { stdio: ["ignore", "ignore", "pipe"] });
  const wsUrl = await new Promise((resolve, reject) => {
    let buf = "";
    const t = setTimeout(() => reject(new Error("Chrome DevTools endpoint timeout")), 20000);
    proc.stderr.on("data", (d) => {
      buf += d.toString();
      const m = buf.match(/ws:\/\/[^\s]+/);
      if (m) { clearTimeout(t); resolve(m[0]); }
    });
    proc.on("exit", () => reject(new Error("Chrome exited early")));
  });
  const ws = new WebSocket(wsUrl);
  await new Promise((r, j) => { ws.onopen = r; ws.onerror = j; });
  const cdp = new CDP(ws);

  const themeExpr = `(() => { const v = n => getComputedStyle(document.documentElement).getPropertyValue(n).trim(); const cs = getComputedStyle(document.body); return { dataTheme: document.documentElement.getAttribute("data-theme"), varDeep: v("--v2-background-bg-deep"), varGrey: v("--v2-grey-1100"), bodyBg: cs.backgroundColor, bodyColor: cs.color, themeStyle: !!document.getElementById("oc-theme") }; })()`;
  const rgb = (s) => (s.match(/\d+/g) || []).slice(0, 3).map(Number);
  const lum = (c) => { const [r, g, b] = c.map((v) => { const s = v / 255; return s <= 0.03928 ? s / 12.92 : Math.pow((s + 0.055) / 1.055, 2.4); }); return 0.2126 * r + 0.7152 * g + 0.0722 * b; };
  const contrast = (a, b) => { const l1 = lum(rgb(a)), l2 = lum(rgb(b)); return Math.round((Math.max(l1, l2) + 0.05) / (Math.min(l1, l2) + 0.05) * 100) / 100; };

  try {
    // ---- Home route (desktop) ----
    {
      const p = await openPage(cdp, `${base}/`, { width: 1280, height: 900 }, SHOTS, "home-1280");
      const t = await p.evaluate(themeExpr);
      check("theme: data-theme oc-2", t.dataTheme === "oc-2", `got ${t.dataTheme}`);
      check("theme: --v2-background-bg-deep resolves", t.varDeep.length > 0, `got "${t.varDeep}"`);
      check("theme: theme style element present", t.themeStyle === true);
      check("theme: dark canvas", rgb(t.bodyBg)[0] < 60 && rgb(t.bodyBg)[1] < 60 && rgb(t.bodyBg)[2] < 60, t.bodyBg);
      check("theme: body contrast >= 4.5", contrast(t.bodyBg, t.bodyColor) >= 4.5, `ratio ${contrast(t.bodyBg, t.bodyColor)}`);
      const home = await p.evaluate(`(async () => { ${HELPERS}
        const root = document.querySelector('[data-component="home"]');
        const project = document.querySelector('[data-project-id]');
        const sessions = [...document.querySelectorAll('[data-session-id]')];
        return { homeVisible: vis(root), hasProject: !!project, sessions: sessions.length, text: (root ? root.textContent : "").slice(0, 300) };
      })()`);
      check("home: route renders", home.homeVisible === true, JSON.stringify(home));
      check("home: project listed", home.hasProject === true);
      check("home: sessions listed", home.sessions >= 2, `sessions ${home.sessions}`);
      await p.shot("home-1280.png");
      await p.close();
    }

    // ---- In-place routing with a document-reload detector ----
    {
      const p = await openPage(cdp, `${base}/`, { width: 1280, height: 900 }, SHOTS, "routing-inplace");
      const docId = await p.evaluate(`window.__wayshardDocId`);
      const first = await p.evaluate(`(async () => { ${HELPERS}
        const s = [...document.querySelectorAll('[data-session-id]')];
        s[0].click(); await sleep(1400);
        return { path: location.pathname, docId: window.__wayshardDocId,
          sessionRegion: !!document.querySelector('[data-slot="session-region"]'),
          home: !!document.querySelector('[data-component="home"]'),
          timeline: !!document.querySelector('[data-slot="message-timeline"]') };
      })()`);
      check("routing: session A renders in place without reload", first.sessionRegion === true && first.docId === docId, JSON.stringify(first));
      const back = await p.evaluate(`(async () => { ${HELPERS}
        history.back(); await sleep(1000);
        return { path: location.pathname, docId: window.__wayshardDocId, home: !!document.querySelector('[data-component="home"]') };
      })()`);
      check("routing: back returns to Home without reload", back.home === true && back.path === "/" && back.docId === docId, JSON.stringify(back));
      const second = await p.evaluate(`(async () => { ${HELPERS}
        const s = [...document.querySelectorAll('[data-session-id]')];
        s[s.length - 1].click(); await sleep(1200);
        return { path: location.pathname, docId: window.__wayshardDocId, shell: !!document.querySelector('[data-component="titlebar"]') };
      })()`);
      check("routing: session B navigates in place", second.shell === true && second.docId === docId, JSON.stringify(second));
      await p.shot("routing-inplace.png");
      await p.close();
    }

    // ---- Direct session route (SPA fallback) ----
    {
      const p = await openPage(cdp, `${base}/prj_smoke/session/cnv_1`, { width: 1280, height: 900 }, SHOTS, "session-direct-1280");
      const s = await p.evaluate(`(() => ({ shell: !!document.querySelector('[data-component="titlebar"]'), composer: !!document.querySelector(".wh-composer"), home: !!document.querySelector('[data-component="home"]'), timeline: !!document.querySelector('[data-slot="message-timeline"]') }))()`);
      check("session route: renders session shell", s.shell === true && s.home === false, JSON.stringify(s));
      check("session route: timeline renders", s.timeline === true);
      await p.close();
    }

    // ---- New Session flow + remediated session surfaces ----
    {
      const p = await openPage(cdp, `${base}/new-session`, { width: 1280, height: 900 }, SHOTS, "new-session-1280");
      const ns = await p.evaluate(`(async () => { ${HELPERS}
        const root = document.querySelector('[data-component="session-new-design"]');
        const composer = document.querySelector(".wh-composer");
        return { newDesign: vis(root), composer: !!composer };
      })()`);
      check("new-session: inherited composition renders", ns.newDesign === true && ns.composer === true, JSON.stringify(ns));

      // Type into the imported composer and submit.
      const submitted = await p.evaluate(`(async () => { ${HELPERS}
        const ed = document.querySelector('[data-component="composer"] [contenteditable="true"]') || document.querySelector('[data-component="composer"] textarea');
        if (ed) { ed.focus(); document.execCommand('insertText', false, 'Add a smoke task'); ed.dispatchEvent(new Event('input', { bubbles: true })); }
        await sleep(300);
        const send = [...document.querySelectorAll('.wh-composer button')].find(b => b.textContent.trim().toLowerCase().includes('send'));
        if (send) send.click();
        await sleep(1800);
        return { path: location.pathname, docId: window.__wayshardDocId,
          shell: !!document.querySelector('[data-component="titlebar"]'),
          newDesign: !!document.querySelector('[data-component="session-new-design"]'),
          sessionRegion: !!document.querySelector('[data-slot="session-region"]'),
          timeline: !!document.querySelector('[data-slot="message-timeline"]'),
          activeTabs: [...document.querySelectorAll('[data-slot="titlebar-tab-item"]')].filter(b => b.getAttribute('aria-selected') === 'true').map(b => b.textContent.trim()),
          calls: window.__wayshardTest.calls.filter(c => c.method === 'POST').map(c => c.path) };
      })()`);
      check("new-session: submit opens the new session route", submitted.path.startsWith("/prj_smoke/session/") && submitted.sessionRegion === true, JSON.stringify(submitted));
      check("new-session: conversation + message created", submitted.calls.includes("/v1/projects/prj_smoke/conversations") && submitted.calls.some((c) => c.endsWith("/messages")), JSON.stringify(submitted.calls));
      await p.shot("new-session-1280.png");

      // ---- Timeline: long fixture, bounded window, bottom-follow, preserve ----
      const tl = await p.evaluate(`(async () => { ${HELPERS}
        const scroller = document.querySelector('[data-slot="message-timeline"]');
        const rows = document.querySelectorAll('[data-timeline-row]');
        const atBottom = scroller ? (scroller.scrollHeight - scroller.scrollTop - scroller.clientHeight) <= 48 : false;
        const earlier = !!byText('button', 'Show earlier messages');
        return { rows: rows.length, atBottom, earlier, hasJump: !!byText('button', 'Jump to latest') };
      })()`);
      check("timeline: long fixture is bounded to a mounted window", tl.rows > 0 && tl.rows <= 60, JSON.stringify(tl));
      check("timeline: window offers earlier messages", tl.earlier === true, JSON.stringify(tl));
      check("timeline: bottom-follow keeps the end in view", tl.atBottom === true, JSON.stringify(tl));

      const preserve = await p.evaluate(`(async () => { ${HELPERS}
        const scroller = document.querySelector('[data-slot="message-timeline"]');
        if (!scroller) return { missing: true, before: -1, after: -1, jump: false };
        scroller.scrollTop = 0; scroller.dispatchEvent(new Event('scroll', { bubbles: true })); await sleep(200);
        const before = scroller.scrollTop;
        const ev = window.__wayshardWS.all.find(w => w.url.includes('/v1/ws') && !w.url.includes('/pty'));
        window.__wayshardTest.advanceStages = true;
        if (ev) ev.emit(JSON.stringify({ seq: 2, type: 'run.updated' }));
        await sleep(700);
        return { before, after: scroller.scrollTop, jump: !!byText('button', 'Jump to latest') };
      })()`);
      check("timeline: position preserved when scrolled away", Math.abs(preserve.after - preserve.before) <= 4, JSON.stringify(preserve));
      check("timeline: jump-to-latest appears when scrolled away", preserve.jump === true, JSON.stringify(preserve));

      const jumped = await p.evaluate(`(async () => { ${HELPERS}
        const jump = byText('button', 'Jump to latest'); if (jump) jump.click(); await sleep(300);
        const scroller = document.querySelector('[data-slot="message-timeline"]');
        return scroller ? (scroller.scrollHeight - scroller.scrollTop - scroller.clientHeight) <= 48 : false;
      })()`);
      check("timeline: jump-to-latest follows the end", jumped === true);
      await p.shot("timeline-1280.png");

      // ---- Changes / Review ----
      const changes = await p.evaluate(`(async () => { ${HELPERS}
        await clickTab('Changes');
        await sleep(1400);
        const review = document.querySelector('[data-component="session-review"]');
        const files = [...document.querySelectorAll('[data-slot="session-review-filename"]')].map(e => e.textContent);
        const hasRunToggle = !!byText('button', 'Run changes');
        const hasWorkspaceToggle = !!byText('button', 'Workspace changes');
        const surface = document.querySelector('[data-slot="session-review-tab"]');
        return { review: vis(review), files, hasRunToggle, hasWorkspaceToggle, text: surface ? surface.textContent.slice(0, 160) : null };
      })()`);
      check("changes: SessionReview is the production surface", changes.review === true, JSON.stringify(changes));
      check("changes: changed files listed through the inherited review", changes.files.length >= 2, JSON.stringify(changes.files));
      check("changes: Run/Workspace provenance controls", changes.hasRunToggle && changes.hasWorkspaceToggle, JSON.stringify(changes));

      const workspace = await p.evaluate(`(async () => { ${HELPERS}
        const ws = byText('button', 'Workspace changes'); if (ws) ws.click(); await sleep(700);
        const entries = [...document.querySelectorAll('[data-slot="session-review-workspace"] [data-kind]')];
        return { entries: entries.length };
      })()`);
      check("changes: workspace state is visible", workspace.entries >= 1, JSON.stringify(workspace));
      await p.shot("changes-1280.png");

      // ---- Files ----
      const files = await p.evaluate(`(async () => { ${HELPERS}
        await clickTab('Files');
        const tree = document.querySelector('[data-slot="session-file-tabs"]');
        const hasAll = !!byText('button', 'All');
        const hasChanges = !!byText('button', 'Changes');
        return { tree: vis(tree), hasAll, hasChanges };
      })()`);
      check("files: file tabs surface renders", files.tree === true && files.hasAll && files.hasChanges, JSON.stringify(files));

      const opened = await p.evaluate(`(async () => { ${HELPERS}
        const all = byText('button', 'All'); if (all) all.click(); await sleep(600);
        const src = byText('[data-slot="session-file-tabs"] button', 'src'); if (src) src.click(); await sleep(300);
        const nested = byText('[data-slot="session-file-tabs"] button', 'nested'); if (nested) nested.click(); await sleep(300);
        const deep = byText('[data-slot="session-file-tabs"] button', 'deep.ts'); if (deep) deep.click(); await sleep(700);
        const tabs = [...document.querySelectorAll('[data-slot="file-open-tabs"] button')].map(b => b.textContent.trim());
        const editor = !!document.querySelector('[data-slot="file-editor"]') || !!document.querySelector('[data-slot="session-file-tabs"] [data-component="file"]');
        return { tabs, editor };
      })()`);
      check("files: nested file opens a tab and renders", opened.tabs.some((t) => t.includes("deep.ts")), JSON.stringify(opened));

      const saved = await p.evaluate(`(async () => { ${HELPERS}
        const edit = byText('button', 'Edit'); if (edit) edit.click(); await sleep(300);
        const ta = document.querySelector('[data-slot="file-editor"]');
        if (ta) { ta.value = '# edited\\n'; ta.dispatchEvent(new Event('input', { bubbles: true })); }
        await sleep(200);
        const save = byText('button', 'Save'); if (save) save.click(); await sleep(700);
        const put = window.__wayshardTest.calls.filter(c => c.method === 'PUT' && c.path === '/v1/projects/prj_smoke/file');
        return { put: put.length, expectedHash: put.length ? put[put.length - 1].body && put[put.length - 1].body.expectedHash : null };
      })()`);
      check("files: save issues a compare-and-set write with expectedHash", saved.put >= 1 && typeof saved.expectedHash === "string", JSON.stringify(saved));
      await p.shot("files-1280.png");

      // ---- Terminal ----
      const term = await p.evaluate(`(async () => { ${HELPERS}
        await clickTab('Terminal');
        await sleep(900);
        const strip = document.querySelector('[data-slot="terminal-tab-strip"]');
        const tabs = [...document.querySelectorAll('[data-slot="terminal-tab"]')];
        const host = document.querySelector('[data-component="terminal"]');
        const created = window.__wayshardTest.calls.filter(c => c.method === 'POST' && c.path === '/v1/projects/prj_smoke/terminals').length;
        return { strip: vis(strip), tabs: tabs.length, host: !!host, created };
      })()`);
      check("terminal: panel tab strip renders", term.strip === true, JSON.stringify(term));
      check("terminal: a server-owned PTY is created", term.created >= 1 && term.tabs >= 1, JSON.stringify(term));
      check("terminal: ghostty host mounts", term.host === true, JSON.stringify(term));

      const pty = await p.evaluate(`(async () => { ${HELPERS}
        const sock = window.__wayshardWS.all.find(w => w.url.includes('/v1/ws/pty'));
        if (sock) { sock.emit('hello from pty\\n'); }
        await sleep(200);
        window.dispatchEvent(new Event('resize'));
        await sleep(300);
        const resize = sock ? sock.sent.filter(d => typeof d === 'string' && d.includes('wayshard')) : [];
        const lost = !!document.querySelector('[data-slot="terminal-lost"]');
        if (sock) sock.close();
        await sleep(300);
        const lostAfter = !!document.querySelector('[data-slot="terminal-lost"]');
        return { socket: !!sock, resize: resize.length, lost, lostAfter };
      })()`);
      check("terminal: renderer receives PTY data over the Wayshard transport", pty.socket === true, JSON.stringify(pty));
      check("terminal: loss/recovery state is visible", pty.lostAfter === true, JSON.stringify(pty));

      const closed = await p.evaluate(`(async () => { ${HELPERS}
        const before = document.querySelectorAll('[data-slot="terminal-tab"]').length;
        const close = document.querySelector('[data-slot="terminal-tab-close"]'); if (close) close.click();
        await sleep(600);
        const after = document.querySelectorAll('[data-slot="terminal-tab"]').length;
        const deletes = window.__wayshardTest.calls.filter(c => c.method === 'DELETE' && c.path.includes('/terminals/')).length;
        return { before, after, deletes };
      })()`);
      check("terminal: close removes the tab and closes the PTY", closed.after < closed.before && closed.deletes >= 1, JSON.stringify(closed));
      await p.shot("terminal-1280.png");

      // ---- Composer / Approval dock ----
      const composer = await p.evaluate(`(async () => { ${HELPERS}
        await clickTab('Session');
        const dock = document.querySelector('[data-slot="approval-dock"]');
        const allow = dock ? [...dock.querySelectorAll('button')].find(b => b.textContent.trim().toLowerCase().includes('allow')) : null;
        const before = window.__wayshardTest.calls.filter(c => c.path === '/v1/approvals/a1/resolve').length;
        if (allow) allow.click();
        await sleep(800);
        const resolved = window.__wayshardTest.calls.filter(c => c.path === '/v1/approvals/a1/resolve').length;
        const dockAfter = !!document.querySelector('[data-slot="approval-dock"]');
        const prompt = document.querySelector('[data-component="session-prompt-dock"]');
        return { dock: !!dock, resolved: resolved > before, dockAfter, prompt: !!prompt };
      })()`);
      check("composer: approval dock renders the server-owned approval", composer.dock === true, JSON.stringify(composer));
      check("composer: Allow resolves the approval once", composer.resolved === true, JSON.stringify(composer));
      check("composer: dock clears after the decision", composer.dockAfter === false, JSON.stringify(composer));
      check("composer: prompt dock remains mounted", composer.prompt === true, JSON.stringify(composer));
      await p.shot("composer-1280.png");
      await p.close();
    }

    // ---- Command palette + advanced surfaces (session route) ----
    {
      const p = await openPage(cdp, `${base}/prj_smoke/session/cnv_1`, { width: 1280, height: 900 }, SHOTS, "palette");
      const pal = await p.evaluate(`(async () => { ${HELPERS}
        window.dispatchEvent(new KeyboardEvent("keydown", { key: "k", ctrlKey: true, bubbles: true, cancelable: true })); await sleep(600);
        const items = [...document.querySelectorAll(".wh-palette-item")];
        return { items: items.length, visible: items.filter(vis).length };
      })()`);
      check("palette: rows visible", pal.visible >= 3 && pal.visible === pal.items, JSON.stringify(pal));
      await p.close();

      const more = await openPage(cdp, `${base}/prj_smoke/session/cnv_1`, { width: 1280, height: 900 }, SHOTS, "more");
      const m = await more.evaluate(`(async () => { ${HELPERS}
        const btn = tab('More'); if (btn) btn.click(); await sleep(500);
        const items = [...document.querySelectorAll(".wh-more-item")];
        return { dialog: vis(document.querySelector('[data-component="dialog"]')), items: items.length, visible: items.filter(vis).length };
      })()`);
      check("more: dialog visible", m.dialog === true, JSON.stringify(m));
      check("more: advanced items visible", m.visible === 10, `visible ${m.visible}/${m.items}`);
      await more.close();
    }

    // ---- Intermediate desktop width (900) ----
    {
      const p = await openPage(cdp, `${base}/prj_smoke/session/cnv_1`, { width: 900, height: 900 }, SHOTS, "session-900");
      const s = await p.evaluate(`(async () => { ${HELPERS}
        const shell = !!document.querySelector('[data-component="titlebar"]');
        const composer = !!document.querySelector(".wh-composer");
        await clickTab('Files');
        const files = !!document.querySelector('[data-slot="session-file-tabs"]');
        return { shell, composer, files, overflow: document.documentElement.scrollWidth > window.innerWidth + 2 };
      })()`);
      check("desktop 900: session + composer + files render", s.shell && s.composer && s.files, JSON.stringify(s));
      check("desktop 900: no horizontal overflow", s.overflow === false, JSON.stringify(s));
      await p.shot("session-900.png");
      await p.close();
    }

    // ---- Mobile (390) ----
    {
      const home = await openPage(cdp, `${base}/`, { width: 390, height: 844 }, SHOTS, "home-390");
      const g = await home.evaluate(`(() => { const r = (s) => { const e = document.querySelector(s); if (!e) return null; const b = e.getBoundingClientRect(); return { w: Math.round(b.width), right: Math.round(b.right) }; }; return { home: !!document.querySelector('[data-component="home"]'), overflow: document.documentElement.scrollWidth > window.innerWidth + 2, innerWidth: window.innerWidth, root: r('[data-component="home"]') }; })()`);
      check("mobile home: renders", g.home === true, JSON.stringify(g));
      check("mobile home: no horizontal overflow", g.overflow === false, JSON.stringify(g));
      await home.shot("home-390.png");
      await home.close();

      const sess = await openPage(cdp, `${base}/new-session`, { width: 390, height: 844 }, SHOTS, "new-session-390");
      const sg = await sess.evaluate(`(async () => { ${HELPERS}
        const ns = !!document.querySelector('[data-component="session-new-design"]');
        const composer = !!document.querySelector(".wh-composer");
        return { ns, composer, overflow: document.documentElement.scrollWidth > window.innerWidth + 2 };
      })()`);
      check("mobile new-session: renders", sg.ns === true && sg.composer === true, JSON.stringify(sg));
      check("mobile new-session: no horizontal overflow", sg.overflow === false, JSON.stringify(sg));
      await sess.shot("new-session-390.png");
      await sess.close();

      const nav = await openPage(cdp, `${base}/prj_smoke/session/cnv_1`, { width: 390, height: 844 }, SHOTS, "mobile-nav-390");
      const before = await nav.evaluate(`(() => { const e = document.querySelector('[data-component="sidebar-nav-mobile"]'); if (!e) return null; const b = e.getBoundingClientRect(); return { x: Math.round(b.x), w: Math.round(b.width) }; })()`);
      await nav.evaluate(`(async () => { const t = document.querySelector('[data-component="titlebar"] button[aria-label="Toggle navigation"]'); if (t) t.click(); await new Promise((r) => setTimeout(r, 400)); })()`);
      const open = await nav.evaluate(`(() => { const e = document.querySelector('[data-component="sidebar-nav-mobile"]'); const b = e ? e.getBoundingClientRect() : null; return { x: b ? Math.round(b.x) : null, w: b ? Math.round(b.width) : 0, scrim: !!document.querySelector('[data-component="sidebar-mobile-scrim"]') }; })()`);
      await nav.shot("mobile-nav-390.png");
      await nav.evaluate(`document.querySelector('[data-component="sidebar-nav-mobile"] [data-session-id]')?.click()`).catch(() => {});
      await sleep(1500);
      const after = await nav.evaluate(`(() => { const e = document.querySelector('[data-component="sidebar-nav-mobile"]'); if (!e) return null; const b = e.getBoundingClientRect(); return { x: Math.round(b.x), w: Math.round(b.width) }; })()`);
      check("mobile nav: overlay off-canvas when closed", before && before.x < 0, JSON.stringify(before));
      check("mobile nav: toggle opens overlay", open && open.x >= 0 && open.w > 0, JSON.stringify(open));
      check("mobile nav: selecting a session closes overlay", after && after.x < 0, JSON.stringify(after));
      await nav.close();

      const surfaces = await openPage(cdp, `${base}/prj_smoke/session/cnv_1`, { width: 390, height: 844 }, SHOTS, "mobile-surfaces-390");
      const sm = await surfaces.evaluate(`(async () => { ${HELPERS}
        await clickTab('Changes');
        const review = !!document.querySelector('[data-slot="session-review-tab"]');
        await clickTab('Files');
        const files = !!document.querySelector('[data-slot="session-file-tabs"]');
        await clickTab('Terminal');
        const terminal = !!document.querySelector('[data-slot="session-terminal-panel"]');
        await clickTab('Session');
        const composer = !!document.querySelector('.wh-composer');
        return { review, files, terminal, composer, overflow: document.documentElement.scrollWidth > window.innerWidth + 2 };
      })()`);
      check("mobile surfaces: Changes/Files/Terminal/Composer render", sm.review && sm.files && sm.terminal && sm.composer, JSON.stringify(sm));
      check("mobile surfaces: no horizontal overflow", sm.overflow === false, JSON.stringify(sm));
      await surfaces.shot("mobile-surfaces-390.png");
      await surfaces.close();
    }

    // ---- Pairing gate (unauthenticated) ----
    {
      const p = await openPage(cdp, `${base}/?noauth=1`, { width: 390, height: 844 }, SHOTS, "pairing-390");
      const gate = await p.evaluate(`(async () => { ${HELPERS} await sleep(300); const v = (n) => getComputedStyle(document.documentElement).getPropertyValue(n).trim(); return { visible: !!document.querySelector(".wh-pairing"), theme: document.documentElement.getAttribute("data-theme"), varDeep: v("--v2-background-bg-deep"), hasVerify: !!document.querySelector(".wh-pairing button") }; })()`);
      check("pairing: gate visible", gate.visible === true, JSON.stringify(gate));
      check("pairing: themed before auth", gate.theme === "oc-2" && gate.varDeep.length > 0, JSON.stringify(gate));
      await p.shot("pairing-390.png");
      await p.close();
    }
  } finally {
    try { ws.close(); } catch {}
    proc.kill("SIGKILL");
    server.close();
    await rm(userDir, { recursive: true, force: true }).catch(() => {});
  }

  for (const n of notes) console.log(n);
  if (failures.length) {
    console.error(`\nui_render_smoke FAILED (${failures.length}):`);
    for (const f of failures) console.error(`  FAIL ${f}`);
    process.exit(1);
  }
  console.log(`\nui_render_smoke ok (${notes.length} checks)`);
}

main().catch((e) => { console.error("ui_render_smoke error:", e); process.exit(1); });
