package routing

import (
	"context"
	"strings"
	"testing"

	"github.com/Wayshard/wayshard/internal/domain"
)

func readyHarness(id string) domain.HarnessInstallation {
	return domain.HarnessInstallation{ID: id, DisplayName: id, Health: domain.HarnessReady, Compatibility: domain.CompatRoutable}
}

func providerCand(transport domain.ProviderTransport) Candidate {
	return Candidate{
		Harness:           readyHarness("real"),
		Isolation:         domain.IsolationOuterOnly,
		Network:           domain.NetworkProvider,
		ProviderTransport: transport,
	}
}

func noNetCand() Candidate {
	return Candidate{
		Harness:   readyHarness("fake"),
		Isolation: domain.IsolationOuterOnly,
		Network:   domain.NetworkNone,
	}
}

func validDests() []domain.ProviderDestination {
	return []domain.ProviderDestination{{Host: "api.example.com", Port: 443}}
}

// TestProviderRouteMatrix proves a provider route needs permission, real
// platform capability, a compatible transport and a valid destination policy,
// all independently.
func TestProviderRouteMatrix(t *testing.T) {
	capOn := domain.ProviderNetworkCapability{Available: true, Mode: "netns_connect_broker"}
	capOff := domain.ProviderNetworkCapability{Available: false, Reason: "no userns"}
	cands := []Candidate{providerCand(domain.TransportHTTPProxy)}

	cases := []struct {
		name     string
		cfg      Config
		blocked  bool
		contains string
	}{
		{name: "permission false capability true", cfg: Config{AllowProviderNetwork: false, ProviderNet: capOn, ProviderDestinations: validDests()}, blocked: true, contains: "permission"},
		{name: "permission true capability false", cfg: Config{AllowProviderNetwork: true, ProviderNet: capOff, ProviderDestinations: validDests()}, blocked: true, contains: "isolation unavailable"},
		{name: "transport incompatible", cfg: Config{AllowProviderNetwork: true, ProviderNet: capOn, ProviderDestinations: validDests()}, blocked: true, contains: "transport"},
		{name: "missing destination", cfg: Config{AllowProviderNetwork: true, ProviderNet: capOn}, blocked: true, contains: "destination"},
		{name: "invalid destination", cfg: Config{AllowProviderNetwork: true, ProviderNet: capOn, ProviderDestinations: []domain.ProviderDestination{{Host: "127.0.0.1", Port: 443}}}, blocked: true, contains: "destination"},
		{name: "all satisfied", cfg: Config{AllowProviderNetwork: true, ProviderNet: capOn, ProviderDestinations: validDests()}, blocked: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Use a transport-incompatible candidate only for the transport case.
			cs := cands
			if tc.name == "transport incompatible" {
				cs = []Candidate{providerCand(domain.TransportUnknown)}
			}
			if tc.name == "all satisfied" {
				cs = []Candidate{providerCand(domain.TransportHTTPProxy)}
			}
			got := (&Router{}).Route(context.Background(), domain.StagePlan, tc.cfg, cs, nil)
			if tc.blocked {
				if got.Blocked != domain.BlockedNoViableRoute {
					t.Fatalf("expected blocked, got %+v", got)
				}
				if tc.contains != "" && !strings.Contains(got.Detail, tc.contains) {
					t.Fatalf("detail %q does not contain %q", got.Detail, tc.contains)
				}
				return
			}
			if got.Blocked != "" {
				t.Fatalf("expected viable, got blocked %s: %s", got.Blocked, got.Detail)
			}
			if !strings.Contains(got.Evidence, "provider{") {
				t.Fatalf("viable provider decision lacks evidence: %q", got.Evidence)
			}
		})
	}

	// A no-network harness stays viable even with provider disabled.
	if got := (&Router{}).Route(context.Background(), domain.StagePlan, Config{}, []Candidate{noNetCand()}, nil); got.Blocked != "" {
		t.Fatalf("no-network harness should be viable: %+v", got)
	}
}
