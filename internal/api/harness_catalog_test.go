package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/Wayshard/wayshard/internal/domain"
	"github.com/Wayshard/wayshard/internal/harness"
)

// TestSandboxDiagnosticConsistency proves GET /v1/sandbox never reports provider
// networking as unavailable while the authoritative capability says it is
// available. This is the Pass 1C audit diagnostic regression.
func TestSandboxDiagnosticConsistency(t *testing.T) {
	s, ts := testAPI(t)
	s.ProviderNet = domain.ProviderNetworkCapability{Available: true, Mode: "netns_broker"}
	cred := issueCred(t, s)

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/v1/sandbox", nil)
	req.Header.Set("Authorization", "Bearer "+cred)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var body struct {
		Missing         []string `json:"missing"`
		Detail          string   `json:"detail"`
		ProviderNetwork struct {
			Available bool `json:"available"`
		} `json:"providerNetwork"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if !body.ProviderNetwork.Available {
		t.Fatalf("providerNetwork.available = false, want true: %+v", body)
	}
	for _, m := range body.Missing {
		if m == "provider_network" {
			t.Fatalf("missing contains provider_network while providerNetwork.available=true: %+v", body)
		}
	}
	if containsUnavailableClaim(body.Detail) {
		t.Fatalf("detail claims provider network unavailable while available=true: %q", body.Detail)
	}
}

func containsUnavailableClaim(detail string) bool {
	d := strings.ToLower(detail)
	return strings.Contains(d, "provider network unavailable") || strings.Contains(d, "secure provider network unavailable")
}

// TestHarnessDefinitionsEndpoint proves the effective catalog is exposed
// separately from discovered installations.
func TestHarnessDefinitionsEndpoint(t *testing.T) {
	s, ts := testAPI(t)
	s.Catalog = harness.ShippedCatalog()
	cred := issueCred(t, s)

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/v1/harness-definitions", nil)
	req.Header.Set("Authorization", "Bearer "+cred)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var body struct {
		Definitions []struct {
			ID      string `json:"id"`
			Source  string `json:"source"`
			Enabled bool   `json:"enabled"`
			ACP     string `json:"acp"`
		} `json:"definitions"`
		Diagnostics []any `json:"diagnostics"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	found := map[string]bool{}
	for _, d := range body.Definitions {
		found[d.ID] = true
		if d.Source != string(harness.SourceShipped) {
			t.Fatalf("shipped definition %s has source %q", d.ID, d.Source)
		}
	}
	for _, id := range []string{"opencode", "codex", "claude", "grok", "amp", "pi", "omp"} {
		if !found[id] {
			t.Fatalf("definition %s missing from endpoint: %+v", id, found)
		}
	}
}
