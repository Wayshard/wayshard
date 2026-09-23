package app

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Wayshard/wayshard/internal/domain"
	"github.com/Wayshard/wayshard/internal/testutil"
)

// TestCancellationInterruptsActiveHarness proves cancellation propagates to the
// active harness process and yields a cancelled run instead of hanging.
func TestCancellationInterruptsActiveHarness(t *testing.T) {
	// Cancellation propagates through an actually-running harness, so it needs a
	// platform that can launch one under required isolation (Linux).
	testutil.RequireNativeIsolation(t)
	bin := buildFakeACP(t)
	t.Setenv("PATH", filepath.Dir(bin)+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("WAYSHARD_FAKE_SCENARIO", "timeout")

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	a, err := Open(ctx, Config{DataDir: t.TempDir(), Listen: "127.0.0.1:0"})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	a.Sched.Start(ctx)

	p := &domain.Project{Name: "p", Path: t.TempDir(), SourceKind: "filesystem"}
	if err := a.Store.InsertProject(ctx, p); err != nil {
		t.Fatal(err)
	}
	c := &domain.Conversation{ProjectID: p.ID, Title: "t"}
	_ = a.Store.InsertConversation(ctx, c)
	msg := &domain.Message{ConversationID: c.ID, Role: domain.RoleUser, Body: "hang"}
	task := &domain.Task{ProjectID: p.ID, ConversationID: c.ID, Objective: "hang"}
	run := &domain.Run{ProjectID: p.ID, ConversationID: c.ID, Profile: domain.ProfileAuto}
	if err := a.Store.CreateTaskRun(ctx, msg, task, run); err != nil {
		t.Fatal(err)
	}
	a.Sched.Enqueue(run.ID)

	// Wait until the harness is actually running.
	deadline := time.Now().Add(30 * time.Second)
	running := false
	for time.Now().Before(deadline) {
		r, err := a.Store.GetRun(ctx, run.ID)
		if err == nil && (r.Status == domain.RunPlanning || r.Status == domain.RunExecuting) {
			running = true
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !running {
		r, _ := a.Store.GetRun(ctx, run.ID)
		t.Fatalf("run never started: %+v", r)
	}

	start := time.Now()
	if !a.Sched.Cancel(run.ID) {
		t.Fatal("scheduler did not find an active execution to cancel")
	}
	// Cancellation must propagate to the active harness and yield a cancelled
	// run. The bound is deliberately generous: on a heavily loaded CI runner the
	// harness shutdown can take tens of seconds, and the property under test is
	// that cancellation terminates rather than hangs.
	cancelDeadline := time.Now().Add(120 * time.Second)
	for time.Now().Before(cancelDeadline) {
		r, err := a.Store.GetRun(ctx, run.ID)
		if err == nil && r.Status.Terminal() {
			if r.Status != domain.RunCancelled {
				t.Fatalf("expected cancelled, got %s", r.Status)
			}
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	r, _ := a.Store.GetRun(ctx, run.ID)
	if r.Status != domain.RunCancelled {
		t.Fatalf("run not cancelled after cancel: %+v", r)
	}
	if elapsed := time.Since(start); elapsed > 120*time.Second {
		t.Fatalf("cancellation took too long: %s", elapsed)
	}
	// No orphaned fake harness should remain (allow a moment for teardown).
	// Match this test's built harness path specifically so a concurrently
	// running test's fake harness cannot be mistaken for an orphan.
	orphanDeadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(orphanDeadline) {
		out, _ := exec.Command("pgrep", "-f", bin).Output()
		if strings.TrimSpace(string(out)) == "" {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	out, _ := exec.Command("pgrep", "-f", bin).Output()
	t.Fatalf("orphaned harness process remains: %s", out)
}
