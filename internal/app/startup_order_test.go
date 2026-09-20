package app

import (
	"context"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"

	"github.com/Wayshard/wayshard/internal/domain"
	"github.com/Wayshard/wayshard/internal/process"
	"github.com/Wayshard/wayshard/internal/storage"
)

// TestStartupOrderingRecoveryBeforeDiscovery proves startup recovery (including
// stale probe reconciliation) completes before harness discovery/probing, and
// that a stale owned probe process is terminated during recovery.
func TestStartupOrderingRecoveryBeforeDiscovery(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	// Seed a stale probe ownership record and a surviving process carrying its
	// token, as a previous server crash would leave.
	st, err := storage.Open(ctx, dataDir)
	if err != nil {
		t.Fatal(err)
	}
	token, err := process.NewToken()
	if err != nil {
		t.Fatal(err)
	}
	if err := st.InsertProbeOwner(ctx, &domain.ProbeOwner{
		Kind: "version", TokenHash: process.HashToken(token), State: domain.ProcessOwnerActive,
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command("/bin/sh", "-c", "while true; do sleep 1; done")
	cmd.Env = append(os.Environ(), process.TokenEnv+"="+token)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	stalePid := cmd.Process.Pid
	reaped := make(chan error, 1)
	go func() { reaped <- cmd.Wait() }()
	t.Cleanup(func() {
		_ = syscall.Kill(stalePid, syscall.SIGKILL)
		select {
		case <-reaped:
		case <-time.After(5 * time.Second):
		}
	})
	time.Sleep(100 * time.Millisecond)
	if syscall.Kill(stalePid, 0) != nil {
		t.Fatal("stale process not alive before startup")
	}

	var steps []string
	startupObserver = func(step string) { steps = append(steps, step) }
	defer func() { startupObserver = nil }()

	a, err := Open(ctx, Config{DataDir: dataDir, Listen: "127.0.0.1:0", Log: log})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()

	// Discovery must not start before recovery completes.
	recoveryIdx, discoveryIdx := -1, -1
	for i, s := range steps {
		if s == "recovery" && recoveryIdx < 0 {
			recoveryIdx = i
		}
		if s == "discovery.start" && discoveryIdx < 0 {
			discoveryIdx = i
		}
	}
	if recoveryIdx < 0 || discoveryIdx < 0 {
		t.Fatalf("startup steps not observed: %v", steps)
	}
	if recoveryIdx > discoveryIdx {
		t.Fatalf("discovery ran before recovery: %v", steps)
	}

	// The stale probe process was terminated during recovery; once reaped it
	// must disappear.
	select {
	case <-reaped:
	case <-time.After(5 * time.Second):
		t.Fatal("stale probe process survived startup recovery")
	}

	// The probe owner record is reconciled.
	active, err := listActiveProbeOwners(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 0 {
		t.Fatalf("probe ownership not reconciled after startup: %+v", active)
	}
}

func listActiveProbeOwners(dataDir string) ([]domain.ProbeOwner, error) {
	ctx := context.Background()
	s, err := storage.Open(ctx, dataDir)
	if err != nil {
		return nil, err
	}
	defer s.Close()
	return s.ListActiveProbeOwners(ctx)
}
