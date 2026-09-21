// Adversarial tests for Wayshard application-identity verification.
import { describe, expect, test } from "bun:test"
import { parseInvitation, randomNonce, verifyServerIdentity, type PairingChallenge } from "./index"

function toHex(b: Uint8Array): string {
  return Array.from(b).map((x) => x.toString(16).padStart(2, "0")).join("")
}
function fromHex(h: string): Uint8Array {
  const out = new Uint8Array(h.length / 2)
  for (let i = 0; i < out.length; i++) out[i] = parseInt(h.slice(i * 2, i * 2 + 2), 16)
  return out
}
async function fingerprint(pub: ArrayBuffer): Promise<string> {
  const digest = await crypto.subtle.digest("SHA-256", pub)
  return toHex(new Uint8Array(digest)).slice(0, 16)
}

async function fixture(nonce: string) {
  const kp = await crypto.subtle.generateKey({ name: "Ed25519" }, true, ["sign", "verify"])
  const raw = new Uint8Array(await crypto.subtle.exportKey("raw", kp.publicKey))
  const sig = new Uint8Array(await crypto.subtle.sign({ name: "Ed25519" }, kp.privateKey, fromHex(nonce) as unknown as ArrayBuffer))
  const challenge: PairingChallenge = {
    serverId: "srv-1",
    fingerprint: await fingerprint(raw.buffer),
    publicKey: toHex(raw),
    signature: toHex(sig),
  }
  return { kp, challenge }
}

describe("verifyServerIdentity", () => {
  test("valid expected identity succeeds with a fresh nonce", async () => {
    const nonce = randomNonce()
    const { challenge } = await fixture(nonce)
    const r = await verifyServerIdentity(nonce, challenge, { serverId: "srv-1", fingerprint: challenge.fingerprint })
    expect(r.ok).toBe(true)
    expect(r.fingerprint).toBe(challenge.fingerprint)
  })

  test("wrong expected server id fails", async () => {
    const nonce = randomNonce()
    const { challenge } = await fixture(nonce)
    const r = await verifyServerIdentity(nonce, challenge, { serverId: "other", fingerprint: challenge.fingerprint })
    expect(r.ok).toBe(false)
  })

  test("wrong expected fingerprint fails", async () => {
    const nonce = randomNonce()
    const { challenge } = await fixture(nonce)
    const r = await verifyServerIdentity(nonce, challenge, { serverId: "srv-1", fingerprint: "0000000000000000" })
    expect(r.ok).toBe(false)
  })

  test("presented fingerprint not matching the public key fails", async () => {
    const nonce = randomNonce()
    const { challenge } = await fixture(nonce)
    const r = await verifyServerIdentity(nonce, { ...challenge, fingerprint: "0000000000000000" }, {})
    expect(r.ok).toBe(false)
  })

  test("invalid signature fails", async () => {
    const nonce = randomNonce()
    const { challenge } = await fixture(nonce)
    const bad = challenge.signature.slice(0, -2) + (challenge.signature.endsWith("00") ? "11" : "00")
    const r = await verifyServerIdentity(nonce, { ...challenge, signature: bad }, { serverId: "srv-1", fingerprint: challenge.fingerprint })
    expect(r.ok).toBe(false)
  })

  test("signature for a different nonce fails (replay)", async () => {
    const nonce = randomNonce()
    const { challenge } = await fixture(nonce)
    // verify the old signature against a brand new nonce
    const fresh = randomNonce()
    const r = await verifyServerIdentity(fresh, challenge, { serverId: "srv-1", fingerprint: challenge.fingerprint })
    expect(r.ok).toBe(false)
  })

  test("malformed public key fails", async () => {
    const nonce = randomNonce()
    const r = await verifyServerIdentity(nonce, { serverId: "s", fingerprint: "x", publicKey: "zz", signature: "00" }, {})
    expect(r.ok).toBe(false)
  })
})

describe("parseInvitation", () => {
  test("parses the pairing card text", () => {
    const inv = parseInvitation("Wayshard pairing\nServer: srv-1\nFingerprint: abc123\nURL: http://host:7420\nCode: ABCD-1234\nExpires: soon")
    expect(inv?.serverId).toBe("srv-1")
    expect(inv?.fingerprint).toBe("abc123")
    expect(inv?.advertisedUrl).toBe("http://host:7420")
    expect(inv?.code).toBe("ABCD-1234")
  })

  test("parses JSON invitations", () => {
    const inv = parseInvitation(JSON.stringify({ serverId: "s", fingerprint: "f", advertisedUrl: "http://h", code: "C" }))
    expect(inv?.code).toBe("C")
    expect(inv?.advertisedUrl).toBe("http://h")
  })

  test("rejects material without a code", () => {
    expect(parseInvitation("Server: s\nFingerprint: f")).toBeNull()
    expect(parseInvitation("")).toBeNull()
  })
})
