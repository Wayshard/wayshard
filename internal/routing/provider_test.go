package routing

import (
	"context"
	"strings"
	"testing"

	"github.com/Wayshard/wayshard/internal/domain"
)

// TestProviderRouteBlockedWithoutCapability proves a harness that needs
// provider network is non-viable unless secure provider isolation is enabled.
func TestProviderRouteBlockedWithoutCapability(t *testing.T) {
	cands := []Candidate{{
		Harness:   domain.HarnessInstallation{ID: "real", DisplayName: "real", Health: domain.HarnessReady, Compatibility: domain.CompatRoutable},
		Isolation: domain.IsolationOuterOnly,
		Network:   domain.NetworkProvider,
	}}
	dec := (&Router{}).Route(context.Background(), domain.StagePlan, Config{}, cands, nil)
	if dec.Blocked != domain.BlockedNoViableRoute {
		t.Fatalf("expected NO_VIABLE_ROUTE, got %+v", dec)
	}
	if !strings.Contains(dec.Detail, "provider network") {
		t.Fatalf("detail should explain provider isolation: %q", dec.Detail)
	}
	if dec.Candidate.Harness.ID != "" {
		t.Fatal("no candidate may be selected")
	}

	// A no-network harness remains viable.
	none := []Candidate{{
		Harness:   domain.HarnessInstallation{ID: "fake", DisplayName: "fake", Health: domain.HarnessReady, Compatibility: domain.CompatRoutable},
		Isolation: domain.IsolationOuterOnly,
		Network:   domain.NetworkNone,
	}}
	if got := (&Router{}).Route(context.Background(), domain.StagePlan, Config{}, none, nil); got.Blocked != "" {
		t.Fatalf("no-network harness should be viable: %+v", got)
	}

	// Permission without capability must still block.
	permOnly := Config{AllowProviderNetwork: true, ProviderNetworkAvailable: false}
	if got := (&Router{}).Route(context.Background(), domain.StagePlan, permOnly, cands, nil); got.Blocked != domain.BlockedNoViableRoute {
		t.Fatalf("permission without capability must block: %+v", got)
	}
	// Capability without permission must still block.
	capOnly := Config{AllowProviderNetwork: false, ProviderNetworkAvailable: true}
	if got := (&Router{}).Route(context.Background(), domain.StagePlan, capOnly, cands, nil); got.Blocked != domain.BlockedNoViableRoute {
		t.Fatalf("capability without permission must block: %+v", got)
	}
	// Permission + capability makes the provider route viable.
	both := Config{AllowProviderNetwork: true, ProviderNetworkAvailable: true}
	if got := (&Router{}).Route(context.Background(), domain.StagePlan, both, cands, nil); got.Blocked != "" {
		t.Fatalf("provider route should be viable with permission+capability: %+v", got)
	}
}
