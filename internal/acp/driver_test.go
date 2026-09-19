package acp

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wayshard/wayshard/internal/artifacts"
	"github.com/Wayshard/wayshard/internal/domain"
	"github.com/Wayshard/wayshard/internal/testutil"
)

var fakeACP string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "wayshard-fake-acp")
	if err != nil {
		panic(err)
	}
	fakeACP = filepath.Join(dir, testutil.ExeName("wayshard-fake-acp"))
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	cmd := exec.Command("go", "build", "-o", fakeACP, "./cmd/wayshard-fake-acp")
	cmd.Dir = root
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		os.RemoveAll(dir)
		panic("build wayshard-fake-acp: " + err.Error())
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

func withFakeEnv(scenario, stage string, extra ...string) []string {
	override := map[string]string{
		"WAYSHARD_FAKE_SCENARIO": scenario,
		"WAYSHARD_FAKE_STAGE":    stage,
	}
	for i := 0; i+1 < len(extra); i += 2 {
		override[extra[i]] = extra[i+1]
	}
	env := os.Environ()
	out := make([]string, 0, len(env)+len(override))
	seen := map[string]struct{}{}
	for _, e := range env {
		k, _, _ := strings.Cut(e, "=")
		if v, ok := override[k]; ok {
			out = append(out, k+"="+v)
			seen[k] = struct{}{}
			continue
		}
		out = append(out, e)
	}
	for k, v := range override {
		if _, ok := seen[k]; !ok {
			out = append(out, k+"="+v)
		}
	}
	return out
}

func launchFake(t *testing.T, scenario, stage string, hooks Hooks, extraEnv ...string) *Driver {
	t.Helper()
	ctx := context.Background()
	d, err := Launch(ctx, Spec{
		Command: fakeACP,
		Env:     withFakeEnv(scenario, stage, extraEnv...),
	}, DefaultClientConfig(), hooks, Limits{HandshakeTimeout: 5 * time.Second, ShutdownWait: 500 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })
	if _, err := d.Handshake(ctx); err != nil {
		t.Fatal(err)
	}
	return d
}

func TestDriverSuccessPlan(t *testing.T) {
	var events []Event
	var mu sync.Mutex
	d := launchFake(t, "success", "plan", Hooks{
		OnEvent: func(e Event) {
			mu.Lock()
			events = append(events, e)
			mu.Unlock()
		},
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	sess, err := d.NewSession(ctx, NewSessionRequest{CWD: t.TempDir(), MCPServers: []MCPServer{}})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := d.Prompt(ctx, PromptRequest{
		SessionID: sess.SessionID,
		Prompt:    []ContentBlock{TextBlock("plan the work")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.StopReason != StopEndTurn {
		t.Fatalf("stopReason = %q", resp.StopReason)
	}
	got := d.MessageText()
	art, err := artifacts.ParseAndValidate(domain.ArtifactPlan, got)
	if err != nil {
		t.Fatalf("artifact: %v\n%s", err, got)
	}
	if art.(artifacts.PlanArtifact).Objective == "" {
		t.Fatal("empty objective")
	}
	mu.Lock()
	defer mu.Unlock()
	var sawDone, sawMsg bool
	for _, e := range events {
		if e.Kind == EventDone {
			sawDone = true
		}
		if e.Kind == EventMessageDelta {
			sawMsg = true
		}
	}
	if !sawDone || !sawMsg {
		t.Fatalf("events = %+v", events)
	}
}

func TestDriverSuccessExecuteAndReview(t *testing.T) {
	ctx := context.Background()
	for _, stage := range []string{"execute", "review"} {
		d := launchFake(t, "success", stage, Hooks{})
		sess, err := d.NewSession(ctx, NewSessionRequest{CWD: t.TempDir()})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := d.Prompt(ctx, PromptRequest{SessionID: sess.SessionID, Prompt: []ContentBlock{TextBlock(stage)}}); err != nil {
			t.Fatal(err)
		}
		kind := domain.ArtifactImplementation
		if stage == "review" {
			kind = domain.ArtifactReview
		}
		if _, err := artifacts.ParseAndValidate(kind, d.MessageText()); err != nil {
			t.Fatalf("%s: %v\n%s", stage, err, d.MessageText())
		}
	}
}

func TestDriverExploreRepairReviewReject(t *testing.T) {
	ctx := context.Background()
	d := launchFake(t, "explore", "plan", Hooks{})
	sess, err := d.NewSession(ctx, NewSessionRequest{CWD: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.Prompt(ctx, PromptRequest{SessionID: sess.SessionID, Prompt: []ContentBlock{TextBlock("explore")}}); err != nil {
		t.Fatal(err)
	}
	if _, err := artifacts.ParseAndValidate(domain.ArtifactInvestigation, d.MessageText()); err != nil {
		t.Fatal(err)
	}

	d = launchFake(t, "repair", "execute", Hooks{})
	sess, err = d.NewSession(ctx, NewSessionRequest{CWD: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.Prompt(ctx, PromptRequest{SessionID: sess.SessionID, Prompt: []ContentBlock{TextBlock("repair")}}); err != nil {
		t.Fatal(err)
	}
	if _, err := artifacts.ParseAndValidate(domain.ArtifactImplementation, d.MessageText()); err != nil {
		t.Fatal(err)
	}

	d = launchFake(t, "review_reject", "review", Hooks{})
	sess, err = d.NewSession(ctx, NewSessionRequest{CWD: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.Prompt(ctx, PromptRequest{SessionID: sess.SessionID, Prompt: []ContentBlock{TextBlock("review")}}); err != nil {
		t.Fatal(err)
	}
	art, err := artifacts.ParseAndValidate(domain.ArtifactReview, d.MessageText())
	if err != nil {
		t.Fatal(err)
	}
	if art.(artifacts.ReviewArtifact).Verdict != "fail" {
		t.Fatalf("verdict = %s", art.(artifacts.ReviewArtifact).Verdict)
	}
}

func TestDriverMalformed(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	d, err := Launch(ctx, Spec{Command: fakeACP, Env: withFakeEnv("malformed", "plan")}, DefaultClientConfig(), Hooks{}, Limits{HandshakeTimeout: 2 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	_, err = d.Handshake(ctx)
	if err == nil {
		t.Fatal("expected malformed protocol error")
	}
	if !errors.Is(err, ErrStdoutPollution) && !errors.Is(err, ErrProtocol) && !errors.Is(err, ErrProcessExited) {
		// handshake surfaces the RPC/internal error wrapping pollution
		if !strings.Contains(err.Error(), "not protocol") && !strings.Contains(err.Error(), "protocol") && !strings.Contains(err.Error(), "exited") && !strings.Contains(err.Error(), "rpc") {
			t.Fatalf("err = %v", err)
		}
	}
}

func TestDriverPermission(t *testing.T) {
	var sawPerm bool
	d := launchFake(t, "permission", "execute", Hooks{
		RequestPermission: func(ctx context.Context, p RequestPermissionParams) (RequestPermissionResult, error) {
			sawPerm = true
			if p.ToolCall.ToolCallID == "" {
				t.Error("missing toolCallId")
			}
			return SelectedPermission("allow-once"), nil
		},
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	sess, err := d.NewSession(ctx, NewSessionRequest{CWD: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.Prompt(ctx, PromptRequest{SessionID: sess.SessionID, Prompt: []ContentBlock{TextBlock("edit")}}); err != nil {
		t.Fatal(err)
	}
	if !sawPerm {
		t.Fatal("permission callback not invoked")
	}
}

func TestDriverAuthRequired(t *testing.T) {
	d := launchFake(t, "auth_required", "plan", Hooks{})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := d.NewSession(ctx, NewSessionRequest{CWD: t.TempDir()})
	if err == nil {
		t.Fatal("expected auth_required")
	}
	var rpc *Error
	if !errors.As(err, &rpc) || !rpc.AuthRequired() {
		t.Fatalf("err = %v", err)
	}
	if _, err := d.Authenticate(ctx, AuthenticateRequest{MethodID: "fake"}); err != nil {
		t.Fatal(err)
	}
	sess, err := d.NewSession(ctx, NewSessionRequest{CWD: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if sess.SessionID == "" {
		t.Fatal("empty session")
	}
}

func TestDriverCrash(t *testing.T) {
	d := launchFake(t, "crash", "plan", Hooks{})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	sess, err := d.NewSession(ctx, NewSessionRequest{CWD: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	_, err = d.Prompt(ctx, PromptRequest{SessionID: sess.SessionID, Prompt: []ContentBlock{TextBlock("go")}})
	if err == nil {
		t.Fatal("expected crash error")
	}
}

func TestDriverTimeout(t *testing.T) {
	d := launchFake(t, "timeout", "plan", Hooks{})
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	sess, err := d.NewSession(context.Background(), NewSessionRequest{CWD: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	_, err = d.Prompt(ctx, PromptRequest{SessionID: sess.SessionID, Prompt: []ContentBlock{TextBlock("wait")}})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v", err)
	}
}

func TestDriverIgnoreCancel(t *testing.T) {
	started := make(chan struct{}, 1)
	d := launchFake(t, "ignore_cancel", "plan", Hooks{
		OnEvent: func(e Event) {
			if e.Kind == EventMessageDelta || e.Kind == EventThoughtDelta {
				select {
				case started <- struct{}{}:
				default:
				}
			}
		},
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	sess, err := d.NewSession(ctx, NewSessionRequest{CWD: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	var resp *PromptResponse
	go func() {
		var e error
		resp, e = d.Prompt(ctx, PromptRequest{SessionID: sess.SessionID, Prompt: []ContentBlock{TextBlock("x")}})
		errCh <- e
	}()
	time.Sleep(50 * time.Millisecond)
	if err := d.Cancel(sess.SessionID); err != nil {
		t.Fatal(err)
	}
	if err := <-errCh; err != nil {
		t.Fatal(err)
	}
	if resp == nil || resp.StopReason == StopCancelled {
		t.Fatalf("expected agent to ignore cancel, got %+v", resp)
	}
}

func TestDriverMissingModelProviderInvalid(t *testing.T) {
	ctx := context.Background()
	d := launchFake(t, "missing_model", "plan", Hooks{})
	sess, err := d.NewSession(ctx, NewSessionRequest{CWD: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	_, err = d.Prompt(ctx, PromptRequest{SessionID: sess.SessionID, Prompt: []ContentBlock{TextBlock("x")}})
	if err == nil {
		t.Fatal("expected missing model error")
	}

	d = launchFake(t, "provider_fail", "plan", Hooks{})
	sess, err = d.NewSession(ctx, NewSessionRequest{CWD: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	_, err = d.Prompt(ctx, PromptRequest{SessionID: sess.SessionID, Prompt: []ContentBlock{TextBlock("x")}})
	if err == nil {
		t.Fatal("expected provider error")
	}

	d = launchFake(t, "invalid_output", "plan", Hooks{})
	sess, err = d.NewSession(ctx, NewSessionRequest{CWD: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.Prompt(ctx, PromptRequest{SessionID: sess.SessionID, Prompt: []ContentBlock{TextBlock("x")}}); err != nil {
		t.Fatal(err)
	}
	if _, err := artifacts.ParseAndValidate(domain.ArtifactPlan, d.MessageText()); err == nil {
		t.Fatal("expected invalid artifact")
	}
}

func TestDriverFSCallback(t *testing.T) {
	path := filepath.Join(t.TempDir(), "note.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	var read string
	d := launchFake(t, "success", "plan", Hooks{
		ReadTextFile: func(ctx context.Context, p ReadTextFileParams) (ReadTextFileResult, error) {
			read = p.Path
			b, err := os.ReadFile(p.Path)
			if err != nil {
				return ReadTextFileResult{}, err
			}
			return ReadTextFileResult{Content: string(b)}, nil
		},
	}, "WAYSHARD_FAKE_READ_FILE", path)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	sess, err := d.NewSession(ctx, NewSessionRequest{CWD: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.Prompt(ctx, PromptRequest{SessionID: sess.SessionID, Prompt: []ContentBlock{TextBlock("x")}}); err != nil {
		t.Fatal(err)
	}
	if read != path {
		t.Fatalf("read path = %q", read)
	}
}

func TestDriverUnknownExtensionTolerated(t *testing.T) {
	ev := NormalizeUpdate(SessionNotification{
		SessionID: "s",
		Update:    SessionUpdate{SessionUpdate: "config_option_update"},
	})
	if len(ev) != 1 || ev[0].Diagnostic == nil {
		t.Fatalf("%+v", ev)
	}
	if ev[0].Error != "" {
		t.Fatalf("unknown extension should not be a workflow error: %+v", ev[0])
	}
}

func TestRedactSecrets(t *testing.T) {
	s := Redact("Authorization: Bearer supersecretvalue api_key=sk-abcdefghijk")
	if strings.Contains(s, "supersecretvalue") || strings.Contains(s, "sk-abcdefghijk") {
		t.Fatalf("not redacted: %s", s)
	}
}

func TestOversizedFrame(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "main.go")
	body := "package main\nimport (\n\t\"bufio\"\n\t\"os\"\n\t\"strings\"\n)\nfunc main() {\n\tbufio.NewReader(os.Stdin).ReadBytes('\\n')\n\tos.Stdout.WriteString(`{\"jsonrpc\":\"2.0\",\"id\":1,\"result\":\"` + strings.Repeat(\"a\", 8192) + `\"}` + \"\\n\")\n}\n"
	if err := os.WriteFile(src, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(dir, testutil.ExeName("big"))
	build := exec.Command("go", "build", "-o", bin, src)
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build oversized helper: %v\n%s", err, out)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	d, err := Launch(ctx, Spec{Command: bin}, DefaultClientConfig(), Hooks{}, Limits{MaxFrameBytes: 1024, HandshakeTimeout: 2 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	_, err = d.Handshake(ctx)
	if err == nil {
		t.Fatal("expected oversized frame error")
	}
}

func TestNormalizeToolAndPlan(t *testing.T) {
	tool := NormalizeUpdate(SessionNotification{
		SessionID: "s",
		Update: SessionUpdate{
			SessionUpdate: UpdateToolCall,
			ToolCallID:    "t1",
			Title:         "edit",
			Kind:          "edit",
		},
	})
	if tool[0].Kind != EventToolCall || tool[0].ToolCallID != "t1" {
		t.Fatalf("%+v", tool)
	}
	plan := NormalizeUpdate(SessionNotification{
		SessionID: "s",
		Update: SessionUpdate{
			SessionUpdate: UpdatePlan,
			Entries:       []PlanEntry{{Content: "step", Status: "pending"}},
		},
	})
	if plan[0].Kind != EventPlan || len(plan[0].Plan) != 1 {
		t.Fatalf("%+v", plan)
	}
	perm := NormalizePermission(RequestPermissionParams{SessionID: "s", ToolCall: ToolCallUpdate{ToolCallID: "t1", Title: "edit"}})
	if perm.Kind != EventPermission {
		t.Fatalf("%+v", perm)
	}
}
