package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestFreshServerAuthBoundary proves that a fresh loopback server does not grant
// unauthenticated access to product APIs, while the recovery pairing path works.
func TestFreshServerAuthBoundary(t *testing.T) {
	_, ts := testAPI(t)
	protected := []string{"/v1/projects", "/v1/devices", "/v1/settings", "/v1/harnesses",
		"/v1/storage", "/v1/notifications", "/v1/approvals", "/v1/server"}
	for _, ep := range protected {
		req, _ := http.NewRequest(http.MethodGet, ts.URL+ep, nil)
		req.RemoteAddr = "127.0.0.1:1"
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		io.Copy(io.Discard, res.Body)
		res.Body.Close()
		if res.StatusCode != http.StatusUnauthorized {
			t.Fatalf("%s unauthenticated => %d, want 401", ep, res.StatusCode)
		}
	}
	// Recovery pairing invitation is permitted from loopback when no device exists.
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/v1/pairing/invitations", strings.NewReader("{}"))
	req.RemoteAddr = "127.0.0.1:1"
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != 200 {
		t.Fatalf("recovery invitation => %d", res.StatusCode)
	}
}

// TestFileAPISymlinkEscape denies reads/writes that escape the project root via
// a symlinked directory.
func TestFileAPISymlinkEscape(t *testing.T) {
	s, ts := testAPI(t)
	cred := issueCred(t, s)
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("host secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]string{"path": root})
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/v1/projects/open", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cred)
	res, _ := http.DefaultClient.Do(req)
	var proj struct {
		ID string `json:"id"`
	}
	_ = json.NewDecoder(res.Body).Decode(&proj)

	req, _ = http.NewRequest(http.MethodGet, ts.URL+"/v1/projects/"+proj.ID+"/file?path=link/secret.txt", nil)
	req.Header.Set("Authorization", "Bearer "+cred)
	res, _ = http.DefaultClient.Do(req)
	if res.StatusCode < 400 {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("symlink read escaped: %d %s", res.StatusCode, b)
	}

	payload, _ := json.Marshal(map[string]string{"path": "link/evil.txt", "content": "pwned"})
	req, _ = http.NewRequest(http.MethodPut, ts.URL+"/v1/projects/"+proj.ID+"/file", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cred)
	res, _ = http.DefaultClient.Do(req)
	if res.StatusCode < 400 {
		t.Fatalf("symlink write escaped: %d", res.StatusCode)
	}
	if _, err := os.Stat(filepath.Join(outside, "evil.txt")); err == nil {
		t.Fatal("file was written outside the project through a symlink")
	}
}

// TestIdempotencyConflict rejects reuse of an idempotency key with a different body.
func TestIdempotencyConflict(t *testing.T) {
	s, ts := testAPI(t)
	cred := issueCred(t, s)
	root := t.TempDir()
	body, _ := json.Marshal(map[string]string{"path": root})
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/v1/projects/open", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cred)
	res, _ := http.DefaultClient.Do(req)
	var proj struct {
		ID string `json:"id"`
	}
	_ = json.NewDecoder(res.Body).Decode(&proj)
	req, _ = http.NewRequest(http.MethodPost, ts.URL+"/v1/projects/"+proj.ID+"/conversations", bytes.NewReader([]byte(`{"title":"s"}`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cred)
	res, _ = http.DefaultClient.Do(req)
	var conv struct {
		ID string `json:"id"`
	}
	_ = json.NewDecoder(res.Body).Decode(&conv)

	send := func(text string) int {
		msg, _ := json.Marshal(map[string]any{"text": text})
		req, _ := http.NewRequest(http.MethodPost, ts.URL+"/v1/conversations/"+conv.ID+"/messages", bytes.NewReader(msg))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+cred)
		req.Header.Set("Idempotency-Key", "same-key")
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		io.Copy(io.Discard, res.Body)
		res.Body.Close()
		return res.StatusCode
	}
	if code := send("first"); code != 202 {
		t.Fatalf("first send %d", code)
	}
	if code := send("different body"); code != http.StatusConflict {
		t.Fatalf("key reuse with different body => %d, want 409", code)
	}
	if code := send("first"); code != 202 {
		t.Fatalf("replay of same body => %d, want 202", code)
	}
}
