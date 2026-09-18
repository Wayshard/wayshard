package auth

import (
	"context"
	"testing"

	"github.com/Wayshard/wayshard/internal/secrets"
	"github.com/Wayshard/wayshard/internal/storage"
)

func TestPairingRoundTripAndRevoke(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	st, err := storage.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	v, err := secrets.Open(dir+"/vault", secrets.FileProvider{Path: dir + "/vault.key"})
	if err != nil {
		t.Fatal(err)
	}
	svc := &Service{Store: st, Vault: v, Listen: "http://127.0.0.1:7420"}
	inv, err := svc.CreateInvitation(ctx, "https://host.ts.net", "test")
	if err != nil {
		t.Fatal(err)
	}
	if inv.AdvertisedURL != "https://host.ts.net" {
		t.Fatalf("advertised %s", inv.AdvertisedURL)
	}
	res, err := svc.CompletePairing(ctx, CompletePairingRequest{Code: inv.Code, DeviceName: "cli", DeviceKind: "cli", ExpectedID: inv.ServerID, ExpectedPrint: inv.Fingerprint})
	if err != nil {
		t.Fatal(err)
	}
	p, err := svc.Authenticate(ctx, res.Credential, "")
	if err != nil {
		t.Fatal(err)
	}
	if p.DeviceID != res.DeviceID {
		t.Fatal("device")
	}
	if _, err := svc.CompletePairing(ctx, CompletePairingRequest{Code: inv.Code, DeviceName: "x", DeviceKind: "cli"}); err == nil {
		t.Fatal("invitation must be single-use")
	}
	if err := st.RevokeDevice(ctx, res.DeviceID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Authenticate(ctx, res.Credential, ""); err == nil {
		t.Fatal("revoked device must not authenticate")
	}
}

func TestWrongFingerprintRejected(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	st, _ := storage.Open(ctx, dir)
	defer st.Close()
	v, _ := secrets.Open(dir+"/vault", secrets.FileProvider{Path: dir + "/vault.key"})
	svc := &Service{Store: st, Vault: v, Listen: "http://127.0.0.1:7420"}
	inv, err := svc.CreateInvitation(ctx, "", "t")
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.CompletePairing(ctx, CompletePairingRequest{Code: inv.Code, ExpectedPrint: "deadbeef"})
	if err != ErrIdentity {
		t.Fatalf("got %v", err)
	}
}
