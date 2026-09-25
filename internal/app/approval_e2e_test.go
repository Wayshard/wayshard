package app

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Wayshard/wayshard/internal/domain"
)

// approvalSetup starts an in-process server whose deterministic ACP fixture
// requests a permission at the execute stage and performs a protected canary
// write only when the selected option is an allow.
func approvalSetup(t *testing.T) (base, cred, runID, canary string, a *App) {
	t.Helper()
	bin := buildFakeACP(t)
	t.Setenv("PATH", filepath.Dir(bin)+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("WAYSHARD_FAKE_SCENARIO", "permission")
	t.Setenv("WAYSHARD_FAKE_PERMISSION_STAGE", "execute")
	canaryDir := t.TempDir()
	canary = filepath.Join(canaryDir, "must-not-exist-after-deny.txt")
	t.Setenv("WAYSHARD_FAKE_PERMISSION_CANARY", canary)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)
	var err error
	a, err = Open(ctx, Config{DataDir: t.TempDir(), Listen: "127.0.0.1:0"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { a.Close() })
	ts := httptest.NewServer(a.Handler())
	t.Cleanup(ts.Close)
	base = ts.URL
	cred = appCred(t, a)

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("demo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	open, _ := json.Marshal(map[string]string{"path": dir, "name": "approval"})
	resp := postJSON(t, base+"/v1/projects/open", string(open), cred)
	var proj struct {
		ID string `json:"id"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&proj)
	resp = postJSON(t, base+"/v1/projects/"+proj.ID+"/conversations", `{"title":"s"}`, cred)
	var conv struct {
		ID string `json:"id"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&conv)
	msg, _ := json.Marshal(map[string]any{"text": "make a change", "artifactOnly": false})
	req, _ := http.NewRequest(http.MethodPost, base+"/v1/conversations/"+conv.ID+"/messages", bytes.NewReader(msg))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cred)
	req.Header.Set("Idempotency-Key", "approval-"+conv.ID)
	r, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Body.Close()
	b, _ := io.ReadAll(r.Body)
	if r.StatusCode != 202 {
		t.Fatalf("send %d %s", r.StatusCode, b)
	}
	var out struct {
		Run struct {
			ID string `json:"id"`
		} `json:"run"`
	}
	_ = json.Unmarshal(b, &out)
	runID = out.Run.ID
	if runID == "" {
		t.Fatalf("no run: %s", b)
	}
	return base, cred, runID, canary, a
}

// approvalHTTPJSON is a platform-neutral helper so this test compiles on every
// Go CI target.
func approvalHTTPJSON(t *testing.T, method, url, body, cred string) (int, []byte) {
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
	resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, b
}

func waitApproval(t *testing.T, base, cred string) domain.Approval {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		code, body := approvalHTTPJSON(t, http.MethodGet, base+"/v1/approvals", "", cred)
		if code == 200 {
			var list []domain.Approval
			_ = json.Unmarshal(body, &list)
			for _, ap := range list {
				if ap.Status == "pending" {
					return ap
				}
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("no pending approval appeared")
	return domain.Approval{}
}

func waitRunSettled(t *testing.T, base, cred, runID string, timeout time.Duration) domain.Run {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var last domain.Run
	for time.Now().Before(deadline) {
		code, body := approvalHTTPJSON(t, http.MethodGet, base+"/v1/runs/"+runID, "", cred)
		if code == 200 {
			_ = json.Unmarshal(body, &last)
			if last.Status.Terminal() || last.Status == domain.RunIntegrationBlocked {
				return last
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("run did not settle: %+v", last)
	return last
}

// TestApprovalDenyEndToEnd proves a denied ACP permission never executes the
// protected operation.
func TestApprovalDenyEndToEnd(t *testing.T) {
	base, cred, runID, canary, _ := approvalSetup(t)
	ap := waitApproval(t, base, cred)
	if ap.RunID != runID {
		t.Fatalf("approval run %s != %s", ap.RunID, runID)
	}
	code, body := approvalHTTPJSON(t, http.MethodPost, base+"/v1/approvals/"+ap.ID+"/resolve", `{"status":"denied"}`, cred)
	if code != 200 {
		t.Fatalf("resolve deny %d %s", code, body)
	}
	waitRunSettled(t, base, cred, runID, 40*time.Second)
	if _, err := os.Stat(canary); err == nil {
		t.Fatal("protected operation executed despite DENY")
	}
	// No pending approval remains.
	code, body = approvalHTTPJSON(t, http.MethodGet, base+"/v1/approvals", "", cred)
	if code != 200 {
		t.Fatalf("list approvals %d", code)
	}
	var list []domain.Approval
	_ = json.Unmarshal(body, &list)
	for _, ap := range list {
		if ap.Status == "pending" {
			t.Fatalf("pending approval after deny: %+v", ap)
		}
	}
}

// TestApprovalApproveEndToEnd proves an approved ACP permission executes the
// protected operation exactly once.
func TestApprovalApproveEndToEnd(t *testing.T) {
	base, cred, runID, canary, _ := approvalSetup(t)
	ap := waitApproval(t, base, cred)
	code, body := approvalHTTPJSON(t, http.MethodPost, base+"/v1/approvals/"+ap.ID+"/resolve", `{"status":"allowed"}`, cred)
	if code != 200 {
		t.Fatalf("resolve allow %d %s", code, body)
	}
	waitRunSettled(t, base, cred, runID, 40*time.Second)
	data, err := os.ReadFile(canary)
	if err != nil {
		t.Fatalf("protected operation did not execute on APPROVE: %v", err)
	}
	if string(data) != "executed\n" {
		t.Fatalf("canary content %q", data)
	}
}

// TestApprovalResolveRequiresAuth proves approval resolution is not reachable
// without a paired device credential.
func TestApprovalResolveRequiresAuth(t *testing.T) {
	bin := buildFakeACP(t)
	t.Setenv("PATH", filepath.Dir(bin)+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("WAYSHARD_FAKE_SCENARIO", "permission")
	t.Setenv("WAYSHARD_FAKE_PERMISSION_STAGE", "execute")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	a, err := Open(ctx, Config{DataDir: t.TempDir(), Listen: "127.0.0.1:0"})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	ts := httptest.NewServer(a.Handler())
	defer ts.Close()
	ap := &domain.Approval{RunID: "run-x", Kind: "acp_permission", Resource: "edit", Status: "pending"}
	if err := a.Store.InsertApproval(ctx, ap); err != nil {
		t.Fatal(err)
	}
	code, _ := approvalHTTPJSON(t, http.MethodPost, ts.URL+"/v1/approvals/"+ap.ID+"/resolve", `{"status":"denied"}`, "")
	if code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated resolve = %d, want 401", code)
	}
	got, _ := a.Store.GetApproval(ctx, ap.ID)
	if got.Status != "pending" {
		t.Fatalf("approval changed by unauthenticated request: %s", got.Status)
	}
}

// TestApprovalCancelWhilePending proves cancelling a run with a pending
// permission resolves the approval, never executes the operation, and leaves no
// pending approval.
func TestApprovalCancelWhilePending(t *testing.T) {
	base, cred, runID, canary, _ := approvalSetup(t)
	waitApproval(t, base, cred)
	code, body := approvalHTTPJSON(t, http.MethodPost, base+"/v1/runs/"+runID+"/cancel", `{}`, cred)
	if code != 200 {
		t.Fatalf("cancel %d %s", code, body)
	}
	final := waitRunSettled(t, base, cred, runID, 40*time.Second)
	if final.Status != domain.RunCancelled {
		t.Fatalf("expected cancelled, got %s %s", final.Status, final.BlockedDetail)
	}
	if _, err := os.Stat(canary); err == nil {
		t.Fatal("protected operation executed after cancellation")
	}
	code, body = approvalHTTPJSON(t, http.MethodGet, base+"/v1/approvals", "", cred)
	if code != 200 {
		t.Fatalf("list approvals %d", code)
	}
	var list []domain.Approval
	_ = json.Unmarshal(body, &list)
	for _, a := range list {
		if a.Status == "pending" {
			t.Fatalf("pending approval after cancel: %+v", a)
		}
	}
}
