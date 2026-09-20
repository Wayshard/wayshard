package harness

import (
	"context"
	"testing"

	"github.com/Wayshard/wayshard/internal/domain"
	"github.com/Wayshard/wayshard/internal/orchestrator"
	"github.com/Wayshard/wayshard/internal/routing"
)

// TestACPExecProviderFailsClosed proves the executor independently rejects a
// provider route whose harness lacks a compatible transport or whose
// destination policy is absent. A forged or legacy RouteDecision therefore
// cannot bypass current enforcement.
func TestACPExecProviderFailsClosed(t *testing.T) {
	base := orchestrator.StageRequest{
		Run:     domain.Run{ID: "r"},
		Task:    domain.Task{ID: "t", Objective: "x"},
		Stage:   domain.Stage{ID: "s", Kind: domain.StagePlan, Ordinal: 1},
		Attempt: domain.StageAttempt{ID: "a"},
		Route: routing.Candidate{
			Harness: domain.HarnessInstallation{ID: "x", Executable: "/bin/true"},
			Network: domain.NetworkProvider,
		},
	}
	exec := &ACPExec{}

	t.Run("incompatible transport", func(t *testing.T) {
		req := base
		req.Route.ProviderTransport = domain.TransportUnknown
		res, err := exec.Execute(context.Background(), req)
		if err == nil || res.Class != domain.FailPolicy {
			t.Fatalf("expected FailPolicy, got class=%s err=%v", res.Class, err)
		}
	})

	t.Run("missing destination policy", func(t *testing.T) {
		req := base
		req.Route.ProviderTransport = domain.TransportHTTPProxy
		res, err := exec.Execute(context.Background(), req)
		if err == nil || res.Class != domain.FailPolicy {
			t.Fatalf("expected FailPolicy, got class=%s err=%v", res.Class, err)
		}
	})

	t.Run("invalid destination policy", func(t *testing.T) {
		req := base
		req.Route.ProviderTransport = domain.TransportHTTPProxy
		req.ProviderDestinations = []domain.ProviderDestination{{Host: "127.0.0.1", Port: 443}}
		res, err := exec.Execute(context.Background(), req)
		if err == nil || res.Class != domain.FailPolicy {
			t.Fatalf("expected FailPolicy, got class=%s err=%v", res.Class, err)
		}
	})
}
