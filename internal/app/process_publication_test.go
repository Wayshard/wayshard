//go:build linux

package app

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/Wayshard/wayshard/internal/domain"
	"github.com/Wayshard/wayshard/internal/integration"
	"github.com/Wayshard/wayshard/internal/storage"
	"github.com/Wayshard/wayshard/internal/workspace"
)

// TestProcessBoundaryPublicationPartialReconcile proves a durable publication
// journal and partial source state produced by one OS process are reconciled by
// a fresh real server process on the same data/source.
func TestProcessBoundaryPublicationPartialReconcile(t *testing.T) {
	serverBin := buildServerBinary(t)
	root := t.TempDir()
	dataDir := filepath.Join(root, "data")
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(root, "src")
	_ = os.MkdirAll(src, 0o755)
	writeSrc := func(rel, body string) {
		if err := os.WriteFile(filepath.Join(src, rel), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	writeSrc("mod.txt", "old\n")
	writeSrc("del.txt", "gone\n")
	writeSrc("keep.txt", "keep\n")

	ctx := context.Background()
	st, err := storage.Open(ctx, dataDir)
	if err != nil {
		t.Fatal(err)
	}
	b, err := workspace.Open(src)
	if err != nil {
		t.Fatal(err)
	}
	snapDir := filepath.Join(root, "snap")
	snap, err := b.CaptureSnapshot(ctx, workspace.CaptureOptions{SnapshotDir: snapDir})
	if err != nil {
		t.Fatal(err)
	}
	runDir := filepath.Join(root, "run")
	if err := b.Materialize(ctx, snap, runDir); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(runDir, "mod.txt"), []byte("new\n"), 0o644)
	_ = os.WriteFile(filepath.Join(runDir, "added.txt"), []byte("added\n"), 0o644)
	_ = os.Remove(filepath.Join(runDir, "del.txt"))

	p := &domain.Project{Name: "pub", Path: src, SourceKind: b.Kind()}
	if err := st.InsertProject(ctx, p); err != nil {
		t.Fatal(err)
	}
	c := &domain.Conversation{ProjectID: p.ID, Title: "t"}
	_ = st.InsertConversation(ctx, c)
	msg := &domain.Message{ConversationID: c.ID, Role: domain.RoleUser, Body: "x"}
	task := &domain.Task{ProjectID: p.ID, ConversationID: c.ID, Objective: "x"}
	run := &domain.Run{ProjectID: p.ID, ConversationID: c.ID, Profile: domain.ProfileAuto}
	if err := st.CreateTaskRun(ctx, msg, task, run); err != nil {
		t.Fatal(err)
	}
	if err := st.UpdateRunStatus(ctx, run.ID, domain.RunIntegrating, "", ""); err != nil {
		t.Fatal(err)
	}
	rec := &domain.WorkspaceRecord{RunID: run.ID, ProjectID: p.ID, Kind: b.Kind(), SourcePath: src, RunPath: runDir}
	if err := st.InsertWorkspace(ctx, rec); err != nil {
		t.Fatal(err)
	}
	runID, projectID := run.ID, p.ID
	_ = st.Close()

	// Separate OS process performs the publication and pauses after one write.
	ready := filepath.Join(root, "ready")
	fixture := exec.Command(os.Args[0], "-test.run=TestPublicationFixtureProcess")
	fixture.Env = append(os.Environ(),
		"WAYSHARD_PUB_FIXTURE=1",
		"PUB_SRC="+src,
		"PUB_RUN="+runDir,
		"PUB_SNAP="+snapDir,
		"PUB_WORK="+filepath.Join(root, "work"),
		"PUB_JOURNAL="+filepath.Join(dataDir, "runtime", "journals", runID),
		"PUB_RUNID="+runID,
		"PUB_PROJECT="+projectID,
		"PUB_READY="+ready,
	)
	logFile, _ := os.Create(filepath.Join(root, "fixture.log"))
	fixture.Stdout = logFile
	fixture.Stderr = logFile
	if err := fixture.Start(); err != nil {
		t.Fatal(err)
	}
	fixturePID := fixture.Process.Pid
	waitFile(t, ready, 30*time.Second)

	// Partial state: added.txt published, mod.txt/del.txt still original.
	if body, _ := os.ReadFile(filepath.Join(src, "added.txt")); string(body) != "added\n" {
		t.Fatalf("first entry not published: %q", body)
	}
	if body, _ := os.ReadFile(filepath.Join(src, "mod.txt")); string(body) != "old\n" {
		t.Fatalf("mod.txt published too early: %q", body)
	}

	// Terminate the publication process.
	_ = fixture.Process.Kill()
	_ = fixture.Wait()

	portB := freePort(t)
	psB := startServer(t, serverBin, dataDir, portB, nil)
	if fixturePID == psB.pid() {
		t.Fatalf("fixture and server PIDs not distinct: %d", fixturePID)
	}
	t.Logf("publication process boundary: fixture PID=%d server B PID=%d", fixturePID, psB.pid())

	// Startup reconciliation must resume and finalize the publication.
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		mod, _ := os.ReadFile(filepath.Join(src, "mod.txt"))
		added, _ := os.ReadFile(filepath.Join(src, "added.txt"))
		_, delErr := os.Stat(filepath.Join(src, "del.txt"))
		if string(mod) == "new\n" && string(added) == "added\n" && os.IsNotExist(delErr) {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if body, _ := os.ReadFile(filepath.Join(src, "mod.txt")); string(body) != "new\n" {
		t.Fatalf("mod.txt not reconciled: %q", body)
	}
	if _, err := os.Stat(filepath.Join(src, "del.txt")); err == nil {
		t.Fatal("del.txt not deleted by reconciliation")
	}
	psB.kill(t)

	withStore(t, dataDir, func(st *storage.Store) {
		r, err := st.GetRun(context.Background(), runID)
		if err != nil || r.Status != domain.RunComplete {
			t.Fatalf("run = %+v err=%v, want complete", r, err)
		}
		in, err := st.GetIntegrationByRun(context.Background(), runID)
		if err != nil || in.Status != integration.StatusPublished {
			t.Fatalf("integration = %+v err=%v, want published", in, err)
		}
	})
}
