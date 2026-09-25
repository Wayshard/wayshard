package routing

import (
	"context"
	"testing"

	"github.com/Wayshard/wayshard/internal/domain"
	"github.com/Wayshard/wayshard/internal/jev"
)

func TestHardFilterImpossible(t *testing.T) {
	r := Router{Engine: jev.DeterministicEngine{}}
	d := r.Route(context.Background(), domain.StageExecute, Config{}, []Candidate{
		{Harness: domain.HarnessInstallation{ID: "a", Health: domain.HarnessIncompatible, Compatibility: domain.CompatIncompatible}},
	}, nil)
	if d.Blocked != domain.BlockedNoViableRoute {
		t.Fatalf("blocked = %s", d.Blocked)
	}
}

func TestForcedImpossibleRejected(t *testing.T) {
	r := Router{}
	cands := []Candidate{{
		Harness: domain.HarnessInstallation{ID: "open", DisplayName: "OpenCode", Health: domain.HarnessReady, Compatibility: domain.CompatRoutable},
		ModelID: "gpt",
	}}
	d := r.Route(context.Background(), domain.StagePlan, Config{ForceModel: "nope"}, cands, nil)
	if d.Blocked != domain.BlockedNoViableRoute {
		t.Fatal("expected reject forced impossible model")
	}
}

func TestUnauthNotRoutable(t *testing.T) {
	r := Router{}
	d := r.Route(context.Background(), domain.StageExecute, Config{}, []Candidate{{
		Harness: domain.HarnessInstallation{ID: "x", Health: domain.HarnessUnauth, Compatibility: domain.CompatRoutable},
	}}, nil)
	if d.Blocked != domain.BlockedNoViableRoute {
		t.Fatal("unauthenticated harness must not route")
	}
}

func TestJevNeverChoosesImpossible(t *testing.T) {
	r := Router{Engine: jev.DeterministicEngine{}}
	good := Candidate{Harness: domain.HarnessInstallation{ID: "ok", DisplayName: "ok", Health: domain.HarnessReady, Compatibility: domain.CompatRoutable}, ModelID: "m"}
	bad := Candidate{Harness: domain.HarnessInstallation{ID: "bad", Health: domain.HarnessUnavailable, Compatibility: domain.CompatIncompatible}}
	d := r.Route(context.Background(), domain.StagePlan, Config{Profile: domain.ProfileAuto}, []Candidate{bad, good}, &jev.Assessment{Degraded: true})
	if d.Blocked != "" {
		t.Fatal(d.Detail)
	}
	if d.Candidate.Harness.ID != "ok" {
		t.Fatalf("picked %s", d.Candidate.Harness.ID)
	}
}
