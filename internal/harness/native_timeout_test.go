package harness

import (
	"context"
	"testing"
	"time"

	"github.com/Wayshard/wayshard/internal/domain"
)

// These tests exercise native probe timeouts on the host OS: discovery launches
// the installed executable as the server OS user, and a harness that never
// answers must be bounded by ProbeTimeout rather than hanging discovery.
// Process cleanup is best-effort; the assertion is that discovery returns.

func discoverHangingFake(t *testing.T, env string) (Installation, time.Duration) {
	t.Helper()
	t.Setenv(env, "1")
	dir := t.TempDir()
	placeFake(t, dir, "wayshard-fake-acp")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	start := time.Now()
	got, err := Discover(ctx, DiscoverOptions{
		PATH:             dir,
		IncludeLoginPATH: false,
		WellKnownDirs:    false,
		Probe:            true,
		ProbeTimeout:     1 * time.Second,
		LookupNames:      []string{"wayshard-fake-acp"},
	})
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("installations = %d, want 1", len(got))
	}
	// Generous bound: two bounded probes (version + initialize) plus process
	// teardown. The property is that discovery terminates promptly.
	if elapsed > 10*time.Second {
		t.Fatalf("discovery did not time out promptly: %s", elapsed)
	}
	return got[0], elapsed
}

// TestNativeProbeInitializeTimeout proves an ACP initialize that never answers is
// bounded and the harness is reported non-ready instead of hanging discovery.
func TestNativeProbeInitializeTimeout(t *testing.T) {
	inst, elapsed := discoverHangingFake(t, "WAYSHARD_FAKE_HANG_INITIALIZE")
	if inst.Health == domain.HarnessReady {
		t.Fatalf("hanging initialize reported ready in %s: %+v", elapsed, inst)
	}
	if inst.ACPStatus == "ok" {
		t.Fatalf("hanging initialize reported acpStatus ok in %s: %+v", elapsed, inst)
	}
}

// TestNativeProbeVersionTimeout proves a version probe that never answers is
// bounded and the harness is reported non-ready instead of hanging discovery.
func TestNativeProbeVersionTimeout(t *testing.T) {
	inst, elapsed := discoverHangingFake(t, "WAYSHARD_FAKE_HANG_VERSION")
	if inst.Health == domain.HarnessReady {
		t.Fatalf("hanging version probe reported ready in %s: %+v", elapsed, inst)
	}
}
