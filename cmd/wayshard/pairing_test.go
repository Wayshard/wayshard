package main

import (
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wayshard/wayshard/internal/crypto"
)

type fakeServer struct {
	serverID string
	pub      []byte
	priv     []byte
	// overrides for adversarial cases
	forceServerID    string
	forceFingerprint string
	forcePublicKey   string
	forceSignature   string
	nonceShift       bool
}

func (f *fakeServer) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		nonce, _ := hex.DecodeString(r.URL.Query().Get("nonce"))
		if f.nonceShift {
			nonce = append(nonce, 0x00)
		}
		sig := crypto.Sign(f.priv, nonce)
		resp := map[string]string{
			"serverId":    f.serverID,
			"fingerprint": crypto.Fingerprint(f.pub),
			"publicKey":   hex.EncodeToString(f.pub),
			"signature":   hex.EncodeToString(sig),
		}
		if f.forceServerID != "" {
			resp["serverId"] = f.forceServerID
		}
		if f.forceFingerprint != "" {
			resp["fingerprint"] = f.forceFingerprint
		}
		if f.forcePublicKey != "" {
			resp["publicKey"] = f.forcePublicKey
		}
		if f.forceSignature != "" {
			resp["signature"] = f.forceSignature
		}
		_ = json.NewEncoder(w).Encode(resp)
	}
}

func newFakeServer(t *testing.T) (*fakeServer, *httptest.Server) {
	t.Helper()
	pub, priv, err := crypto.NewKeypair()
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeServer{serverID: "srv-1", pub: pub, priv: priv}
	ts := httptest.NewServer(f.handler())
	t.Cleanup(ts.Close)
	return f, ts
}

func TestVerifyServerIdentityValid(t *testing.T) {
	f, ts := newFakeServer(t)
	c := &client{base: ts.URL, http: ts.Client()}
	if err := verifyServerIdentity(c, f.serverID, crypto.Fingerprint(f.pub)); err != nil {
		t.Fatalf("valid identity rejected: %v", err)
	}
}

func TestVerifyServerIdentityAdversarial(t *testing.T) {
	f, ts := newFakeServer(t)
	c := &client{base: ts.URL, http: ts.Client()}
	fp := crypto.Fingerprint(f.pub)

	if err := verifyServerIdentity(c, "wrong-id", fp); err == nil {
		t.Fatal("wrong server id accepted")
	}
	if err := verifyServerIdentity(c, f.serverID, "0000000000000000"); err == nil {
		t.Fatal("wrong fingerprint accepted")
	}

	// Wrong public key (a different valid key) must fail the fingerprint check.
	otherPub, _, err := crypto.NewKeypair()
	if err != nil {
		t.Fatal(err)
	}
	f.forcePublicKey = hex.EncodeToString(otherPub)
	if err := verifyServerIdentity(c, f.serverID, fp); err == nil {
		t.Fatal("wrong public key accepted")
	}
	f.forcePublicKey = ""

	// Invalid signature.
	f.forceSignature = "00"
	if err := verifyServerIdentity(c, f.serverID, fp); err == nil {
		t.Fatal("invalid signature accepted")
	}
	f.forceSignature = ""

	// Signature for a different nonce (replay).
	f.nonceShift = true
	if err := verifyServerIdentity(c, f.serverID, fp); err == nil {
		t.Fatal("signature over a different nonce accepted")
	}
	f.nonceShift = false

	// Presented fingerprint not matching the presented public key.
	f.forceFingerprint = "0000000000000000"
	if err := verifyServerIdentity(c, f.serverID, fp); err == nil {
		t.Fatal("mismatched presented fingerprint accepted")
	}
}

func TestParseInvitationCard(t *testing.T) {
	card := "Wayshard pairing\nServer: srv-1\nFingerprint: abc123\nURL: http://host:7420\nCode: ABCD-1234\nExpires: soon"
	inv, err := parseInvitation(card)
	if err != nil {
		t.Fatal(err)
	}
	if inv.ServerID != "srv-1" || inv.Fingerprint != "abc123" || inv.AdvertisedURL != "http://host:7420" || inv.Code != "ABCD-1234" {
		t.Fatalf("parsed invitation = %+v", inv)
	}
	if _, err := parseInvitation("Server: s\nFingerprint: f"); err == nil {
		t.Fatal("invitation without a code accepted")
	}
	if _, err := parseInvitation(JSONInvitation()); err != nil {
		t.Fatalf("JSON invitation rejected: %v", err)
	}
}

func JSONInvitation() string {
	b, _ := json.Marshal(map[string]string{"serverId": "s", "fingerprint": "f", "advertisedUrl": "http://h", "code": "C"})
	return string(b)
}
