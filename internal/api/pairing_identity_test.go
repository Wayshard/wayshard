package api

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"testing"
)

// TestPairingChallengeRequiresNonce proves the challenge endpoint refuses a
// missing/short nonce so a fixed challenge cannot be replayed.
func TestPairingChallengeRequiresNonce(t *testing.T) {
	_, ts := testAPI(t)
	res, err := http.Get(ts.URL + "/v1/pairing/challenge")
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("missing nonce => %d, want 400", res.StatusCode)
	}
	res, err = http.Get(ts.URL + "/v1/pairing/challenge?nonce=abcd")
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("short nonce => %d, want 400", res.StatusCode)
	}
}

// TestPairingChallengeProvesIdentity proves the challenge returns a public key
// whose fingerprint matches and whose signature verifies over the fresh nonce.
func TestPairingChallengeProvesIdentity(t *testing.T) {
	_, ts := testAPI(t)
	nonce := make([]byte, 32)
	for i := range nonce {
		nonce[i] = byte(i + 1)
	}
	res, err := http.Get(ts.URL + "/v1/pairing/challenge?nonce=" + hex.EncodeToString(nonce))
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != 200 {
		t.Fatalf("challenge => %d", res.StatusCode)
	}
	var ch struct {
		ServerID    string `json:"serverId"`
		Fingerprint string `json:"fingerprint"`
		PublicKey   string `json:"publicKey"`
		Signature   string `json:"signature"`
	}
	if err := json.NewDecoder(res.Body).Decode(&ch); err != nil {
		t.Fatal(err)
	}
	pub, err := hex.DecodeString(ch.PublicKey)
	if err != nil || len(pub) != ed25519.PublicKeySize {
		t.Fatalf("public key = %q (%v)", ch.PublicKey, err)
	}
	sum := sha256.Sum256(pub)
	if ch.Fingerprint != hex.EncodeToString(sum[:])[:16] {
		t.Fatalf("fingerprint %q does not match the public key", ch.Fingerprint)
	}
	sig, _ := hex.DecodeString(ch.Signature)
	if !ed25519.Verify(ed25519.PublicKey(pub), nonce, sig) {
		t.Fatal("signature does not verify over the nonce")
	}
	// A different nonce must not verify with the same signature.
	other := append([]byte{}, nonce...)
	other[0] ^= 0xff
	if ed25519.Verify(ed25519.PublicKey(pub), other, sig) {
		t.Fatal("signature verified over a different nonce")
	}
}

func pairingComplete(t *testing.T, ts string, body map[string]any) int {
	t.Helper()
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, ts+"/v1/pairing/complete", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "127.0.0.1:1"
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	return res.StatusCode
}

// TestPairingCompleteBindsExpectedIdentity proves completion enforces the
// expected server identity and that an invitation code is single-use.
func TestPairingCompleteBindsExpectedIdentity(t *testing.T) {
	s, ts := testAPI(t)
	ctx := t.Context()
	inv, err := s.Auth.CreateInvitation(ctx, "", "test")
	if err != nil {
		t.Fatal(err)
	}

	// Wrong expected server id / fingerprint must be rejected before any device
	// is created.
	if code := pairingComplete(t, ts.URL, map[string]any{
		"code": inv.Code, "deviceName": "t", "deviceKind": "cli",
		"expectedServerId": "not-the-server", "expectedFingerprint": inv.Fingerprint,
	}); code != http.StatusUnauthorized {
		t.Fatalf("wrong expected server id => %d, want 401", code)
	}
	if code := pairingComplete(t, ts.URL, map[string]any{
		"code": inv.Code, "deviceName": "t", "deviceKind": "cli",
		"expectedServerId": inv.ServerID, "expectedFingerprint": "0000000000000000",
	}); code != http.StatusUnauthorized {
		t.Fatalf("wrong expected fingerprint => %d, want 401", code)
	}

	// Correct expected identity succeeds.
	if code := pairingComplete(t, ts.URL, map[string]any{
		"code": inv.Code, "deviceName": "t", "deviceKind": "cli",
		"expectedServerId": inv.ServerID, "expectedFingerprint": inv.Fingerprint,
	}); code != http.StatusOK {
		t.Fatalf("valid pairing => %d, want 200", code)
	}

	// The invitation code is single-use: a reused code no longer matches an
	// open invitation and is rejected.
	if code := pairingComplete(t, ts.URL, map[string]any{
		"code": inv.Code, "deviceName": "t2", "deviceKind": "cli",
		"expectedServerId": inv.ServerID, "expectedFingerprint": inv.Fingerprint,
	}); code == http.StatusOK {
		t.Fatalf("reused invitation => %d, want rejection", code)
	}
}
