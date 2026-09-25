// Package auth implements server identity, pairing invitations, and per-device credentials.
package auth

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/Wayshard/wayshard/internal/credentials"
	"github.com/Wayshard/wayshard/internal/crypto"
	"github.com/Wayshard/wayshard/internal/domain"
	"github.com/Wayshard/wayshard/internal/id"
	"github.com/Wayshard/wayshard/internal/storage"
)

const (
	pairingTTL      = 10 * time.Minute
	sessionTTL      = 30 * 24 * time.Hour
	deviceSecretLen = 32
	inviteSecretLen = 24
)

var (
	ErrUnauthorized = errors.New("unauthorized")
	ErrExpired      = errors.New("pairing invitation expired")
	ErrUsed         = errors.New("pairing invitation already used")
	ErrRevoked      = errors.New("device revoked")
	ErrIdentity     = errors.New("server identity mismatch")
)

type Service struct {
	Store       *storage.Store
	Credentials *credentials.Store
	Listen      string
}

type PairingResult struct {
	InvitationID  string `json:"invitationId"`
	Code          string `json:"code"`
	AdvertisedURL string `json:"advertisedUrl"`
	ListenURL     string `json:"listenUrl"`
	ServerID      string `json:"serverId"`
	Fingerprint   string `json:"fingerprint"`
	ExpiresAt     string `json:"expiresAt"`
}

type CompletePairingRequest struct {
	Code          string `json:"code"`
	DeviceName    string `json:"deviceName"`
	DeviceKind    string `json:"deviceKind"`
	ExpectedID    string `json:"expectedServerId"`
	ExpectedPrint string `json:"expectedFingerprint"`
}

type CompletePairingResponse struct {
	DeviceID    string `json:"deviceId"`
	Credential  string `json:"credential"`
	ServerID    string `json:"serverId"`
	Fingerprint string `json:"fingerprint"`
	Session     string `json:"session,omitempty"`
}

type Principal struct {
	DeviceID string
	Kind     string
	Name     string
}

func (s *Service) EnsureIdentity(ctx context.Context) (*domain.ServerIdentity, ed25519.PrivateKey, error) {
	ident, err := s.Store.GetServerIdentity(ctx)
	if err == nil {
		priv, err := s.Credentials.Get(ctx, "server.identity.private")
		if err != nil {
			return nil, nil, err
		}
		return ident, ed25519.PrivateKey(priv), nil
	}
	if !errors.Is(err, storage.ErrNotFound) {
		return nil, nil, err
	}
	pub, priv, err := crypto.NewKeypair()
	if err != nil {
		return nil, nil, err
	}
	ident = &domain.ServerIdentity{
		ServerID:    id.New(),
		PublicKey:   pub,
		DisplayName: "Wayshard",
		CreatedAt:   time.Now().UTC(),
	}
	if err := s.Credentials.Put(ctx, "server.identity.private", priv); err != nil {
		return nil, nil, err
	}
	if err := s.Store.UpsertServerIdentity(ctx, *ident); err != nil {
		return nil, nil, err
	}
	return ident, priv, nil
}

func (s *Service) Fingerprint(ctx context.Context) (serverID, fp string, err error) {
	ident, _, err := s.EnsureIdentity(ctx)
	if err != nil {
		return "", "", err
	}
	return ident.ServerID, crypto.Fingerprint(ident.PublicKey), nil
}

func (s *Service) CreateInvitation(ctx context.Context, advertisedURL, createdBy string) (*PairingResult, error) {
	ident, _, err := s.EnsureIdentity(ctx)
	if err != nil {
		return nil, err
	}
	secret, err := crypto.Random(inviteSecretLen)
	if err != nil {
		return nil, err
	}
	code := crypto.EncodeCode(secret)
	salt, err := crypto.NewSalt()
	if err != nil {
		return nil, err
	}
	sum := crypto.ArgonHash([]byte(code), salt)
	stored := append(salt, sum...)
	inv := &domain.PairingInvitation{
		CodeHash:      stored,
		AdvertisedURL: advertisedURL,
		ListenURL:     s.Listen,
		ServerFinger:  crypto.Fingerprint(ident.PublicKey),
		ExpiresAt:     time.Now().UTC().Add(pairingTTL),
		CreatedBy:     createdBy,
	}
	if advertisedURL == "" {
		inv.AdvertisedURL = s.Listen
	}
	if err := s.Store.InsertInvitation(ctx, inv); err != nil {
		return nil, err
	}
	return &PairingResult{
		InvitationID:  inv.ID,
		Code:          code,
		AdvertisedURL: inv.AdvertisedURL,
		ListenURL:     inv.ListenURL,
		ServerID:      ident.ServerID,
		Fingerprint:   inv.ServerFinger,
		ExpiresAt:     inv.ExpiresAt.Format(time.RFC3339),
	}, nil
}

func (s *Service) CompletePairing(ctx context.Context, req CompletePairingRequest) (*CompletePairingResponse, error) {
	ident, _, err := s.EnsureIdentity(ctx)
	if err != nil {
		return nil, err
	}
	fp := crypto.Fingerprint(ident.PublicKey)
	if req.ExpectedID != "" && req.ExpectedID != ident.ServerID {
		return nil, ErrIdentity
	}
	if req.ExpectedPrint != "" && req.ExpectedPrint != fp {
		return nil, ErrIdentity
	}
	invs, err := s.Store.ListOpenInvitations(ctx)
	if err != nil {
		return nil, err
	}
	var match *domain.PairingInvitation
	for i := range invs {
		inv := &invs[i]
		if len(inv.CodeHash) < 16+32 {
			continue
		}
		salt := inv.CodeHash[:16]
		want := inv.CodeHash[16:]
		got := crypto.ArgonHash([]byte(req.Code), salt)
		if crypto.Equal(want, got) {
			match = inv
			break
		}
	}
	if match == nil {
		return nil, ErrUnauthorized
	}
	if time.Now().After(match.ExpiresAt) {
		return nil, ErrExpired
	}
	if err := s.Store.ConsumeInvitation(ctx, match.ID); err != nil {
		return nil, ErrUsed
	}
	cred, err := crypto.Random(deviceSecretLen)
	if err != nil {
		return nil, err
	}
	salt, err := crypto.NewSalt()
	if err != nil {
		return nil, err
	}
	ver := crypto.ArgonHash(cred, salt)
	kind := req.DeviceKind
	if kind == "" {
		kind = "unknown"
	}
	name := req.DeviceName
	if name == "" {
		name = kind
	}
	dev := &domain.Device{
		Name:      name,
		Kind:      kind,
		Verifier:  ver,
		Salt:      salt,
		PairingID: match.ID,
	}
	if err := s.Store.InsertDevice(ctx, dev); err != nil {
		return nil, err
	}
	token := "wsd_" + crypto.EncodeCode(cred)
	resp := &CompletePairingResponse{
		DeviceID:    dev.ID,
		Credential:  token,
		ServerID:    ident.ServerID,
		Fingerprint: fp,
	}
	if kind == "web" {
		sess, err := s.IssueSession(ctx, dev.ID)
		if err != nil {
			return nil, err
		}
		resp.Session = sess
	}
	return resp, nil
}

func (s *Service) IssueSession(ctx context.Context, deviceID string) (string, error) {
	raw, err := crypto.Random(32)
	if err != nil {
		return "", err
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	sess := &domain.AuthSession{
		DeviceID:  deviceID,
		TokenHash: crypto.HashBytes([]byte(token)),
		ExpiresAt: time.Now().UTC().Add(sessionTTL),
	}
	if err := s.Store.InsertAuthSession(ctx, sess); err != nil {
		return "", err
	}
	return token, nil
}

func (s *Service) Authenticate(ctx context.Context, bearer, sessionCookie string) (*Principal, error) {
	if bearer != "" {
		return s.authenticateDevice(ctx, bearer)
	}
	if sessionCookie != "" {
		return s.authenticateSession(ctx, sessionCookie)
	}
	return nil, ErrUnauthorized
}

func (s *Service) authenticateDevice(ctx context.Context, token string) (*Principal, error) {
	const prefix = "wsd_"
	if len(token) < len(prefix)+8 {
		return nil, ErrUnauthorized
	}
	if token[:len(prefix)] != prefix {
		return nil, ErrUnauthorized
	}
	raw, err := crypto.DecodeCode(token[len(prefix):])
	if err != nil {
		return nil, ErrUnauthorized
	}
	devs, err := s.Store.ListDevices(ctx)
	if err != nil {
		return nil, err
	}
	for i := range devs {
		d := &devs[i]
		if d.RevokedAt != nil {
			continue
		}
		got := crypto.ArgonHash(raw, d.Salt)
		if crypto.Equal(got, d.Verifier) {
			_ = s.Store.TouchDevice(ctx, d.ID)
			return &Principal{DeviceID: d.ID, Kind: d.Kind, Name: d.Name}, nil
		}
	}
	return nil, ErrUnauthorized
}

func (s *Service) authenticateSession(ctx context.Context, token string) (*Principal, error) {
	sess, err := s.Store.LookupAuthSession(ctx, crypto.HashBytes([]byte(token)))
	if err != nil {
		return nil, ErrUnauthorized
	}
	if time.Now().After(sess.ExpiresAt) {
		return nil, ErrExpired
	}
	d, err := s.Store.GetDevice(ctx, sess.DeviceID)
	if err != nil {
		return nil, ErrUnauthorized
	}
	if d.RevokedAt != nil {
		return nil, ErrRevoked
	}
	_ = s.Store.TouchDevice(ctx, d.ID)
	return &Principal{DeviceID: d.ID, Kind: d.Kind, Name: d.Name}, nil
}

// Challenge signs a fresh client nonce with the server application identity key
// and returns the public identity so a client can independently verify
// possession of the expected key.
func (s *Service) Challenge(ctx context.Context, nonce []byte) (serverID, fp string, pub ed25519.PublicKey, sig []byte, err error) {
	ident, priv, err := s.EnsureIdentity(ctx)
	if err != nil {
		return "", "", nil, nil, err
	}
	return ident.ServerID, crypto.Fingerprint(ident.PublicKey), ident.PublicKey, crypto.Sign(priv, nonce), nil
}

func LocalAdminBypass(listenHost string) bool {
	switch listenHost {
	case "127.0.0.1", "localhost", "::1":
		return true
	default:
		return false
	}
}

func FormatPairingCard(p *PairingResult) string {
	return fmt.Sprintf("Wayshard pairing\nServer: %s\nFingerprint: %s\nURL: %s\nCode: %s\nExpires: %s\n",
		p.ServerID, p.Fingerprint, p.AdvertisedURL, p.Code, p.ExpiresAt)
}
