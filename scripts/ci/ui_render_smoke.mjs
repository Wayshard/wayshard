#!/usr/bin/env node
// Rendered smoke test for the production Wayshard graphical client.
//
// Drives the installed Chrome over the DevTools Protocol (no browser download,
// no npm dependency) against the real production Web build, with the Wayshard
// HTTP API and event socket mocked in-page. It asserts *rendered* behaviour:
// theme tokens resolve, the app canvas is legible, the "More" overflow control
// opens visible advanced surfaces, the command palette shows visible rows, and
// the narrow/mobile shell is a usable non-overlapping drawer.
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
      const file = join(DIST, rel === "/" ? "index.html" : rel);
      if (!file.startsWith(DIST)) { res.writeHead(403); return res.end(); }
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
  await sleep(900);
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

  try {
    // ---- Theme: normal production app, no overrides ----
    {
      const p = await openPage(cdp, `${base}/index.html`, { width: 1280, height: 900 }, SHOTS, "session-1280");
      const t = await p.evaluate(`(() => { const v = n => getComputedStyle(document.documentElement).getPropertyValue(n).trim(); const cs = getComputedStyle(document.body); return { dataTheme: document.documentElement.getAttribute("data-theme"), colorScheme: getComputedStyle(document.documentElement).colorScheme, varDeep: v("--v2-background-bg-deep"), varGrey: v("--v2-grey-1100"), bodyBg: cs.backgroundColor, bodyColor: cs.color, themeStyle: !!document.getElementById("oc-theme") }; })()`);
      const rgb = (s) => (s.match(/\d+/g) || []).slice(0, 3).map(Number);
      const lum = (c) => { const [r, g, b] = c.map((v) => { const s = v / 255; return s <= 0.03928 ? s / 12.92 : Math.pow((s + 0.055) / 1.055, 2.4); }); return 0.2126 * r + 0.7152 * g + 0.0722 * b; };
      const L1 = lum(rgb(t.bodyBg)), L2 = lum(rgb(t.bodyColor));
      const ratio = Math.round((Math.max(L1, L2) + 0.05) / (Math.min(L1, L2) + 0.05) * 100) / 100;
      check("theme: data-theme set", t.dataTheme === "oc-2", `got ${t.dataTheme}`);
      check("theme: --v2-background-bg-deep resolves", t.varDeep.length > 0 && t.varDeep !== "initial", `got "${t.varDeep}"`);
      check("theme: --v2-grey-1100 resolves", t.varGrey.length > 0, `got "${t.varGrey}"`);
      check("theme: theme style element present", t.themeStyle === true);
      check("theme: dark canvas", rgb(t.bodyBg)[0] < 60 && rgb(t.bodyBg)[1] < 60 && rgb(t.bodyBg)[2] < 60, t.bodyBg);
      check("theme: body contrast >= 4.5", ratio >= 4.5, `ratio ${ratio}`);
      await p.shot("session-1280.png");
      await p.close();
    }

    // ---- More overflow control (rendered, not state) ----
    {
      const p = await openPage(cdp, `${base}/index.html`, { width: 1280, height: 900 }, SHOTS, "more");
      const more = await p.evaluate(`(async () => { ${HELPERS}
        const m = [...document.querySelectorAll(".wh-tab-button")].find(b => b.textContent.trim() === "More");
        m && m.click(); await sleep(500);
        const items = [...document.querySelectorAll(".wh-more-item")];
        const grid = document.querySelector(".wh-more-grid");
        const dlg = document.querySelector('[data-component="dialog"]');
        const before = { gridVisible: vis(grid), dialogVisible: vis(dlg), items: items.length, itemsVisible: items.filter(vis).length, labels: items.map(i => i.textContent.trim()) };
        const usage = byText(".wh-more-item", "Usage"); usage && usage.click(); await sleep(500);
        const surface = { dialogVisible: vis(document.querySelector('[data-component="dialog"]')), hasSurface: !!document.querySelector(".wh-table, .wh-panel, .wh-structured") };
        window.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true })); await sleep(300);
        const closed = !document.querySelector('[data-component="dialog"]');
        m && m.click(); await sleep(400);
        const reopened = !!document.querySelector(".wh-more-grid");
        return { before, surface, closed, reopened };
      })()`);
      check("more: dialog visible", more.before.dialogVisible, JSON.stringify(more.before));
      check("more: grid visible", more.before.gridVisible);
      check("more: all 10 advanced items visible", more.before.itemsVisible === 10, `visible ${more.before.itemsVisible}/${more.before.items}`);
      check("more: selecting opens surface", more.surface.dialogVisible && more.surface.hasSurface, JSON.stringify(more.surface));
      check("more: Escape closes", more.closed === true);
      check("more: reopen works", more.reopened === true);
      await p.shot("more-1280.png");
      await p.close();
    }

    // ---- Command palette (visible rows + keyboard + filter) ----
    {
      const p = await openPage(cdp, `${base}/index.html`, { width: 1280, height: 900 }, SHOTS, "palette");
      const pal = await p.evaluate(`(async () => { ${HELPERS}
        window.dispatchEvent(new KeyboardEvent("keydown", { key: "k", ctrlKey: true, bubbles: true, cancelable: true })); await sleep(500);
        const items = [...document.querySelectorAll(".wh-palette-item")];
        const initial = { items: items.length, visible: items.filter(vis).length, active: items.filter(i => i.getAttribute("data-active") === "true").length };
        window.dispatchEvent(new KeyboardEvent("keydown", { key: "ArrowDown", bubbles: true })); await sleep(150);
        window.dispatchEvent(new KeyboardEvent("keydown", { key: "Enter", bubbles: true })); await sleep(400);
        const entered = vis(document.querySelector('[data-component="dialog"]'));
        window.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true })); await sleep(300);
        window.dispatchEvent(new KeyboardEvent("keydown", { key: "k", ctrlKey: true, bubbles: true, cancelable: true })); await sleep(400);
        const input = document.querySelector(".wh-palette input");
        if (input) { input.value = "files"; input.dispatchEvent(new Event("input", { bubbles: true })); }
        await sleep(300);
        const filtered = [...document.querySelectorAll(".wh-palette-title")].map(e => e.textContent.trim());
        return { initial, entered, filtered };
      })()`);
      check("palette: rows visible", pal.initial.visible >= 5 && pal.initial.visible === pal.initial.items, JSON.stringify(pal.initial));
      check("palette: active row present", pal.initial.active === 1);
      check("palette: Enter opens a dialog", pal.entered === true);
      check("palette: filter narrows", pal.filtered.some((t) => /files/i.test(t)), JSON.stringify(pal.filtered));
      await p.shot("palette-1280.png");
      await p.close();
    }

    // ---- Narrow / mobile shell geometry ----
    for (const vp of [{ w: 900, h: 900, mobile: false }, { w: 820, h: 900, mobile: true }, { w: 390, h: 844, mobile: true }]) {
      const p = await openPage(cdp, `${base}/index.html`, { width: vp.w, height: vp.h }, SHOTS, `session-${vp.w}`);
      const g = await p.evaluate(`(async () => { ${HELPERS}
        const r = (s) => { const e = document.querySelector(s); if (!e) return null; const b = e.getBoundingClientRect(); return { x: Math.round(b.x), right: Math.round(b.right), w: Math.round(b.width) }; };
        const toggle = document.querySelector(".wh-sidebar-toggle");
        const toggleVisible = vis(toggle);
        const tabs = document.querySelector(".wh-tabs").getBoundingClientRect();
        const tgl = toggle ? toggle.getBoundingClientRect() : null;
        const closed = { sidebar: r(".wh-sidebar"), content: r(".wh-content"), toggleVisible, toggleTabsOverlap: !!(tgl && tgl.right > tabs.x + 2), overflow: document.documentElement.scrollWidth > window.innerWidth + 2, innerWidth: window.innerWidth };
        let opened = null;
        if (toggleVisible) {
          toggle.click(); await sleep(500);
          const sb = document.querySelector(".wh-sidebar").getBoundingClientRect();
          const bd = document.querySelector(".wh-backdrop");
          opened = { sidebarX: Math.round(sb.x), sidebarW: Math.round(sb.width), backdrop: vis(bd) };
          const nav = document.querySelector(".wh-sidebar .wh-nav-item");
          nav && nav.click(); await sleep(500);
          opened.closedAfterSelect = document.querySelector(".wh-sidebar").getBoundingClientRect().right <= 0;
        }
        return { closed, opened };
      })()`);
      const label = `viewport ${vp.w}`;
      check(`${label}: no horizontal overflow`, g.closed.overflow === false, JSON.stringify(g.closed));
      check(`${label}: content has meaningful width`, g.closed.content.w > vp.w * 0.6, JSON.stringify(g.closed.content));
      if (vp.mobile) {
        check(`${label}: sidebar off-canvas when closed`, g.closed.sidebar.right <= 0, JSON.stringify(g.closed.sidebar));
        check(`${label}: nav toggle visible`, g.closed.toggleVisible === true);
        check(`${label}: toggle does not overlap tabs`, g.closed.toggleTabsOverlap === false);
        check(`${label}: opening shows drawer`, g.opened && g.opened.sidebarX >= 0 && g.opened.sidebarW > 0, JSON.stringify(g.opened));
        check(`${label}: backdrop visible when open`, g.opened && g.opened.backdrop === true);
        check(`${label}: selecting closes drawer`, g.opened && g.opened.closedAfterSelect === true);
      } else {
        check(`${label}: desktop sidebar in flow`, g.closed.sidebar && g.closed.sidebar.w >= 200 && g.closed.sidebar.x >= 0, JSON.stringify(g.closed.sidebar));
        check(`${label}: no nav toggle on desktop`, g.closed.toggleVisible === false);
      }
      await p.shot(`session-${vp.w}.png`);
      await p.close();
    }

    // ---- Pairing gate (unauthenticated) ----
    {
      const p = await openPage(cdp, `${base}/index.html?noauth=1`, { width: 390, height: 844 }, SHOTS, "pairing-390");
      const gate = await p.evaluate(`(async () => { ${HELPERS}
        await sleep(300);
        const v = (n) => getComputedStyle(document.documentElement).getPropertyValue(n).trim();
        return { visible: !!document.querySelector(".wh-pairing"), title: (document.querySelector(".wh-pairing-title") || {}).textContent || "", hasVerify: !!document.querySelector(".wh-pairing button"), theme: document.documentElement.getAttribute("data-theme"), varDeep: v("--v2-background-bg-deep") };
      })()`);
      check("pairing: gate visible", gate.visible === true, JSON.stringify(gate));
      check("pairing: themed before auth", gate.theme === "oc-2" && gate.varDeep.length > 0, JSON.stringify(gate));
      check("pairing: verify action present", gate.hasVerify === true);
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
