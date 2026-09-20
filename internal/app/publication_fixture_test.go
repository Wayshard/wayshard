//go:build linux

package app

import (
	"context"
	"os"
	"testing"

	"github.com/Wayshard/wayshard/internal/integration"
	"github.com/Wayshard/wayshard/internal/workspace"
)

// TestPublicationFixtureProcess is a helper process (not a real test). It runs
// the production integration package with a deterministic crash-after-one
// publication, records that it paused, then blocks until the controller kills
// it. It is spawned by TestProcessBoundaryPublicationPartialReconcile.
func TestPublicationFixtureProcess(t *testing.T) {
	if os.Getenv("WAYSHARD_PUB_FIXTURE") != "1" {
		t.Skip("publication fixture helper")
	}
	ctx := context.Background()
	b, err := workspace.Open(os.Getenv("PUB_SRC"))
	if err != nil {
		t.Fatal(err)
	}
	snap, err := workspace.LoadSnapshot(os.Getenv("PUB_SNAP"))
	if err != nil {
		t.Fatal(err)
	}
	res, err := integration.Integrate(ctx, integration.Request{
		RunID: os.Getenv("PUB_RUNID"), ProjectID: os.Getenv("PUB_PROJECT"),
		Source: b, Snapshot: snap, RunWorkspace: os.Getenv("PUB_RUN"),
		WorkDir: os.Getenv("PUB_WORK"), JournalDir: os.Getenv("PUB_JOURNAL"),
		CrashAfter: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(os.Getenv("PUB_READY"), []byte(res.Status), 0o644)
	select {} // pause until the controller terminates this process
}
