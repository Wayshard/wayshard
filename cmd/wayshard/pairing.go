package main

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/Wayshard/wayshard/internal/crypto"
)

// pairInvitation carries the expected server application identity from the
// trusted pairing invitation so terminal pairing binds to it.
type pairInvitation struct {
	AdvertisedURL string
	ServerID      string
	Fingerprint   string
	Code          string
}

var cardLine = regexp.MustCompile(`(?i)^\s*(server|fingerprint|url|listen|code|expires)\s*:\s*(.+?)\s*$`)

// parseInvitation accepts the Wayshard pairing card text or its JSON form.
func parseInvitation(text string) (*pairInvitation, error) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return nil, fmt.Errorf("empty pairing invitation")
	}
	if strings.HasPrefix(trimmed, "{") {
		var raw struct {
			AdvertisedURL string `json:"advertisedUrl"`
			URL           string `json:"url"`
			ListenURL     string `json:"listenUrl"`
			ServerID      string `json:"serverId"`
			Fingerprint   string `json:"fingerprint"`
			Code          string `json:"code"`
		}
		if err := json.Unmarshal([]byte(trimmed), &raw); err != nil {
			return nil, fmt.Errorf("parse pairing invitation JSON: %w", err)
		}
		out := &pairInvitation{AdvertisedURL: raw.AdvertisedURL, ServerID: raw.ServerID, Fingerprint: raw.Fingerprint, Code: raw.Code}
		if out.AdvertisedURL == "" {
			out.AdvertisedURL = raw.URL
		}
		if out.AdvertisedURL == "" {
			out.AdvertisedURL = raw.ListenURL
		}
		if out.Code == "" {
			return nil, fmt.Errorf("pairing invitation is missing a code")
		}
		return out, nil
	}
	out := &pairInvitation{}
	for _, line := range strings.Split(trimmed, "\n") {
		m := cardLine.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		switch strings.ToLower(m[1]) {
		case "server":
			out.ServerID = m[2]
		case "fingerprint":
			out.Fingerprint = m[2]
		case "url":
			out.AdvertisedURL = m[2]
		case "listen":
			if out.AdvertisedURL == "" {
				out.AdvertisedURL = m[2]
			}
		case "code":
			out.Code = m[2]
		}
	}
	if out.Code == "" {
		return nil, fmt.Errorf("pairing invitation is missing a code")
	}
	return out, nil
}

// verifyServerIdentity fetches a signed challenge for a fresh nonce and proves
// the server possesses the expected application identity private key.
func verifyServerIdentity(c *client, expectedID, expectedFP string) error {
	nonce, err := crypto.Random(32)
	if err != nil {
		return err
	}
	path := "/v1/pairing/challenge?nonce=" + hex.EncodeToString(nonce)
	body, err := c.get(path)
	if err != nil {
		return fmt.Errorf("challenge request: %w", err)
	}
	var ch struct {
		ServerID    string `json:"serverId"`
		Fingerprint string `json:"fingerprint"`
		PublicKey   string `json:"publicKey"`
		Signature   string `json:"signature"`
	}
	if err := json.Unmarshal(body, &ch); err != nil {
		return fmt.Errorf("parse challenge: %w", err)
	}
	pub, err := hex.DecodeString(ch.PublicKey)
	if err != nil || len(pub) == 0 {
		return fmt.Errorf("challenge returned no usable public key")
	}
	fp := crypto.Fingerprint(pub)
	if fp != expectedFP {
		return fmt.Errorf("server fingerprint %q does not match the trusted invitation %q", fp, expectedFP)
	}
	if ch.Fingerprint != "" && ch.Fingerprint != fp {
		return fmt.Errorf("presented fingerprint does not match the presented public key")
	}
	if ch.ServerID != expectedID {
		return fmt.Errorf("server id %q does not match the trusted invitation %q", ch.ServerID, expectedID)
	}
	sig, err := hex.DecodeString(ch.Signature)
	if err != nil {
		return fmt.Errorf("challenge signature malformed: %w", err)
	}
	if !crypto.Verify(pub, nonce, sig) {
		return fmt.Errorf("server signature is not valid for this nonce")
	}
	return nil
}
