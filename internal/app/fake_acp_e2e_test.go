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
	"testing"
	"time"

	"github.com/Wayshard/wayshard/internal/domain"
	"github.com/Wayshard/wayshard/internal/testutil"
)

func TestFakeACPIntakeToCompleteArtifactOnly(t *testing.T) {
	bin := buildFakeACP(t)
	t.Setenv("PATH", filepath.Dir(bin)+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("WAYSHARD_FAKE_SCENARIO", "success")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	a, err := Open(ctx, Config{DataDir: t.TempDir(), Listen: "127.0.0.1:0"})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	ts := httptest.NewServer(a.Handler())
	defer ts.Close()

	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "README.md"), []byte("demo\n"), 0o644)
	open, _ := json.Marshal(map[string]string{"path": dir, "name": "demo"})
	resp, err := http.Post(ts.URL+"/v1/projects/open", "application/json", bytes.NewReader(open))
	if err != nil {
		t.Fatal(err)
	}
	var proj struct {
		ID string `json:"id"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&proj)
	resp, _ = http.Post(ts.URL+"/v1/projects/"+proj.ID+"/conversations", "application/json", bytes.NewReader([]byte(`{"title":"s"}`)))
	var conv struct {
		ID string `json:"id"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&conv)
	msg, _ := json.Marshal(map[string]any{"text": "brainstorm a README outline", "artifactOnly": true})
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/v1/conversations/"+conv.ID+"/messages", bytes.NewReader(msg))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "fake-acp-e2e")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 202 {
		t.Fatalf("send %d %s", resp.StatusCode, b)
	}
	var out struct {
		Run struct {
			ID string `json:"id"`
		} `json:"run"`
	}
	_ = json.Unmarshal(b, &out)
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		r, err := a.Store.GetRun(ctx, out.Run.ID)
		if err == nil && (r.Status.Terminal() || r.Status == domain.RunBlocked || r.Status == domain.RunReadyToIntegrate) {
			if r.Status == domain.RunFailed {
				t.Fatalf("failed: %s", r.BlockedDetail)
			}
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	r, _ := a.Store.GetRun(ctx, out.Run.ID)
	t.Fatalf("run did not settle: %+v", r)
}

func TestNoViableRouteWithoutHarness(t *testing.T) {
	t.Setenv("PATH", t.TempDir()) // empty of harnesses
	ctx := context.Background()
	a, err := Open(ctx, Config{DataDir: t.TempDir(), Listen: "127.0.0.1:0"})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	p := &domain.Project{Name: "p", Path: t.TempDir(), SourceKind: "filesystem"}
	_ = a.Store.InsertProject(ctx, p)
	c := &domain.Conversation{ProjectID: p.ID, Title: "t"}
	_ = a.Store.InsertConversation(ctx, c)
	msg := &domain.Message{ConversationID: c.ID, Role: domain.RoleUser, Body: "x"}
	task := &domain.Task{ProjectID: p.ID, ConversationID: c.ID, Objective: "x"}
	run := &domain.Run{ProjectID: p.ID, ConversationID: c.ID, Profile: domain.ProfileAuto}
	_ = a.Store.CreateTaskRun(ctx, msg, task, run)
	_ = a.Engine.ProcessRun(ctx, run.ID)
	got, _ := a.Store.GetRun(ctx, run.ID)
	if got.BlockedReason != domain.BlockedNoViableRoute && got.Status != domain.RunBlocked {
		t.Fatalf("status=%s reason=%s", got.Status, got.BlockedReason)
	}
}

func buildFakeACP(t *testing.T) string {
	t.Helper()
	return testutil.BuildFakeACP(t)
}
