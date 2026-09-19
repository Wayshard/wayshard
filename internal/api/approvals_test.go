package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/Wayshard/wayshard/internal/domain"
)

// TestApprovalLifecycle proves approvals are durable and resolvable from any
// authorized device, with pending state reflected.
func TestApprovalLifecycle(t *testing.T) {
	s, ts := testAPI(t)
	d1 := issueCred(t, s)
	d2 := issueCred(t, s)
	ap := &domain.Approval{RunID: "run-1", Kind: "acp_permission", Resource: "edit", Reason: "needs approval", Status: "pending"}
	if err := s.Store.InsertApproval(context.Background(), ap); err != nil {
		t.Fatal(err)
	}
	// Device 1 sees the pending approval.
	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/v1/approvals", nil)
	req.Header.Set("Authorization", "Bearer "+d1)
	res, _ := http.DefaultClient.Do(req)
	var list []struct {
		ID string `json:"id"`
	}
	_ = json.NewDecoder(res.Body).Decode(&list)
	found := false
	for _, a := range list {
		if a.ID == ap.ID {
			found = true
		}
	}
	if !found {
		t.Fatalf("pending approval not visible: %+v", list)
	}
	// Device 2 resolves it.
	body, _ := json.Marshal(map[string]string{"status": "allowed"})
	req, _ = http.NewRequest(http.MethodPost, ts.URL+"/v1/approvals/"+ap.ID+"/resolve", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+d2)
	res, _ = http.DefaultClient.Do(req)
	io.Copy(io.Discard, res.Body)
	if res.StatusCode != 200 {
		t.Fatalf("resolve => %d", res.StatusCode)
	}
	got, err := s.Store.GetApproval(context.Background(), ap.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "allowed" || got.ResolvedBy == "" {
		t.Fatalf("approval not persisted correctly: %+v", got)
	}
	pending, _ := s.Store.PendingApprovalCount(context.Background(), "run-1")
	if pending != 0 {
		t.Fatalf("pending count %d after resolve", pending)
	}
}
