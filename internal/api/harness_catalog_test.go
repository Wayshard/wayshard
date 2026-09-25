package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/Wayshard/wayshard/internal/harness"
)

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
