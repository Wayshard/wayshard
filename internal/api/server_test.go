package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/Wayshard/wayshard/internal/auth"
	"github.com/Wayshard/wayshard/internal/events"
	"github.com/Wayshard/wayshard/internal/secrets"
	"github.com/Wayshard/wayshard/internal/storage"
)

func testAPI(t *testing.T) (*Server, *httptest.Server) {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	st, err := storage.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	v, err := secrets.Open(filepath.Join(dir, "vault"), secrets.FileProvider{Path: filepath.Join(dir, "vault.key")})
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{
		Store:  st,
		Auth:   &auth.Service{Store: st, Vault: v, Listen: "http://127.0.0.1:7420"},
		Hub:    events.NewHub(st),
		Listen: "127.0.0.1:7420",
	}
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	return s, ts
}

func issueCred(t *testing.T, s *Server) string {
	t.Helper()
	ctx := context.Background()
	inv, err := s.Auth.CreateInvitation(ctx, "", "test")
	if err != nil {
		t.Fatal(err)
	}
	res, err := s.Auth.CompletePairing(ctx, auth.CompletePairingRequest{Code: inv.Code, DeviceKind: "cli", DeviceName: "test"})
	if err != nil {
		t.Fatal(err)
	}
	return res.Credential
}

func TestHealthAndMeta(t *testing.T) {
	_, ts := testAPI(t)
	res, err := http.Get(ts.URL + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != 200 {
		t.Fatal(res.Status)
	}
}

func TestOpenProjectIsPassive(t *testing.T) {
	s, ts := testAPI(t)
	cred := issueCred(t, s)
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]string{"path": root, "name": "demo"})
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/v1/projects/open", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cred)
	req.RemoteAddr = "127.0.0.1:1"
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != 201 && res.StatusCode != 200 {
		t.Fatalf("status %d", res.StatusCode)
	}
	entries, _ := os.ReadDir(root)
	if len(entries) != 1 {
		t.Fatalf("open mutated tree: %v", entries)
	}
	_ = s
}

func TestFileSaveConflict(t *testing.T) {
	s, ts := testAPI(t)
	cred := issueCred(t, s)
	root := t.TempDir()
	path := filepath.Join(root, "a.txt")
	if err := os.WriteFile(path, []byte("one"), 0o644); err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]string{"path": root})
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/v1/projects/open", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cred)
	req.RemoteAddr = "127.0.0.1:1"
	res, _ := http.DefaultClient.Do(req)
	var proj struct {
		ID string `json:"id"`
	}
	_ = json.NewDecoder(res.Body).Decode(&proj)
	// read
	req, _ = http.NewRequest(http.MethodGet, ts.URL+"/v1/projects/"+proj.ID+"/file?path=a.txt", nil)
	req.Header.Set("Authorization", "Bearer "+cred)
	req.RemoteAddr = "127.0.0.1:1"
	res, _ = http.DefaultClient.Do(req)
	var got struct {
		Hash string `json:"hash"`
	}
	_ = json.NewDecoder(res.Body).Decode(&got)
	_ = os.WriteFile(path, []byte("two"), 0o644)
	payload, _ := json.Marshal(map[string]string{"path": "a.txt", "content": "three", "expectedHash": got.Hash})
	req, _ = http.NewRequest(http.MethodPut, ts.URL+"/v1/projects/"+proj.ID+"/file", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cred)
	req.RemoteAddr = "127.0.0.1:1"
	res, _ = http.DefaultClient.Do(req)
	if res.StatusCode != http.StatusConflict {
		t.Fatalf("expected conflict, got %d", res.StatusCode)
	}
	cur, _ := os.ReadFile(path)
	if string(cur) != "two" {
		t.Fatalf("overwrote external edit: %s", cur)
	}
	_ = s
}

func TestRemoveProjectDoesNotDeleteSource(t *testing.T) {
	s, ts := testAPI(t)
	cred := issueCred(t, s)
	root := t.TempDir()
	_ = os.WriteFile(filepath.Join(root, "keep.txt"), []byte("x"), 0o644)
	body, _ := json.Marshal(map[string]string{"path": root})
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/v1/projects/open", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cred)
	req.RemoteAddr = "127.0.0.1:1"
	res, _ := http.DefaultClient.Do(req)
	var proj struct {
		ID string `json:"id"`
	}
	_ = json.NewDecoder(res.Body).Decode(&proj)
	req, _ = http.NewRequest(http.MethodDelete, ts.URL+"/v1/projects/"+proj.ID, nil)
	req.Header.Set("Authorization", "Bearer "+cred)
	req.RemoteAddr = "127.0.0.1:1"
	res, _ = http.DefaultClient.Do(req)
	if res.StatusCode != 200 {
		t.Fatal(res.Status)
	}
	if _, err := os.Stat(filepath.Join(root, "keep.txt")); err != nil {
		t.Fatal("source was deleted")
	}
}
