//go:build linux

package app

import (
	"context"
	"net/netip"
	"os"
	"testing"

	"github.com/Wayshard/wayshard/internal/domain"
	"github.com/Wayshard/wayshard/internal/harness"
	"github.com/Wayshard/wayshard/internal/orchestrator"
	"github.com/Wayshard/wayshard/internal/provider"
	"github.com/Wayshard/wayshard/internal/routing"
	"github.com/Wayshard/wayshard/internal/sandbox"
	"github.com/Wayshard/wayshard/internal/storage"
	"github.com/Wayshard/wayshard/internal/testutil"
)

// TestMain lets the app test binary act as the in-namespace provider shim when
// the executor launches it for a provider route.
func TestMain(m *testing.M) {
	if len(os.Args) >= 2 && os.Args[1] == provider.ShimArg {
		code := 2
		if len(os.Args) >= 3 {
			code = provider.ShimMain(os.Args[2])
		}
		os.Exit(code)
	}
	os.Exit(m.Run())
}

type staticResolver struct{}

func (staticResolver) LookupNetIP(context.Context, string) ([]netip.Addr, error) {
	return []netip.Addr{netip.MustParseAddr("93.184.216.34")}, nil
}

// TestProviderExecLaunchesThroughSecureBroker proves the executor sets up the
// per-attempt broker, launches the harness in a user+network namespace through
// the production shim, and still obtains a valid artifact from a real ACP
// fixture. The harness therefore runs under the provider policy rather than raw
// host networking.
func TestProviderExecLaunchesThroughSecureBroker(t *testing.T) {
	if cap := provider.Detect(); !cap.Available {
		t.Skipf("provider capability unavailable: %s", cap.Reason)
	}
	fake := testutil.BuildFakeACP(t)
	ctx := context.Background()
	dir := t.TempDir()
	st, err := storage.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	exec := &harness.ACPExec{
		Store:            st,
		Sandbox:          &sandbox.Manager{Backend: sandbox.DefaultBackend()},
		ProviderResolver: staticResolver{},
		ProviderDialer:   provider.DefaultDialer{},
	}
	req := orchestrator.StageRequest{
		Run:     domain.Run{ID: "run-provider"},
		Task:    domain.Task{ID: "task-provider", Objective: "produce a plan"},
		Stage:   domain.Stage{ID: "stage-provider", Kind: domain.StagePlan, Ordinal: 1},
		Attempt: domain.StageAttempt{ID: "attempt-provider"},
		Route: routing.Candidate{
			Harness: domain.HarnessInstallation{
				ID: "fake", DefinitionID: "wayshard-fake-acp", DisplayName: "fake", Executable: fake, Adapter: "generic",
				Health: domain.HarnessReady, Compatibility: domain.CompatRoutable,
			},
			Network:           domain.NetworkProvider,
			ProviderTransport: domain.TransportHTTPProxy,
		},
		ProviderDestinations: []domain.ProviderDestination{{Host: "provider.test", Port: 443}},
	}
	res, err := exec.Execute(ctx, req)
	if err != nil {
		t.Fatalf("provider exec failed: %v", err)
	}
	if res.ArtifactJSON == "" {
		t.Fatal("provider route produced no artifact")
	}
}
