package pty

import (
	"context"
	"runtime"
	"testing"
)

// TestKillTerminatesServerOwnedPTY verifies that closing a terminal terminates
// the server-owned session and is idempotent; a client detach never kills a PTY,
// but an explicit close does.
func TestKillTerminatesServerOwnedPTY(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("pty start is not supported on windows in this test")
	}
	m := New(nil)
	sess, err := m.Start(context.Background(), "prj_1", t.TempDir(), "dev_1")
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if _, ok := m.Attach(sess.ID); !ok {
		t.Fatalf("terminal %s not live after start", sess.ID)
	}
	if !m.Kill(sess.ID) {
		t.Fatalf("kill returned false for live terminal")
	}
	if _, ok := m.Attach(sess.ID); ok {
		t.Fatalf("terminal still live after kill")
	}
	if m.Kill(sess.ID) {
		t.Fatalf("second kill should return false")
	}
	if got := m.List("prj_1"); len(got) != 0 {
		t.Fatalf("expected no live terminals after kill, got %d", len(got))
	}
}

// TestListScopesToProject verifies multiple PTYs may coexist and listing is
// scoped by project, which is what the adapted multi-terminal panel relies on.
func TestListScopesToProject(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("pty start is not supported on windows in this test")
	}
	m := New(nil)
	a, err := m.Start(context.Background(), "prj_a", t.TempDir(), "dev_1")
	if err != nil {
		t.Fatalf("start a: %v", err)
	}
	b, err := m.Start(context.Background(), "prj_b", t.TempDir(), "dev_1")
	if err != nil {
		t.Fatalf("start b: %v", err)
	}
	defer m.Kill(a.ID)
	defer m.Kill(b.ID)

	if got := m.List("prj_a"); len(got) != 1 || got[0].ID != a.ID {
		t.Fatalf("prj_a list = %+v, want [%s]", got, a.ID)
	}
	if got := m.List(""); len(got) != 2 {
		t.Fatalf("unscoped list = %d, want 2", len(got))
	}
}
