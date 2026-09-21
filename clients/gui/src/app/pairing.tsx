// Wayshard pairing / connection gate.
//
// Adapts the imported OpenCode connection/dialog presentation to Wayshard's
// per-device pairing model. No usernames or passwords: the client verifies the
// server identity via the pairing challenge, consumes a short-lived invitation
// code, and persists the resulting device credential.
import { Show, createSignal } from "solid-js"
import { Button } from "@wayshard/ui/button"
import { TextField } from "@wayshard/ui/text-field"
import { Tag } from "@wayshard/ui/tag"
import { Icon } from "@wayshard/ui/icon"
import { WayshardClient } from "@wayshard/sdk"
import { useWayshard } from "../wayshard/state"

export function PairingGate() {
  const ws = useWayshard()
  const [baseUrl, setBaseUrl] = createSignal(ws.state.connection.baseUrl)
  const [identity, setIdentity] = createSignal<{ serverId: string; fingerprint: string } | null>(null)
  const [code, setCode] = createSignal("")
  const [deviceName, setDeviceName] = createSignal("Wayshard client")
  const [deviceKind, setDeviceKind] = createSignal("desktop")
  const [status, setStatus] = createSignal("")
  const [error, setError] = createSignal("")
  const [advanced, setAdvanced] = createSignal(false)
  const [token, setToken] = createSignal(ws.state.connection.token)

  async function verify() {
    setError("")
    setStatus("Contacting server…")
    try {
      const client = new WayshardClient(baseUrl(), token())
      const challenge = await client.pairingChallenge()
      setIdentity({ serverId: challenge.serverId, fingerprint: challenge.fingerprint })
      setStatus("Server identity retrieved. Verify the fingerprint before pairing.")
    } catch (err) {
      setStatus("")
      setError(`Could not reach the server: ${String(err)}`)
    }
  }

  async function completePairing() {
    setError("")
    setStatus("Pairing…")
    try {
      const client = new WayshardClient(baseUrl(), "")
      const res = await client.pair(baseUrl() ? code() : "", deviceName(), deviceKind())
      if (!res.credential) {
        setError("Server did not return a device credential.")
        return
      }
      setStatus("Paired. Connecting…")
      ws.setConnection({ baseUrl: baseUrl(), token: res.credential })
    } catch (err) {
      setStatus("")
      setError(`Pairing failed (invalid or expired code, or revoked credential): ${String(err)}`)
    }
  }

  return (
    <div class="wh-pairing">
      <div class="wh-pairing-card">
        <div class="wh-pairing-title">
          <Icon name="prompt" size="small" /> Connect to Wayshard
        </div>
        <p class="wh-muted">
          Pair this device with a Wayshard server. Pairing uses short-lived invitation codes and per-device credentials —
          never a shared username or password.
        </p>
        <label class="wh-form-row">
          <span>Server</span>
          <TextField value={baseUrl()} onInput={(e: InputEvent) => setBaseUrl((e.currentTarget as HTMLInputElement).value)} />
        </label>
        <div class="wh-pairing-actions">
          <Button size="small" variant="secondary" onClick={() => void verify()}>
            Verify server
          </Button>
          <Show when={identity()}>
            <Tag>id {identity()!.serverId.slice(0, 8)}</Tag>
          </Show>
        </div>
        <Show when={identity()}>
          <div class="wh-pairing-fingerprint">
            <div class="wh-muted">Fingerprint</div>
            <code>{identity()!.fingerprint}</code>
          </div>
          <label class="wh-form-row">
            <span>Pairing code</span>
            <TextField value={code()} onInput={(e: InputEvent) => setCode((e.currentTarget as HTMLInputElement).value)} />
          </label>
          <label class="wh-form-row">
            <span>Device name</span>
            <TextField value={deviceName()} onInput={(e: InputEvent) => setDeviceName((e.currentTarget as HTMLInputElement).value)} />
          </label>
          <label class="wh-form-row">
            <span>Device type</span>
            <select class="wh-select" value={deviceKind()} onChange={(e) => setDeviceKind(e.currentTarget.value)}>
              <option value="desktop">Desktop</option>
              <option value="android">Android</option>
              <option value="web">Web</option>
              <option value="cli">CLI/TUI</option>
            </select>
          </label>
          <Button size="small" variant="primary" onClick={() => void completePairing()}>
            Complete pairing
          </Button>
        </Show>
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
