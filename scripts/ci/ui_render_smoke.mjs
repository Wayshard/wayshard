#!/usr/bin/env node
// Rendered smoke test for the production Wayshard graphical client.
//
// Drives the installed Chrome over the DevTools Protocol (no browser download,
// no npm dependency) against the real production Web build, with the Wayshard
// HTTP API and event socket mocked in-page. It asserts *rendered* behaviour of
// the adapted OpenCode-derived application: theme tokens resolve and the canvas
// is legible, the Home route renders projects/sessions, opening a session
// reaches the session composition with a composer, the command palette and the
// advanced-surfaces overflow work, the narrow layout stays usable, and the
// unauthenticated pairing gate is themed.
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

const MIME = { ".html": "text/html", ".js": "text/javascript", ".mjs": "text/javascript", ".css": "text/css", ".json": "application/json", ".svg": "image/svg+xml", ".woff2": "font/woff2", ".png": "image/png", ".ico": "image/x-icon" };
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

// Mock the Wayshard API + event socket in-page. No visual/CSS override.
const SEED = `(() => {
  try { if (location.search.includes("noauth")) localStorage.removeItem("wayshard.connection"); else localStorage.setItem("wayshard.connection", JSON.stringify({ baseUrl: location.origin, token: "smoke-token" })); } catch {}
  const project = { id: "prj_smoke", name: "wayshard", path: "/home/dev/wayshard", sourceKind: "git", status: "ready" };
  const conv = { id: "cnv_1", projectId: project.id, title: "Smoke session" };
  const messages = [
    { id: "m1", role: "user", body: "The CLI/TUI archive must launch without renaming the companion." },
    { id: "m2", role: "assistant", body: "Packaging and launcher resolution updated." },
  ];
  const J = (b) => Promise.resolve(new Response(JSON.stringify(b), { status: 200, headers: { "content-type": "application/json" } }));
  const route = (p) => {
    p = p.split("?")[0];
    if (p === "/v1/meta") return { product: "Wayshard", version: "0.0.0-smoke", commit: "smoke", api: 1, compatibility: 1 };
    if (p === "/v1/projects") return [project];
    if (p === "/v1/projects/" + project.id + "/conversations") return [conv];
    if (p === "/v1/conversations/" + conv.id + "/messages") return messages;
    if (p === "/v1/approvals") return [{ id: "a1", runId: "r1", kind: "tool_network", resource: "registry.npmjs.org:443", reason: "build dep", status: "pending" }];
    if (p === "/v1/notifications") return [{ id: "n1", kind: "run.complete", title: "Run complete", body: "integrated", attention: false }];
    if (p === "/v1/projects/" + project.id + "/files") return [{ name: "src", dir: true }, { name: "README.md", dir: false }];
    if (p === "/v1/projects/" + project.id + "/file") return { path: "README.md", content: "# Wayshard", hash: "h", binary: false };
    if (p === "/v1/projects/" + project.id + "/changes") return { files: { "a.ts": { kind: "modified" }, "b.ts": { kind: "added" } } };
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
    try { const u = new URL(url, location.origin); if (u.pathname.startsWith("/v1/")) return J(route(u.pathname + u.search)); } catch {}
    return real ? real(input, init) : Promise.reject(new Error("no fetch"));
  };
  class FakeWS { constructor() { this.readyState = 1; setTimeout(() => this.onopen && this.onopen({}), 20); } send() {} close() {} }
  window.WebSocket = FakeWS;
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
  await sleep(1000);
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
        const session = document.querySelector('[data-session-id]');
        return { homeVisible: vis(root), hasProject: !!project, hasSession: !!session, text: (root ? root.textContent : "").slice(0, 300) };
      })()`);
      check("home: route renders", home.homeVisible === true, JSON.stringify(home));
      check("home: project listed", home.hasProject === true);
      check("home: session listed", home.hasSession === true);
      await p.shot("home-1280.png");
      await p.close();
    }

    // ---- Home -> Session navigation + session composition ----
    {
      const p = await openPage(cdp, `${base}/`, { width: 1280, height: 900 }, SHOTS, "session-1280");
      await p.evaluate(`document.querySelector('[data-session-id]')?.click()`).catch(() => {});
      await sleep(1500);
      const nav = await p.evaluate(`({ path: location.pathname, sessionShell: !!document.querySelector(".wh-titlebar"), composer: !!document.querySelector(".wh-composer"), content: !!document.querySelector(".wh-session") })`);
      check("home->session: navigates", nav.sessionShell === true, JSON.stringify(nav));
      check("session: composer region present", nav.composer === true);
      check("session: content region present", nav.content === true);
      await p.shot("session-1280.png");
      await p.close();
    }

    // ---- Direct session route (SPA fallback) ----
    {
      const p = await openPage(cdp, `${base}/prj_smoke/session/cnv_1`, { width: 1280, height: 900 }, SHOTS, "session-direct-1280");
      const s = await p.evaluate(`(() => ({ shell: !!document.querySelector(".wh-titlebar"), composer: !!document.querySelector(".wh-composer"), home: !!document.querySelector('[data-component="home"]') }))()`);
      check("session route: renders session shell", s.shell === true && s.home === false, JSON.stringify(s));
      await p.close();
    }

    // ---- Command palette (session route) ----
    {
      const p = await openPage(cdp, `${base}/prj_smoke/session/cnv_1`, { width: 1280, height: 900 }, SHOTS, "palette");
      const pal = await p.evaluate(`(async () => { ${HELPERS}
        window.dispatchEvent(new KeyboardEvent("keydown", { key: "k", ctrlKey: true, bubbles: true, cancelable: true })); await sleep(600);
        const items = [...document.querySelectorAll(".wh-palette-item")];
        return { items: items.length, visible: items.filter(vis).length };
      })()`);
      check("palette: rows visible", pal.visible >= 3 && pal.visible === pal.items, JSON.stringify(pal));
      await p.shot("palette-1280.png");
      await p.close();
    }

    // ---- Advanced surfaces overflow (session route) ----
    {
      const p = await openPage(cdp, `${base}/prj_smoke/session/cnv_1`, { width: 1280, height: 900 }, SHOTS, "more");
      const more = await p.evaluate(`(async () => { ${HELPERS}
        const m = [...document.querySelectorAll(".wh-tab-button")].find(b => b.textContent.trim() === "More");
        m && m.click(); await sleep(500);
        const items = [...document.querySelectorAll(".wh-more-item")];
        return { dialog: vis(document.querySelector('[data-component="dialog"]')), items: items.length, visible: items.filter(vis).length, labels: items.map(i => i.textContent.trim()) };
      })()`);
      check("more: dialog visible", more.dialog === true, JSON.stringify(more));
      check("more: advanced items visible", more.visible === 10, `visible ${more.visible}/${more.items}`);
      await p.shot("more-1280.png");
      await p.close();
    }

    // ---- Mobile Home + session ----
    {
      const home = await openPage(cdp, `${base}/`, { width: 390, height: 844 }, SHOTS, "home-390");
      const g = await home.evaluate(`(() => { const r = (s) => { const e = document.querySelector(s); if (!e) return null; const b = e.getBoundingClientRect(); return { w: Math.round(b.width), right: Math.round(b.right) }; }; return { home: !!document.querySelector('[data-component="home"]'), overflow: document.documentElement.scrollWidth > window.innerWidth + 2, innerWidth: window.innerWidth, root: r('[data-component="home"]') }; })()`);
      check("mobile home: renders", g.home === true, JSON.stringify(g));
      check("mobile home: no horizontal overflow", g.overflow === false, JSON.stringify(g));
      await home.shot("home-390.png");
      await home.close();

      const sess = await openPage(cdp, `${base}/prj_smoke/session/cnv_1`, { width: 390, height: 844 }, SHOTS, "session-390");
      const sg = await sess.evaluate(`(() => ({ shell: !!document.querySelector(".wh-titlebar"), composer: !!document.querySelector(".wh-composer"), overflow: document.documentElement.scrollWidth > window.innerWidth + 2 }))()`);
      check("mobile session: renders", sg.shell === true && sg.composer === true, JSON.stringify(sg));
      check("mobile session: no horizontal overflow", sg.overflow === false, JSON.stringify(sg));
      await sess.shot("session-390.png");
      await sess.close();
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
