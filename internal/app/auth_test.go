package app

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/Wayshard/wayshard/internal/auth"
)

// appCred pairs a device directly through the auth service so HTTP tests can
// exercise the authenticated product APIs.
func appCred(t *testing.T, a *App) string {
	t.Helper()
	ctx := context.Background()
	inv, err := a.Auth.CreateInvitation(ctx, "", "test")
	if err != nil {
		t.Fatal(err)
	}
	res, err := a.Auth.CompletePairing(ctx, auth.CompletePairingRequest{Code: inv.Code, DeviceKind: "cli", DeviceName: "test"})
	if err != nil {
		t.Fatal(err)
	}
	return res.Credential
}

func postJSON(t *testing.T, url, body, cred string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, url, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if cred != "" {
		req.Header.Set("Authorization", "Bearer "+cred)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return res
}
