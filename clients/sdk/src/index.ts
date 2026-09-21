export type ID = string;

export interface Meta {
  product: string;
  version: string;
  commit: string;
  api: number;
  compatibility: number;
}

export interface Project {
  id: ID;
  name: string;
  path: string;
  sourceKind: string;
  status: string;
}

export interface Conversation {
  id: ID;
  projectId: ID;
  title: string;
}

export interface Run {
  id: ID;
  taskId: ID;
  projectId: ID;
  conversationId: ID;
  status: string;
  blockedReason?: string;
  blockedDetail?: string;
  profile: string;
  degradedRouting?: boolean;
}

export interface Stage {
  id: ID;
  runId: ID;
  kind: string;
  ordinal: number;
  status: string;
}

export interface Artifact {
  id: ID;
  kind: string;
  json: string;
  valid: boolean;
}

export interface Approval {
  id: ID;
  runId: ID;
  kind: string;
  resource: string;
  reason: string;
  status: string;
}

export interface Notification {
  id: ID;
  kind: string;
  title: string;
  body: string;
  attention: boolean;
  runId?: string;
  projectId?: string;
}

export interface EventFrame {
  type: string;
  seq?: number;
  channel?: string;
  payload?: unknown;
}


// ---------------------------------------------------------------------------
// Wayshard application-identity verification.
//
// Pairing binds to an expected server identity taken from the trusted pairing
// invitation (serverId + fingerprint), verifies possession of the identity
// private key via a signed fresh nonce, and only then persists a credential.
// This proves application identity; it does not provide transport security
// (TLS/VPN/tunnel remain the user's responsibility).
// ---------------------------------------------------------------------------

export interface PairingChallenge {
  serverId: string;
  fingerprint: string;
  publicKey: string;
  signature: string;
}

export interface ServerIdentityExpectation {
  serverId?: string;
  fingerprint?: string;
}

export interface IdentityVerification {
  ok: boolean;
  reason?: string;
  serverId?: string;
  fingerprint?: string;
}

export interface PairingInvitation {
  advertisedUrl?: string;
  listenUrl?: string;
  serverId?: string;
  fingerprint?: string;
  code?: string;
  expiresAt?: string;
}

function bytesToHex(b: Uint8Array): string {
  return Array.from(b)
    .map((x) => x.toString(16).padStart(2, "0"))
    .join("");
}

function hexToBytes(hex: string): Uint8Array | null {
  if (hex.length % 2 !== 0 || !/^[0-9a-fA-F]*$/.test(hex)) return null;
  const out = new Uint8Array(hex.length / 2);
  for (let i = 0; i < out.length; i++) out[i] = parseInt(hex.slice(i * 2, i * 2 + 2), 16);
  return out;
}

async function sha256Hex(bytes: Uint8Array): Promise<string> {
  const digest = await crypto.subtle.digest("SHA-256", bytes as unknown as ArrayBuffer);
  return bytesToHex(new Uint8Array(digest));
}

// randomNonce returns a fresh unpredictable nonce (hex) so a challenge response
// cannot be replayed.
export function randomNonce(bytes = 32): string {
  const b = new Uint8Array(bytes);
  crypto.getRandomValues(b);
  return bytesToHex(b);
}

// verifyServerIdentity checks that the challenge was signed by the private key
// for the expected application identity over the fresh nonce.
export async function verifyServerIdentity(
  nonce: string,
  challenge: PairingChallenge,
  expected: ServerIdentityExpectation,
): Promise<IdentityVerification> {
  const pub = hexToBytes(challenge.publicKey ?? "");
  if (!pub || pub.length !== 32) return { ok: false, reason: "malformed server public key" };
  const sig = hexToBytes(challenge.signature ?? "");
  if (!sig) return { ok: false, reason: "malformed signature" };
  const nonceBytes = hexToBytes(nonce);
  if (!nonceBytes) return { ok: false, reason: "malformed nonce" };

  const fingerprint = (await sha256Hex(pub)).slice(0, 16);
  if (challenge.fingerprint && challenge.fingerprint !== fingerprint) {
    return { ok: false, reason: "presented fingerprint does not match the presented public key" };
  }
  if (expected.fingerprint && expected.fingerprint !== fingerprint) {
    return { ok: false, reason: "server fingerprint does not match the trusted invitation" };
  }
  if (expected.serverId && expected.serverId !== challenge.serverId) {
    return { ok: false, reason: "server id does not match the trusted invitation" };
  }
  try {
    const key = await crypto.subtle.importKey("raw", pub as unknown as ArrayBuffer, { name: "Ed25519" }, false, ["verify"]);
    const valid = await crypto.subtle.verify({ name: "Ed25519" }, key, sig as unknown as ArrayBuffer, nonceBytes as unknown as ArrayBuffer);
    if (!valid) return { ok: false, reason: "signature is not valid for this nonce" };
  } catch (err) {
    return { ok: false, reason: `Ed25519 verification unavailable: ${String(err)}` };
  }
  return { ok: true, serverId: challenge.serverId, fingerprint };
}

// parseInvitation accepts the Wayshard pairing card text or its JSON form.
export function parseInvitation(text: string): PairingInvitation | null {
  const trimmed = text.trim();
  if (!trimmed) return null;
  if (trimmed.startsWith("{")) {
    try {
      const raw = JSON.parse(trimmed) as Record<string, unknown>;
      return {
        advertisedUrl: (raw.advertisedUrl ?? raw.url) as string | undefined,
        listenUrl: raw.listenUrl as string | undefined,
        serverId: raw.serverId as string | undefined,
        fingerprint: raw.fingerprint as string | undefined,
        code: raw.code as string | undefined,
        expiresAt: raw.expiresAt as string | undefined,
      };
    } catch {
      return null;
    }
  }
  const out: PairingInvitation = {};
  for (const line of trimmed.split(/\r?\n/)) {
    const m = line.match(/^\s*(Server|Fingerprint|URL|Listen|Code|Expires)\s*:\s*(.+?)\s*$/i);
    if (!m) continue;
    const value = m[2];
    switch (m[1].toLowerCase()) {
      case "server":
        out.serverId = value;
        break;
      case "fingerprint":
        out.fingerprint = value;
        break;
      case "url":
        out.advertisedUrl = value;
        break;
      case "listen":
        out.listenUrl = value;
        break;
      case "code":
        out.code = value;
        break;
      case "expires":
        out.expiresAt = value;
        break;
    }
  }
  if (!out.code) return null;
  return out;
}

export class WayshardClient {
  constructor(
    public baseUrl: string,
    public token?: string,
  ) {}

  private headers(): Record<string, string> {
    const h: Record<string, string> = { "content-type": "application/json" };
    if (this.token) h.authorization = `Bearer ${this.token}`;
    return h;
  }

  async get<T>(path: string): Promise<T> {
    const r = await fetch(this.baseUrl + path, { headers: this.headers() });
    if (!r.ok) throw new Error(`${r.status} ${await r.text()}`);
    return r.json() as Promise<T>;
  }

  async send<T>(method: string, path: string, body?: unknown, idempotencyKey?: string): Promise<T> {
    const headers = { ...this.headers() };
    if (idempotencyKey) headers["Idempotency-Key"] = idempotencyKey;
    const r = await fetch(this.baseUrl + path, {
      method,
      headers,
      body: body === undefined ? undefined : JSON.stringify(body),
    });
    if (!r.ok) throw new Error(`${r.status} ${await r.text()}`);
    if (r.status === 204) return undefined as T;
    return r.json() as Promise<T>;
  }

  post<T>(path: string, body?: unknown, key?: string) {
    return this.send<T>("POST", path, body, key);
  }
  put<T>(path: string, body: unknown) {
    return this.send<T>("PUT", path, body);
  }
  del<T>(path: string) {
    return this.send<T>("DELETE", path);
  }

  meta() {
    return this.get<Meta>("/v1/meta");
  }
  server() {
    return this.get<Record<string, unknown>>("/v1/server");
  }
  projects() {
    return this.get<Project[]>("/v1/projects");
  }
  openProject(path: string, name?: string) {
    return this.post<Project>("/v1/projects/open", { path, name });
  }
  createProject(path: string, name?: string, git?: boolean) {
    return this.post<Project>("/v1/projects/create", { path, name, git });
  }
  cloneProject(url: string, path: string) {
    return this.post<Project>("/v1/projects/clone", { url, path });
  }
  locateProject(id: string, path: string) {
    return this.post<Project>(`/v1/projects/${id}/locate`, { path });
  }
  removeProject(id: string) {
    return this.del<{ ok: boolean }>(`/v1/projects/${id}`);
  }
  conversations(projectId: string) {
    return this.get<Conversation[]>(`/v1/projects/${projectId}/conversations`);
  }
  createConversation(projectId: string, title = "") {
    return this.post<Conversation>(`/v1/projects/${projectId}/conversations`, { title });
  }
  messages(conversationId: string) {
    return this.get<Array<{ id: string; role: string; body: string }>>(`/v1/conversations/${conversationId}/messages`);
  }
  sendMessage(conversationId: string, text: string, opts?: { artifactOnly?: boolean; profile?: string; idempotencyKey?: string }) {
    return this.post<{ run: Run }>(
      `/v1/conversations/${conversationId}/messages`,
      { text, artifactOnly: opts?.artifactOnly, profile: opts?.profile },
      opts?.idempotencyKey,
    );
  }
  run(id: string) {
    return this.get<Run>(`/v1/runs/${id}`);
  }
  stages(runId: string) {
    return this.get<Stage[]>(`/v1/runs/${runId}/stages`);
  }
  artifacts(runId: string) {
    return this.get<Artifact[]>(`/v1/runs/${runId}/artifacts`);
  }
  usage(runId: string) {
    return this.get<unknown[]>(`/v1/runs/${runId}/usage`);
  }
  routes(runId: string) {
    return this.get<unknown[]>(`/v1/runs/${runId}/routes`);
  }
  details(runId: string) {
    return this.get<Record<string, unknown>>(`/v1/runs/${runId}/details`);
  }
  runChanges(runId: string) {
    return this.get<Record<string, unknown>>(`/v1/runs/${runId}/changes`);
  }
  runFile(runId: string, path: string, side: "run" | "snapshot" = "run") {
    return this.get<{ path: string; content: string; hash: string; binary: boolean; missing: boolean }>(
      `/v1/runs/${runId}/file?path=${encodeURIComponent(path)}&side=${side}`,
    );
  }
  workspaceChanges(projectId: string) {
    return this.get<Record<string, unknown>>(`/v1/projects/${projectId}/changes`);
  }
  knowledge(projectId: string) {
    return this.get<Record<string, unknown>>(`/v1/projects/${projectId}/knowledge`);
  }
  context(runId: string) {
    return this.get<Record<string, unknown>>(`/v1/runs/${runId}/context`);
  }
  cancel(runId: string) {
    return this.post(`/v1/runs/${runId}/cancel`);
  }
  retry(runId: string) {
    return this.post(`/v1/runs/${runId}/retry`);
  }
  integrate(runId: string) {
    return this.post(`/v1/runs/${runId}/integrate`);
  }
  listFiles(projectId: string, path = "") {
    return this.get<Array<{ name: string; dir: boolean }>>(`/v1/projects/${projectId}/files?path=${encodeURIComponent(path)}`);
  }
  readFile(projectId: string, path: string) {
    return this.get<{ path: string; content: string; hash: string; binary: boolean }>(
      `/v1/projects/${projectId}/file?path=${encodeURIComponent(path)}`,
    );
  }
  writeFile(projectId: string, path: string, content: string, expectedHash: string) {
    return this.put(`/v1/projects/${projectId}/file`, { path, content, expectedHash });
  }
  approvals() {
    return this.get<Approval[]>("/v1/approvals");
  }
  resolveApproval(id: string, status: "allowed" | "denied") {
    return this.post(`/v1/approvals/${id}/resolve`, { status });
  }
  notifications(unread = false) {
    return this.get<Notification[]>(`/v1/notifications${unread ? "?unread=1" : ""}`);
  }
  readNotification(id: string) {
    return this.post(`/v1/notifications/${id}/read`);
  }
  harnesses() {
    return this.get<unknown[]>("/v1/harnesses");
  }
  rescanHarnesses() {
    return this.post("/v1/harnesses/rescan");
  }
  settings(scope = "server", scopeId = "") {
    return this.get(`/v1/settings?scope=${scope}&scopeId=${encodeURIComponent(scopeId)}`);
  }
  putSetting(body: unknown) {
    return this.put("/v1/settings", body);
  }
  storage() {
    return this.get("/v1/storage");
  }
  gc() {
    return this.post("/v1/storage/gc");
  }
  backup(dest: string, includeSecrets = false) {
    return this.post("/v1/backups", { dest, includeSecrets });
  }
  sandbox() {
    return this.get("/v1/sandbox");
  }
  devices() {
    return this.get("/v1/devices");
  }
  revokeDevice(id: string) {
    return this.post(`/v1/devices/${id}/revoke`);
  }
  invite(advertisedUrl?: string) {
    return this.post("/v1/pairing/invitations", { advertisedUrl });
  }
  pair(code: string, deviceName: string, deviceKind: string, expected?: ServerIdentityExpectation) {
    return this.post<{ credential?: string; session?: string; deviceId?: string; serverId?: string; fingerprint?: string }>(
      "/v1/pairing/complete",
      {
        code,
        deviceName,
        deviceKind,
        expectedServerId: expected?.serverId,
        expectedFingerprint: expected?.fingerprint,
      },
    );
  }
  pairingChallenge(nonce: string) {
    return this.get<PairingChallenge>(`/v1/pairing/challenge?nonce=${encodeURIComponent(nonce)}`);
  }
  terminals(projectId: string) {
    return this.get(`/v1/projects/${projectId}/terminals`);
  }
  startTerminal(projectId: string) {
    return this.post<{ id: string }>(`/v1/projects/${projectId}/terminals`);
  }

  events(lastSeq = 0, projectId?: string, runId?: string): WebSocket {
    const u = new URL(this.baseUrl.replace(/^http/, "ws") + "/v1/ws");
    u.searchParams.set("lastEventSeq", String(lastSeq));
    if (projectId) u.searchParams.set("projectId", projectId);
    if (runId) u.searchParams.set("runId", runId);
    return new WebSocket(u);
  }

  ptySocket(id: string): WebSocket {
    const u = new URL(this.baseUrl.replace(/^http/, "ws") + "/v1/ws/pty");
    u.searchParams.set("id", id);
    return new WebSocket(u);
  }
}
