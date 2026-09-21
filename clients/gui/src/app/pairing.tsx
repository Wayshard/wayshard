// Wayshard pairing / connection gate.
//
// Binds pairing to the expected server application identity from the trusted
// pairing invitation, verifies possession of the identity private key with a
// fresh signed nonce (Ed25519), completes pairing with the expected identity,
// re-checks the returned identity, and only then persists the device credential.
//
// This proves application identity. It does not provide transport security;
// remote exposure/TLS/VPN/tunnel remain the user's responsibility.
import { Show, createSignal } from "solid-js"
import { Button } from "@wayshard/ui/button"
import { TextField } from "@wayshard/ui/text-field"
import { Tag } from "@wayshard/ui/tag"
import { Icon } from "@wayshard/ui/icon"
import { WayshardClient, parseInvitation, randomNonce, verifyServerIdentity } from "@wayshard/sdk"
import { useWayshard } from "../wayshard/state"

type Phase = "idle" | "verifying" | "verified" | "pairing" | "paired"

export function PairingGate() {
  const ws = useWayshard()
  const [baseUrl, setBaseUrl] = createSignal(ws.state.connection.baseUrl)
  const [invitation, setInvitation] = createSignal("")
  const [expectedServerId, setExpectedServerId] = createSignal("")
  const [expectedFingerprint, setExpectedFingerprint] = createSignal("")
  const [code, setCode] = createSignal("")
  const [deviceName, setDeviceName] = createSignal("Wayshard client")
  const [deviceKind, setDeviceKind] = createSignal("desktop")
  const [phase, setPhase] = createSignal<Phase>("idle")
  const [verified, setVerified] = createSignal<{ serverId: string; fingerprint: string } | null>(null)
  const [error, setError] = createSignal("")
  const [status, setStatus] = createSignal("")
  const [advanced, setAdvanced] = createSignal(false)
  const [token, setToken] = createSignal(ws.state.connection.token)

  function applyInvitation(text: string) {
    setInvitation(text)
    const parsed = parseInvitation(text)
    if (!parsed) return
    if (parsed.advertisedUrl) setBaseUrl(parsed.advertisedUrl)
    else if (parsed.listenUrl) setBaseUrl(parsed.listenUrl)
    if (parsed.serverId) setExpectedServerId(parsed.serverId)
    if (parsed.fingerprint) setExpectedFingerprint(parsed.fingerprint)
    if (parsed.code) setCode(parsed.code)
    setPhase("idle")
    setVerified(null)
    setError("")
    setStatus("Invitation loaded. Verify the server identity to continue.")
  }

  async function verifyAndPair() {
    setError("")
    setVerified(null)
    if (!expectedServerId() || !expectedFingerprint()) {
      setError("Paste the pairing invitation (it carries the expected server id and fingerprint).")
      return
    }
    if (!code()) {
      setError("Enter the pairing code from the invitation.")
      return
    }
    setPhase("verifying")
    setStatus("Requesting a signed challenge from the server…")
    const client = new WayshardClient(baseUrl(), "")
    let nonce = ""
    try {
      nonce = randomNonce()
      const challenge = await client.pairingChallenge(nonce)
      const result = await verifyServerIdentity(nonce, challenge, {
        serverId: expectedServerId(),
        fingerprint: expectedFingerprint(),
      })
      if (!result.ok) {
        setPhase("idle")
        setStatus("")
        setError(`Server identity verification failed: ${result.reason}. Pairing aborted.`)
        return
      }
      setVerified({ serverId: result.serverId!, fingerprint: result.fingerprint! })
      setPhase("verified")
      setStatus("Server application identity verified.")
    } catch (err) {
      setPhase("idle")
      setStatus("")
      setError(`Could not verify the server: ${String(err)}`)
      return
    }

    setPhase("pairing")
    setStatus("Completing pairing…")
    try {
      const res = await client.pair(code(), deviceName(), deviceKind(), {
        serverId: expectedServerId(),
        fingerprint: expectedFingerprint(),
      })
      if (!res.credential) {
        setError("Server did not return a device credential.")
        setPhase("idle")
        return
      }
      if (res.serverId && res.serverId !== expectedServerId()) {
        setError("Paired server id does not match the invited identity. Credential not saved.")
        setPhase("idle")
        return
      }
      if (res.fingerprint && res.fingerprint !== expectedFingerprint()) {
        setError("Paired server fingerprint does not match the invited identity. Credential not saved.")
        setPhase("idle")
        return
      }
      setPhase("paired")
      setStatus("Paired. Connecting…")
      ws.setConnection({ baseUrl: baseUrl(), token: res.credential })
    } catch (err) {
      setPhase("idle")
      setStatus("")
      setError(`Pairing failed (invalid or expired code, wrong identity, or revoked credential): ${String(err)}`)
    }
  }

  return (
    <div class="wh-pairing">
      <div class="wh-pairing-card">
        <div class="wh-pairing-title">
          <Icon name="prompt" size="small" /> Connect to Wayshard
        </div>
        <p class="wh-muted">
          Pair using the pairing invitation shown by the server. The client verifies the server
          application identity before saving any credential. Never a shared username or password.
        </p>
        <label class="wh-form-row">
          <span>Pairing invitation</span>
          <textarea
            class="wh-pairing-invite"
            placeholder={"Paste the Wayshard pairing card, e.g.\nServer: …\nFingerprint: …\nURL: …\nCode: …"}
            value={invitation()}
            onInput={(e) => applyInvitation(e.currentTarget.value)}
          />
        </label>
        <label class="wh-form-row">
          <span>Server URL</span>
          <TextField value={baseUrl()} onInput={(e: InputEvent) => setBaseUrl((e.currentTarget as HTMLInputElement).value)} />
        </label>
        <Show when={expectedServerId() || expectedFingerprint()}>
          <div class="wh-pairing-expected">
            <div class="wh-muted">Expected identity (from invitation)</div>
            <code>server {expectedServerId()}</code>
            <code>fingerprint {expectedFingerprint()}</code>
          </div>
        </Show>
        <Show when={verified()}>
          <div class="wh-pairing-verified">
            <Tag>verified</Tag>
            <code>{verified()!.fingerprint}</code>
          </div>
        </Show>
        <label class="wh-form-row">
          <span>Pairing code</span>
          <TextField value={code()} onInput={(e: InputEvent) => setCode((e.currentTarget as HTMLInputElement).value)} />
        </label>
        <div class="wh-form-row">
          <span>Device</span>
          <div class="wh-pairing-actions">
            <TextField value={deviceName()} onInput={(e: InputEvent) => setDeviceName((e.currentTarget as HTMLInputElement).value)} />
            <select class="wh-select" value={deviceKind()} onChange={(e) => setDeviceKind(e.currentTarget.value)}>
              <option value="desktop">Desktop</option>
              <option value="android">Android</option>
              <option value="web">Web</option>
              <option value="cli">CLI/TUI</option>
            </select>
          </div>
        </div>
        <Button size="small" variant="primary" onClick={() => void verifyAndPair()} disabled={phase() === "verifying" || phase() === "pairing"}>
          {phase() === "verifying" ? "Verifying…" : phase() === "pairing" ? "Pairing…" : "Verify identity & pair"}
        </Button>
        <button class="wh-link" onClick={() => setAdvanced(!advanced())}>
          {advanced() ? "Hide advanced" : "Advanced: enter a device token manually"}
        </button>
        <Show when={advanced()}>
          <label class="wh-form-row">
            <span>Device token</span>
            <TextField type="password" value={token()} onInput={(e: InputEvent) => setToken((e.currentTarget as HTMLInputElement).value)} />
          </label>
          <Button size="small" variant="secondary" onClick={() => ws.setConnection({ baseUrl: baseUrl(), token: token() })}>
            Use token
          </Button>
        </Show>
        <Show when={status()}>
          <div class="wh-muted">{status()}</div>
        </Show>
        <Show when={error()}>
          <div class="wh-error-detail">{error()}</div>
        </Show>
      </div>
    </div>
  )
}