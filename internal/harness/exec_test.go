package harness

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/Wayshard/wayshard/internal/domain"
	"github.com/Wayshard/wayshard/internal/orchestrator"
	"github.com/Wayshard/wayshard/internal/routing"
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
	out := filepath.Join(t.TempDir(), "wayshard-fake-acp")
	cmd := exec.Command("go", "build", "-o", out, "github.com/Wayshard/wayshard/cmd/wayshard-fake-acp")
	cmd.Dir = filepath.Join("..", "..")
	if wd, err := os.Getwd(); err == nil {
		cmd.Dir = filepath.Clean(filepath.Join(wd, "..", ".."))
	}
	b, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build fake: %v\n%s", err, b)
	}
	return out
}
