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
)

func TestIntakeThroughCompleteWithSyntheticExec(t *testing.T) {
	t.Setenv("WAYSHARD_SYNTHETIC_ROUTE", "1")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	a, err := Open(ctx, Config{DataDir: t.TempDir(), Listen: "127.0.0.1:0"})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	ts := httptest.NewServer(a.Handler())
	defer ts.Close()
	cred := appCred(t, a)

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("demo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	open, _ := json.Marshal(map[string]string{"path": dir, "name": "demo"})
	resp := postJSON(t, ts.URL+"/v1/projects/open", string(open), cred)
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("open %d %s", resp.StatusCode, b)
	}
	var proj struct {
		ID string `json:"id"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&proj)
	resp = postJSON(t, ts.URL+"/v1/projects/"+proj.ID+"/conversations", `{"title":"s"}`, cred)
	var conv struct {
		ID string `json:"id"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&conv)
	msg, _ := json.Marshal(map[string]any{"text": "brainstorm a README outline", "artifactOnly": true})
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/v1/conversations/"+conv.ID+"/messages", bytes.NewReader(msg))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cred)
	req.Header.Set("Idempotency-Key", "e2e-1")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 202 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("send %d %s", resp.StatusCode, b)
	}
	var out struct {
		Run struct {
			ID string `json:"id"`
		} `json:"run"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if out.Run.ID == "" {
		t.Fatal("missing run id")
	}
	a.Sched.Enqueue(out.Run.ID)
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		r, err := a.Store.GetRun(ctx, out.Run.ID)
		if err == nil && r.Status.Terminal() {
			if r.Status != "complete" && r.Status != "blocked" && r.Status != "failed" {
				t.Fatalf("status %s", r.Status)
			}
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	r, _ := a.Store.GetRun(ctx, out.Run.ID)
	t.Fatalf("run did not finish: %+v", r)
}
