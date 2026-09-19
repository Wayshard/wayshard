//go:build linux

package validation

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/Wayshard/wayshard/internal/artifacts"
	"github.com/Wayshard/wayshard/internal/domain"
)

// TestValidationCancellationKillsDescendants proves a cancelled validation
// check terminates its child/grandchild process tree.
func TestValidationCancellationKillsDescendants(t *testing.T) {
	ws := t.TempDir()
	script := "#!/bin/sh\nsleep 300 >/dev/null 2>&1 &\necho $! > gc.pid\nwait\n"
	if err := os.WriteFile(filepath.Join(ws, "slow.sh"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	r := &Runner{DataDir: t.TempDir()}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		r.Run(ctx, ws, []artifacts.ValidationCheck{{Name: "slow", Command: "./slow.sh", Required: true, Status: string(domain.CheckNotVerified)}}, nil)
		close(done)
	}()
	var pid string
	for i := 0; i < 200; i++ {
		if b, err := os.ReadFile(filepath.Join(ws, "gc.pid")); err == nil {
			pid = strings.TrimSpace(string(b))
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if pid == "" {
		cancel()
		<-done
		t.Fatal("grandchild never started")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(15 * time.Second):
		t.Fatal("validation did not return after cancellation")
	}
	n, err := strconv.Atoi(pid)
	if err != nil {
		t.Fatalf("bad pid %q", pid)
	}
	for i := 0; i < 50; i++ {
		if err := syscall.Kill(n, 0); err != nil {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("grandchild %s survived validation cancellation", pid)
}
