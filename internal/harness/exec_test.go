package harness

import (
	"context"
	"testing"

	"github.com/Wayshard/wayshard/internal/domain"
	"github.com/Wayshard/wayshard/internal/orchestrator"
	"github.com/Wayshard/wayshard/internal/routing"
	"github.com/Wayshard/wayshard/internal/testutil"
)

func TestACPExecPlanViaFakeHarness(t *testing.T) {
	bin := buildFake(t)
	ctx := context.Background()
	ex := &ACPExec{}
	res, err := ex.Execute(ctx, orchestrator.StageRequest{
		Task:  domain.Task{Objective: "plan a change"},
		Stage: domain.Stage{Kind: domain.StagePlan},
		Route: routing.Candidate{Harness: domain.HarnessInstallation{Executable: bin, Adapter: AdapterGeneric, DisplayName: "fake"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Err != nil {
		t.Fatal(res.Err)
	}
	if res.ArtifactJSON == "" {
		t.Fatal("empty artifact")
	}
}

func TestACPExecAuthRequired(t *testing.T) {
	bin := buildFake(t)
	t.Setenv("WAYSHARD_FAKE_SCENARIO", "auth_required")
	ex := &ACPExec{}
	res, err := ex.Execute(context.Background(), orchestrator.StageRequest{
		Task:  domain.Task{Objective: "x"},
		Stage: domain.Stage{Kind: domain.StagePlan},
		Route: routing.Candidate{Harness: domain.HarnessInstallation{Executable: bin, Adapter: AdapterGeneric}},
	})
	if err == nil && res.Err == nil {
		t.Fatal("expected auth required")
	}
}

func buildFake(t *testing.T) string {
	t.Helper()
	return testutil.BuildFakeACP(t)
}
