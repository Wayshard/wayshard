//go:build linux

package harness

import (
	"context"
	"testing"
	"time"

	"github.com/Wayshard/wayshard/internal/domain"
)

func TestProbeVersionArgs(t *testing.T) {
	if got := probeVersionArgs(OpenCodeAdapter{}); len(got) != 1 || got[0] != "--version" {
		t.Fatalf("opencode version args = %v", got)
	}
	if got := probeVersionArgs(CodexAdapter{}); len(got) != 1 || got[0] != "--version" {
		t.Fatalf("codex version args = %v", got)
	}
	if got := probeVersionArgs(GenericACPAdapter{}); got != nil {
		t.Fatalf("generic version args = %v, want nil", got)
	}
}

// TestDiscoverAdvertisedAuthIsUnknownAndRoutable proves a harness that
// advertises auth methods during initialize is reported ready with an unknown
// auth state (not assumed unauthenticated), so it stays routable while
// authentication remains harness-owned.
func TestDiscoverAdvertisedAuthIsUnknownAndRoutable(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	t.Setenv("WAYSHARD_FAKE_SCENARIO", "auth_required")
	dir := t.TempDir()
	placeFake(t, dir, "wayshard-fake-acp")
	got, err := Discover(ctx, DiscoverOptions{
		PATH: dir, IncludeLoginPATH: false, WellKnownDirs: false, Probe: true,
		LookupNames: []string{"wayshard-fake-acp"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("installations = %d, want 1", len(got))
	}
	inst := got[0]
	if inst.Health != domain.HarnessReady {
		t.Fatalf("health = %s, want ready", inst.Health)
	}
	if inst.AuthStatus != "unknown" {
		t.Fatalf("auth status = %q, want unknown", inst.AuthStatus)
	}
	if inst.Compatibility != domain.CompatRoutable && inst.Compatibility != domain.CompatEnhanced {
		t.Fatalf("compatibility = %s, want routable/enhanced", inst.Compatibility)
	}
}
