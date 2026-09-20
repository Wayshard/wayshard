//go:build linux

package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/Wayshard/wayshard/internal/domain"
	"github.com/Wayshard/wayshard/internal/storage"
	"github.com/Wayshard/wayshard/internal/testutil"
	"github.com/Wayshard/wayshard/internal/workspace"
)

// ---------------------------------------------------------------------------
// Process-boundary harness: a real compiled Wayshard server process, a real
// deterministic ACP fixture process, durable on-disk state, and real HTTP.
// ---------------------------------------------------------------------------

type procServer struct {
	t       *testing.T
	cmd     *exec.Cmd
	base    string
	dataDir string
	logFile string
}

func buildServerBinary(t *testing.T) string {
	t.Helper()
	out := filepath.Join(t.TempDir(), testutil.ExeName("wayshard-server"))
	cmd := exec.Command("go", "build", "-o", out, "./cmd/wayshard-server")
	cmd.Dir = testutil.ModuleRoot(t)
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build wayshard-server: %v\n%s", err, b)
	}
	return out
}

func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

func startServer(t *testing.T, bin, dataDir string, port int, extraEnv []string) *procServer {
	t.Helper()
	logFile := filepath.Join(t.TempDir(), fmt.Sprintf("server-%d.log", port))
	f, err := os.Create(logFile)
	if err != nil {
		t.Fatal(err)
	}
	base := fmt.Sprintf("http://127.0.0.1:%d", port)
	cmd := exec.Command(bin, "--listen", fmt.Sprintf("127.0.0.1:%d", port), "--data", dataDir)
	cmd.Env = append(os.Environ(), extraEnv...)
	cmd.Stdout = f
	cmd.Stderr = f
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	ps := &procServer{t: t, cmd: cmd, base: base, dataDir: dataDir, logFile: logFile}
	t.Cleanup(func() {
		if ps.cmd.Process != nil {
			_ = ps.cmd.Process.Kill()
		}
		_ = ps.cmd.Wait()
	})
	// Recovery runs synchronously in app.Open before the listener binds, so a
	// successful health check implies startup recovery has completed.
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(base + "/healthz")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == 200 {
				return ps
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	b, _ := os.ReadFile(logFile)
	t.Fatalf("server %d never became healthy\n%s", port, b)
	return nil
}

func (p *procServer) pid() int { return p.cmd.Process.Pid }

func (p *procServer) kill(t *testing.T) {
	t.Helper()
	if p.cmd.Process == nil {
		return
	}
	// SIGKILL: not a graceful shutdown.
	_ = p.cmd.Process.Kill()
	_ = p.cmd.Wait()
}

func (p *procServer) stopGraceful(t *testing.T) {
	t.Helper()
	if p.cmd.Process == nil {
		return
	}
	_ = p.cmd.Process.Signal(os.Interrupt)
	done := make(chan struct{})
	go func() { _ = p.cmd.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		_ = p.cmd.Process.Kill()
		<-done
	}
}

func (p *procServer) logs(t *testing.T) string {
	b, _ := os.ReadFile(p.logFile)
	return string(b)
}

func httpJSON(t *testing.T, method, url, body, cred string) (int, []byte) {
	t.Helper()
	var r io.Reader
	if body != "" {
		r = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, url, r)
	if err != nil {
		t.Fatal(err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if cred != "" {
		req.Header.Set("Authorization", "Bearer "+cred)
	}
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, b
}

func pairProcessServer(t *testing.T, ps *procServer) string {
	t.Helper()
	code, body := httpJSON(t, http.MethodPost, ps.base+"/v1/pairing/invitations", `{}`, "")
	if code != 200 {
		t.Fatalf("invitation %d %s", code, body)
	}
	var inv struct {
		Code string `json:"code"`
	}
	_ = json.Unmarshal(body, &inv)
	req, _ := json.Marshal(map[string]string{"code": inv.Code, "deviceName": "test", "deviceKind": "cli"})
	code, body = httpJSON(t, http.MethodPost, ps.base+"/v1/pairing/complete", string(req), "")
	if code != 200 {
		t.Fatalf("pair complete %d %s", code, body)
	}
	var res struct {
		Credential string `json:"credential"`
	}
	_ = json.Unmarshal(body, &res)
	if res.Credential == "" {
		t.Fatal("no credential")
	}
	return res.Credential
}

func openProjectViaAPI(t *testing.T, ps *procServer, cred, dir, name string) string {
	t.Helper()
	req, _ := json.Marshal(map[string]string{"path": dir, "name": name})
	code, body := httpJSON(t, http.MethodPost, ps.base+"/v1/projects/open", string(req), cred)
	if code != 200 && code != 201 {
		t.Fatalf("open project %d %s", code, body)
	}
	var p struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(body, &p)
	return p.ID
}

func createConversation(t *testing.T, ps *procServer, cred, projectID string) string {
	t.Helper()
	code, body := httpJSON(t, http.MethodPost, ps.base+"/v1/projects/"+projectID+"/conversations", `{"title":"recovery"}`, cred)
	if code != 200 && code != 201 {
		t.Fatalf("create conversation %d %s", code, body)
	}
	var c struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(body, &c)
	return c.ID
}

func sendMessage(t *testing.T, ps *procServer, cred, convID, text string) string {
	t.Helper()
	req, _ := json.Marshal(map[string]any{"text": text, "artifactOnly": false})
	r, _ := http.NewRequest(http.MethodPost, ps.base+"/v1/conversations/"+convID+"/messages", bytes.NewReader(req))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Authorization", "Bearer "+cred)
	r.Header.Set("Idempotency-Key", "proc-"+convID)
	resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(r)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 202 {
		t.Fatalf("send message %d %s", resp.StatusCode, b)
	}
	var out struct {
		Run struct {
			ID string `json:"id"`
		} `json:"run"`
	}
	_ = json.Unmarshal(b, &out)
	if out.Run.ID == "" {
		t.Fatalf("no run in response: %s", b)
	}
	return out.Run.ID
}

func getRunStatus(t *testing.T, ps *procServer, cred, runID string) domain.Run {
	t.Helper()
	code, body := httpJSON(t, http.MethodGet, ps.base+"/v1/runs/"+runID, "", cred)
	if code != 200 {
		t.Fatalf("get run %d %s", code, body)
	}
	var r domain.Run
	_ = json.Unmarshal(body, &r)
	return r
}

func waitRun(t *testing.T, ps *procServer, cred, runID string, pred func(domain.Run) bool, timeout time.Duration) domain.Run {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var last domain.Run
	for time.Now().Before(deadline) {
		last = getRunStatus(t, ps, cred, runID)
		if pred(last) {
			return last
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("run %s did not settle (last=%s %s):\n%s", runID, last.Status, last.BlockedDetail, ps.logs(t))
	return last
}

func waitFile(t *testing.T, path string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("file %s never appeared", path)
}

// waitFileRun waits for the fixture signal file while reporting run progress on
// failure.
func waitFileRun(t *testing.T, ps *procServer, cred, runID, path string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var last domain.Run
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		last = getRunStatus(t, ps, cred, runID)
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("file %s never appeared; run=%s %s\n%s", path, last.Status, last.BlockedDetail, ps.logs(t))
}

// reapFixture kills the hung ACP fixture process recorded in <signalFile>.pid.
// SIGKILLing the server does not terminate its children, so the test owns
// cleanup of the deliberately-hung fixture.
func reapFixture(t *testing.T, signalFile string) {
	t.Helper()
	b, err := os.ReadFile(signalFile + ".pid")
	if err != nil {
		return
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil || pid <= 0 {
		return
	}
	_ = os.Remove(signalFile + ".pid")
	p, err := os.FindProcess(pid)
	if err != nil {
		return
	}
	_ = p.Kill()
	for i := 0; i < 100; i++ {
		if err := p.Signal(syscall.Signal(0)); err != nil {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// withStore opens the durable store in this test process. Only used while no
// Wayshard server process is running.
func withStore(t *testing.T, dataDir string, fn func(*storage.Store)) {
	t.Helper()
	ctx := context.Background()
	st, err := storage.Open(ctx, dataDir)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	fn(st)
}

func runWorkspacePath(dataDir, runID string) string {
	return filepath.Join(dataDir, "runtime", "workspaces", runID, "run")
}

func hashBytes(b []byte) string { return workspace.HashBytes(b) }

// fixtureEnv builds the deterministic crash-fixture environment.
type fixtureEnv struct {
	fakeBin      string
	signalDir    string
	signalFile   string
	markerFile   string
	hangStage    string
	partialFile  string
	trackedFile  string
	toolFile     string
	reviewReject bool
}

func (f fixtureEnv) serverEnv() []string {
	env := []string{
		"PATH=" + filepath.Dir(f.fakeBin) + string(os.PathListSeparator) + os.Getenv("PATH"),
		"WAYSHARD_FAKE_SCENARIO=hang_after_write",
		"WAYSHARD_FAKE_HANG_STAGE=" + f.hangStage,
		"WAYSHARD_FAKE_WRITE_FILE=agent.go",
		"WAYSHARD_FAKE_TOOL_WRITE_FILE=" + f.toolFile,
		"WAYSHARD_FAKE_PARTIAL_TRACKED=" + f.trackedFile,
		"WAYSHARD_FAKE_PARTIAL_FILE=" + f.partialFile,
		"WAYSHARD_FAKE_SIGNAL_FILE=" + f.signalFile,
		"WAYSHARD_FAKE_SUCCESS_MARKER=" + f.markerFile,
		"WAYSHARD_HARNESS_EXTRA_ROOTS=" + f.signalDir,
	}
	if f.reviewReject {
		env = append(env, "WAYSHARD_FAKE_REVIEW_REJECT=1")
	}
	return env
}

// ---------------------------------------------------------------------------
// Executor crash + recovery across a real server SIGKILL and new process.
// ---------------------------------------------------------------------------

func TestProcessBoundaryExecutorCrashRecovery(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	requireGit(t)
	serverBin := buildServerBinary(t)
	fakeBin := testutil.BuildFakeACP(t)

	root := t.TempDir()
	signalDir := filepath.Join(root, "signal")
	if err := os.MkdirAll(signalDir, 0o755); err != nil {
		t.Fatal(err)
	}
	dataDir := filepath.Join(root, "data")
	src := filepath.Join(root, "src")
	initRepo(t, src, map[string]string{"tracked.txt": "source-original", "keep.go": "package keep\n"})
	// Pre-existing user dirty baseline.
	if err := os.WriteFile(filepath.Join(src, "tracked.txt"), []byte("source-user-dirty"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "user-dirty.txt"), []byte("dirty baseline\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	baselineTracked, _ := os.ReadFile(filepath.Join(src, "tracked.txt"))

	fx := fixtureEnv{
		fakeBin:     fakeBin,
		signalDir:   signalDir,
		signalFile:  filepath.Join(signalDir, "execute.signal"),
		markerFile:  filepath.Join(signalDir, "success.marker"),
		hangStage:   "execute",
		partialFile: "partial.txt",
		trackedFile: "tracked.txt",
		toolFile:    "tool-partial.txt",
	}
	t.Cleanup(func() { reapFixture(t, fx.signalFile) })

	// --- Server A ---------------------------------------------------------
	portA := freePort(t)
	psA := startServer(t, serverBin, dataDir, portA, fx.serverEnv())
	pidA := psA.pid()
	cred := pairProcessServer(t, psA)
	proj := openProjectViaAPI(t, psA, cred, src, "recovery")
	conv := createConversation(t, psA, cred, proj)
	runID := sendMessage(t, psA, cred, conv, "add agent.go")

	waitFile(t, fx.signalFile, 30*time.Second)
	rw := runWorkspacePath(dataDir, runID)

	// The crash fixture made all three write mechanisms durable before hanging.
	if got, _ := os.ReadFile(filepath.Join(rw, "tracked.txt")); string(got) != "partial-execute\n" {
		t.Fatalf("harness direct write not durable: %q", got)
	}
	if _, err := os.Stat(filepath.Join(rw, "partial.txt")); err != nil {
		t.Fatalf("harness partial file missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(rw, "tool-partial.txt")); err != nil {
		t.Fatalf("ACP Tool callback write missing: %v", err)
	}

	// A real v3 pre-attempt checkpoint must exist before the crash.
	var cpTreePath, cpHash string
	var cpAttemptID string
	withStore(t, dataDir, func(st *storage.Store) {
		ctx := context.Background()
		cps, err := st.ListCheckpointsByRun(ctx, runID)
		if err != nil || len(cps) == 0 {
			t.Fatalf("no checkpoint before crash: %v", err)
		}
		last := cps[len(cps)-1]
		if last.HashVersion != 3 {
			t.Fatalf("checkpoint hash_version=%d, want 3", last.HashVersion)
		}
		cpTreePath, cpHash, cpAttemptID = last.TreePath, last.TreeHash, last.AttemptID
		got, err := workspace.CanonicalTreeHashV3(filepath.Join(last.TreePath, "tree"))
		if err != nil || got != last.TreeHash {
			t.Fatalf("persisted checkpoint digest invalid: %v %s %s", err, got, last.TreeHash)
		}
		if got, _ := os.ReadFile(filepath.Join(last.TreePath, "tree", "tracked.txt")); string(got) != string(baselineTracked) {
			t.Fatalf("checkpoint baseline wrong: %q", got)
		}
	})
	if cpAttemptID == "" || cpTreePath == "" || cpHash == "" {
		t.Fatal("checkpoint metadata incomplete")
	}

	// --- SIGKILL server A -------------------------------------------------
	psA.kill(t)
	reapFixture(t, fx.signalFile)

	// --- Live source edit while the server is down ------------------------
	editBody := []byte("edited while wayshard was dead\n")
	if err := os.WriteFile(filepath.Join(src, "source-user-edit.txt"), editBody, 0o644); err != nil {
		t.Fatal(err)
	}
	editHash := hashBytes(editBody)
	if err := os.WriteFile(fx.markerFile, []byte("go\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// --- Server B ---------------------------------------------------------
	portB := freePort(t)
	psB := startServer(t, serverBin, dataDir, portB, fx.serverEnv())
	pidB := psB.pid()
	if pidA == pidB {
		t.Fatalf("expected distinct server PIDs, got %d == %d", pidA, pidB)
	}
	t.Logf("executor crash: PID A=%d PID B=%d", pidA, pidB)
	credB := cred // same durable device credential

	// Recovery already ran before the listener bound. The partial RunWorkspace
	// must be replaced with the verified checkpoint, and the live source must be
	// untouched by recovery.
	restoreDeadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(restoreDeadline) {
		body, err := os.ReadFile(filepath.Join(rw, "tracked.txt"))
		if err == nil && string(body) == string(baselineTracked) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if got, _ := os.ReadFile(filepath.Join(rw, "tracked.txt")); string(got) != string(baselineTracked) {
		t.Fatalf("run workspace not restored: %q", got)
	}
	if _, err := os.Stat(filepath.Join(rw, "partial.txt")); err == nil {
		t.Fatal("partial.txt survived checkpoint restore")
	}
	if _, err := os.Stat(filepath.Join(rw, "tool-partial.txt")); err == nil {
		t.Fatal("ACP Tool callback partial write survived checkpoint restore")
	}
	gotEdit, err := os.ReadFile(filepath.Join(src, "source-user-edit.txt"))
	if err != nil || hashBytes(gotEdit) != editHash {
		t.Fatalf("recovery disturbed live source edit: %v %q", err, gotEdit)
	}

	// --- Retry completes from the restored workspace ----------------------
	final := waitRun(t, psB, credB, runID, func(r domain.Run) bool {
		return r.Status.Terminal() || r.Status == domain.RunIntegrationBlocked
	}, 90*time.Second)
	if final.Status != domain.RunComplete {
		t.Fatalf("run did not complete: %s %s\n%s", final.Status, final.BlockedDetail, psB.logs(t))
	}

	// Live source edit preserved and agent change published.
	if got, _ := os.ReadFile(filepath.Join(src, "source-user-edit.txt")); hashBytes(got) != editHash {
		t.Fatal("live source edit lost through integration")
	}
	if got, _ := os.ReadFile(filepath.Join(src, "tracked.txt")); string(got) != string(baselineTracked) {
		t.Fatalf("pre-existing dirty baseline changed: %q", got)
	}
	if _, err := os.Stat(filepath.Join(src, "agent.go")); err != nil {
		t.Fatalf("agent.go not integrated: %v", err)
	}
	if _, err := os.Stat(filepath.Join(src, "partial.txt")); err == nil {
		t.Fatal("partial.txt leaked into live source")
	}

	psB.kill(t)

	// --- Durable evidence -------------------------------------------------
	withStore(t, dataDir, func(st *storage.Store) {
		ctx := context.Background()
		stages, _ := st.ListStages(ctx, runID)
		oldAttemptInterrupted := false
		newExecuteSucceeded := false
		var oldID, newID string
		executeStages := 0
		for _, stg := range stages {
			if stg.Kind != domain.StageExecute {
				continue
			}
			executeStages++
			atts, _ := st.ListAttempts(ctx, stg.ID)
			for _, a := range atts {
				if a.Status == domain.AttemptInterrupted {
					oldAttemptInterrupted = true
					oldID = a.ID
				}
				if a.Status == domain.AttemptSucceeded {
					newExecuteSucceeded = true
					newID = a.ID
				}
			}
		}
		if executeStages < 2 {
			t.Fatalf("expected a retried execute stage, got %d", executeStages)
		}
		if !oldAttemptInterrupted {
			t.Fatal("crashed execute attempt was not recorded INTERRUPTED")
		}
		if !newExecuteSucceeded {
			t.Fatal("retried execute attempt did not succeed")
		}
		if oldID == newID || oldID == "" || newID == "" {
			t.Fatalf("attempt lineage wrong: old=%s new=%s", oldID, newID)
		}

		evs, _ := st.EventsSince(ctx, 0, "", runID, 10000)
		restoredSeq := int64(-1)
		for _, e := range evs {
			if e.Type == "checkpoint.restored" {
				restoredSeq = e.Seq
			}
		}
		if restoredSeq < 0 {
			t.Fatal("no checkpoint.restored event")
		}
		// The retried execute stage must be appended only after recovery.
		retryStageSeq := int64(-1)
		for _, e := range evs {
			if e.Type != "stage.appended" || e.Seq <= restoredSeq {
				continue
			}
			var p struct {
				Kind string `json:"kind"`
			}
			_ = json.Unmarshal([]byte(e.Payload), &p)
			if p.Kind == string(domain.StageExecute) {
				retryStageSeq = e.Seq
				break
			}
		}
		if retryStageSeq < 0 {
			t.Fatal("no execute stage appended after recovery")
		}
		if retryStageSeq <= restoredSeq {
			t.Fatal("scheduler dispatched before checkpoint restore committed")
		}

		deltaJSON, err := st.LatestRunDelta(ctx, runID)
		if err != nil {
			t.Fatalf("no run delta: %v", err)
		}
		var delta workspace.Delta
		if err := json.Unmarshal([]byte(deltaJSON), &delta); err != nil {
			t.Fatal(err)
		}
		foundAgent := false
		for _, f := range delta.Files {
			if f.Path == "partial.txt" || f.Path == "tool-partial.txt" {
				t.Fatalf("crashed partial write in run delta: %s", f.Path)
			}
			if f.Path == "tracked.txt" && f.AgentModified {
				t.Fatal("crashed tracked.txt mutation attributed to agent")
			}
			if f.Path == "agent.go" {
				foundAgent = true
			}
		}
		if !foundAgent {
			t.Fatalf("run delta missing agent.go: %+v", delta.Files)
		}
	})
}

// TestProcessBoundaryRecoveredIntegrationConflictBlocks proves that a
// compatible user edit made while the server was down survives, while a
// conflicting edit blocks integration after recovered execution.
func TestProcessBoundaryRecoveredIntegrationConflictBlocks(t *testing.T) {
	requireGit(t)
	serverBin := buildServerBinary(t)
	fakeBin := testutil.BuildFakeACP(t)

	root := t.TempDir()
	signalDir := filepath.Join(root, "signal")
	_ = os.MkdirAll(signalDir, 0o755)
	dataDir := filepath.Join(root, "data")
	src := filepath.Join(root, "src")
	initRepo(t, src, map[string]string{"tracked.txt": "source-original", "keep.go": "package keep\n"})

	fx := fixtureEnv{
		fakeBin:     fakeBin,
		signalDir:   signalDir,
		signalFile:  filepath.Join(signalDir, "execute.signal"),
		markerFile:  filepath.Join(signalDir, "success.marker"),
		hangStage:   "execute",
		partialFile: "partial.txt",
		trackedFile: "tracked.txt",
		toolFile:    "tool-partial.txt",
	}
	t.Cleanup(func() { reapFixture(t, fx.signalFile) })

	portA := freePort(t)
	psA := startServer(t, serverBin, dataDir, portA, fx.serverEnv())
	cred := pairProcessServer(t, psA)
	proj := openProjectViaAPI(t, psA, cred, src, "conflict")
	conv := createConversation(t, psA, cred, proj)
	runID := sendMessage(t, psA, cred, conv, "add agent.go")
	waitFile(t, fx.signalFile, 30*time.Second)

	psA.kill(t)
	reapFixture(t, fx.signalFile)

	// Compatible edit + conflicting agent.go while the server is down.
	editBody := []byte("edited while down\n")
	if err := os.WriteFile(filepath.Join(src, "source-user-edit.txt"), editBody, 0o644); err != nil {
		t.Fatal(err)
	}
	conflictBody := []byte("package user\n// concurrent conflicting add\n")
	if err := os.WriteFile(filepath.Join(src, "agent.go"), conflictBody, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fx.markerFile, []byte("go\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	portB := freePort(t)
	psB := startServer(t, serverBin, dataDir, portB, fx.serverEnv())
	defer psB.kill(t)
	if psA.pid() == psB.pid() {
		t.Fatalf("expected distinct server PIDs, got %d == %d", psA.pid(), psB.pid())
	}
	t.Logf("recovered conflict: PID A=%d PID B=%d", psA.pid(), psB.pid())

	final := waitRun(t, psB, cred, runID, func(r domain.Run) bool {
		return r.Status.Terminal() || r.Status == domain.RunIntegrationBlocked
	}, 90*time.Second)
	if final.Status != domain.RunIntegrationBlocked {
		t.Fatalf("expected integration_blocked, got %s %s\n%s", final.Status, final.BlockedDetail, psB.logs(t))
	}
	// The conflicting user file is never overwritten and the compatible edit is
	// preserved.
	if got, _ := os.ReadFile(filepath.Join(src, "agent.go")); string(got) != string(conflictBody) {
		t.Fatalf("conflicting source overwritten: %q", got)
	}
	if got, _ := os.ReadFile(filepath.Join(src, "source-user-edit.txt")); hashBytes(got) != hashBytes(editBody) {
		t.Fatal("compatible user edit lost")
	}
	// The validated run result is preserved (not lost) on conflict.
	rw := runWorkspacePath(dataDir, runID)
	if got, _ := os.ReadFile(filepath.Join(rw, "agent.go")); !strings.Contains(string(got), "fake ACP") {
		t.Fatalf("validated run result lost on conflict: %q", got)
	}
}

// ---------------------------------------------------------------------------
// Repair crash + recovery across a real server SIGKILL and new process.
// ---------------------------------------------------------------------------

func TestProcessBoundaryRepairCrashRecovery(t *testing.T) {
	requireGit(t)
	serverBin := buildServerBinary(t)
	fakeBin := testutil.BuildFakeACP(t)

	root := t.TempDir()
	signalDir := filepath.Join(root, "signal")
	_ = os.MkdirAll(signalDir, 0o755)
	dataDir := filepath.Join(root, "data")
	src := filepath.Join(root, "src")
	initRepo(t, src, map[string]string{"tracked.txt": "source-original", "keep.go": "package keep\n"})

	fx := fixtureEnv{
		fakeBin:      fakeBin,
		signalDir:    signalDir,
		signalFile:   filepath.Join(signalDir, "repair.signal"),
		markerFile:   filepath.Join(signalDir, "success.marker"),
		hangStage:    "repair",
		partialFile:  "repair-partial.txt",
		trackedFile:  "agent.go",
		toolFile:     "tool-repair-partial.txt",
		reviewReject: true,
	}
	t.Cleanup(func() { reapFixture(t, fx.signalFile) })

	portC := freePort(t)
	psC := startServer(t, serverBin, dataDir, portC, fx.serverEnv())
	pidC := psC.pid()
	cred := pairProcessServer(t, psC)
	proj := openProjectViaAPI(t, psC, cred, src, "repair")
	conv := createConversation(t, psC, cred, proj)
	runID := sendMessage(t, psC, cred, conv, "add agent.go")

	waitFileRun(t, psC, cred, runID, fx.signalFile, 60*time.Second)
	rw := runWorkspacePath(dataDir, runID)

	// The crashed repair attempt modified the candidate and left partials.
	if _, err := os.Stat(filepath.Join(rw, "repair-partial.txt")); err != nil {
		t.Fatalf("repair partial missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(rw, "tool-repair-partial.txt")); err != nil {
		t.Fatalf("ACP Tool repair partial missing: %v", err)
	}
	if got, _ := os.ReadFile(filepath.Join(rw, "agent.go")); string(got) != "partial-repair\n" {
		t.Fatalf("repair did not modify the candidate: %q", got)
	}

	// Candidate A is recovered from the pre-repair checkpoint, which captured
	// the executor output before repair touched it.
	var repairCPTree, repairCPHash string
	var candidateA []byte
	withStore(t, dataDir, func(st *storage.Store) {
		ctx := context.Background()
		cps, _ := st.ListCheckpointsByRun(ctx, runID)
		if len(cps) < 2 {
			t.Fatalf("expected executor + repair checkpoints, got %d", len(cps))
		}
		// Latest is the pre-repair checkpoint.
		cp := cps[len(cps)-1]
		if cp.HashVersion != 3 {
			t.Fatalf("repair checkpoint hash_version=%d", cp.HashVersion)
		}
		repairCPTree, repairCPHash = cp.TreePath, cp.TreeHash
		got, err := workspace.CanonicalTreeHashV3(filepath.Join(cp.TreePath, "tree"))
		if err != nil || got != cp.TreeHash {
			t.Fatalf("repair checkpoint digest invalid: %v", err)
		}
		candidateA, err = os.ReadFile(filepath.Join(cp.TreePath, "tree", "agent.go"))
		if err != nil || !strings.Contains(string(candidateA), "fake ACP") {
			t.Fatalf("candidate A not in pre-repair checkpoint: %v %q", err, candidateA)
		}
	})
	if repairCPTree == "" || repairCPHash == "" {
		t.Fatal("repair checkpoint metadata incomplete")
	}

	psC.kill(t)
	reapFixture(t, fx.signalFile)
	if err := os.WriteFile(fx.markerFile, []byte("go\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	portD := freePort(t)
	psD := startServer(t, serverBin, dataDir, portD, fx.serverEnv())
	pidD := psD.pid()
	if pidC == pidD {
		t.Fatalf("expected distinct server PIDs, got %d == %d", pidC, pidD)
	}
	t.Logf("repair crash: PID C=%d PID D=%d", pidC, pidD)

	restoreDeadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(restoreDeadline) {
		if _, err := os.Stat(filepath.Join(rw, "repair-partial.txt")); os.IsNotExist(err) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if _, err := os.Stat(filepath.Join(rw, "repair-partial.txt")); err == nil {
		t.Fatal("repair-partial.txt survived checkpoint restore")
	}
	if _, err := os.Stat(filepath.Join(rw, "tool-repair-partial.txt")); err == nil {
		t.Fatal("ACP Tool repair partial survived checkpoint restore")
	}
	restoredCandidate, _ := os.ReadFile(filepath.Join(rw, "agent.go"))
	if string(restoredCandidate) != string(candidateA) {
		t.Fatalf("candidate A not restored exactly: %q vs %q", restoredCandidate, candidateA)
	}

	final := waitRun(t, psD, cred, runID, func(r domain.Run) bool {
		return r.Status.Terminal() || r.Status == domain.RunIntegrationBlocked
	}, 90*time.Second)
	if final.Status != domain.RunComplete {
		t.Fatalf("repair run did not complete: %s %s\n%s", final.Status, final.BlockedDetail, psD.logs(t))
	}
	psD.kill(t)

	withStore(t, dataDir, func(st *storage.Store) {
		ctx := context.Background()
		stages, _ := st.ListStages(ctx, runID)
		var repairStages int
		oldInterrupted, newSucceeded := false, false
		var oldID, newID string
		for _, stg := range stages {
			if stg.Kind != domain.StageRepair {
				continue
			}
			repairStages++
			atts, _ := st.ListAttempts(ctx, stg.ID)
			for _, a := range atts {
				if a.Status == domain.AttemptInterrupted {
					oldInterrupted = true
					oldID = a.ID
				}
				if a.Status == domain.AttemptSucceeded {
					newSucceeded = true
					newID = a.ID
				}
			}
		}
		if repairStages < 2 {
			t.Fatalf("expected a retried repair stage, got %d", repairStages)
		}
		if !oldInterrupted || !newSucceeded {
			t.Fatalf("repair attempt lineage wrong: interrupted=%v succeeded=%v", oldInterrupted, newSucceeded)
		}
		if oldID == newID || oldID == "" || newID == "" {
			t.Fatalf("repair attempt ids not distinct: %s %s", oldID, newID)
		}
		evs, _ := st.EventsSince(ctx, 0, "", runID, 10000)
		restored := 0
		for _, e := range evs {
			if e.Type == "checkpoint.restored" {
				restored++
			}
		}
		if restored != 1 {
			t.Fatalf("expected exactly one repair checkpoint.restored, got %d", restored)
		}
	})
}

// ---------------------------------------------------------------------------
// Explicit cancellation is never reinterpreted as a crash-interrupted attempt.
// ---------------------------------------------------------------------------

func TestProcessBoundaryCancelledNeverResumes(t *testing.T) {
	requireGit(t)
	serverBin := buildServerBinary(t)
	fakeBin := testutil.BuildFakeACP(t)

	root := t.TempDir()
	signalDir := filepath.Join(root, "signal")
	_ = os.MkdirAll(signalDir, 0o755)
	dataDir := filepath.Join(root, "data")
	src := filepath.Join(root, "src")
	initRepo(t, src, map[string]string{"tracked.txt": "source-original", "keep.go": "package keep\n"})

	fx := fixtureEnv{
		fakeBin:     fakeBin,
		signalDir:   signalDir,
		signalFile:  filepath.Join(signalDir, "execute.signal"),
		markerFile:  filepath.Join(signalDir, "success.marker"),
		hangStage:   "execute",
		partialFile: "partial.txt",
		trackedFile: "tracked.txt",
		toolFile:    "tool-partial.txt",
	}
	t.Cleanup(func() { reapFixture(t, fx.signalFile) })

	portE := freePort(t)
	psE := startServer(t, serverBin, dataDir, portE, fx.serverEnv())
	pidE := psE.pid()
	cred := pairProcessServer(t, psE)
	proj := openProjectViaAPI(t, psE, cred, src, "cancel")
	conv := createConversation(t, psE, cred, proj)
	runID := sendMessage(t, psE, cred, conv, "add agent.go")
	waitFile(t, fx.signalFile, 30*time.Second)

	code, body := httpJSON(t, http.MethodPost, psE.base+"/v1/runs/"+runID+"/cancel", `{}`, cred)
	if code != 200 {
		t.Fatalf("cancel %d %s", code, body)
	}
	waitRun(t, psE, cred, runID, func(r domain.Run) bool { return r.Status == domain.RunCancelled }, 20*time.Second)

	psE.kill(t)
	reapFixture(t, fx.signalFile)
	if err := os.WriteFile(fx.markerFile, []byte("go\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	portF := freePort(t)
	psF := startServer(t, serverBin, dataDir, portF, fx.serverEnv())
	pidF := psF.pid()
	if pidE == pidF {
		t.Fatalf("expected distinct server PIDs, got %d == %d", pidE, pidF)
	}
	t.Logf("cancelled restart: PID E=%d PID F=%d", pidE, pidF)

	// Give any erroneous recovery/scheduler a chance to (incorrectly) act.
	time.Sleep(3 * time.Second)
	got := getRunStatus(t, psF, cred, runID)
	if got.Status != domain.RunCancelled {
		t.Fatalf("cancelled run changed after restart: %s", got.Status)
	}
	psF.kill(t)

	withStore(t, dataDir, func(st *storage.Store) {
		ctx := context.Background()
		stages, _ := st.ListStages(ctx, runID)
		for _, stg := range stages {
			atts, _ := st.ListAttempts(ctx, stg.ID)
			for _, a := range atts {
				if a.Status == domain.AttemptInterrupted {
					t.Fatal("cancelled attempt was rewritten to interrupted")
				}
				if a.Status == domain.AttemptRunning || a.Status == domain.AttemptPending {
					t.Fatalf("cancelled run left a running attempt: %s", a.Status)
				}
			}
		}
		// No retry stage was appended after cancellation.
		if len(stages) > 3 {
			t.Fatalf("cancelled run gained stages after restart: %d", len(stages))
		}
		evs, _ := st.EventsSince(ctx, 0, "", runID, 10000)
		for _, e := range evs {
			if e.Type == "checkpoint.restored" {
				t.Fatal("cancelled run was reactivated by checkpoint recovery")
			}
		}
	})
}
