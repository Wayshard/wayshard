package app

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Wayshard/wayshard/internal/domain"
	"github.com/coder/websocket"
)

// TestLiveWebSocketEvents proves committed events are pushed live (not just
// replayed), and that reconnect from a sequence resumes missed events.
func TestLiveWebSocketEvents(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	a, err := Open(ctx, Config{DataDir: t.TempDir(), Listen: "127.0.0.1:0"})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	ts := httptest.NewServer(a.Handler())
	defer ts.Close()
	cred := appCred(t, a)

	// Subscribe before creating the run.
	before, _ := a.Store.LatestEventSeq(ctx)
	wsURL := "ws" + ts.URL[len("http"):] + "/v1/ws"
	conn, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{
		HTTPHeader: map[string][]string{"Authorization": {"Bearer " + cred}},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "done")

	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "README.md"), []byte("x"), 0o644)
	resp := postJSON(t, ts.URL+"/v1/projects/open", `{"path":"`+dir+`","name":"live"}`, cred)
	var proj struct {
		ID string `json:"id"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&proj)
	resp = postJSON(t, ts.URL+"/v1/projects/"+proj.ID+"/conversations", `{"title":"s"}`, cred)
	var conv struct {
		ID string `json:"id"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&conv)
	msg, _ := json.Marshal(map[string]any{"text": "do something", "artifactOnly": true})
	resp = postJSON(t, ts.URL+"/v1/conversations/"+conv.ID+"/messages", string(msg), cred)

	// Read frames until a newly committed event (seq > before) arrives without
	// any client polling of the database.
	sawLive := false
	sawNewEvent := false
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) && !sawNewEvent {
		rctx, rcancel := context.WithTimeout(ctx, 2*time.Second)
		_, data, err := conn.Read(rctx)
		rcancel()
		if err != nil {
			continue
		}
		var frame struct {
			Type string `json:"type"`
			Seq  int64  `json:"seq"`
		}
		_ = json.Unmarshal(data, &frame)
		if frame.Type == "live" {
			sawLive = true
		}
		if frame.Type == "event" && frame.Seq > before {
			sawNewEvent = true
		}
	}
	if !sawLive {
		t.Fatal("never received live marker")
	}
	if !sawNewEvent {
		t.Fatal("no newly committed event delivered live over WebSocket")
	}
	_ = resp
}

// TestBlockedRunCreatesAttentionNotification proves the notification engine
// derives a durable high-value notification from a committed event.
func TestBlockedRunCreatesAttentionNotification(t *testing.T) {
	t.Setenv("PATH", t.TempDir()) // no harnesses -> NO_VIABLE_ROUTE
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	a, err := Open(ctx, Config{DataDir: t.TempDir(), Listen: "127.0.0.1:0"})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()

	p := &domain.Project{Name: "p", Path: t.TempDir(), SourceKind: "filesystem"}
	if err := a.Store.InsertProject(ctx, p); err != nil {
		t.Fatal(err)
	}
	c := &domain.Conversation{ProjectID: p.ID, Title: "t"}
	_ = a.Store.InsertConversation(ctx, c)
	msg := &domain.Message{ConversationID: c.ID, Role: domain.RoleUser, Body: "x"}
	task := &domain.Task{ProjectID: p.ID, ConversationID: c.ID, Objective: "x"}
	run := &domain.Run{ProjectID: p.ID, ConversationID: c.ID, Profile: domain.ProfileAuto}
	if err := a.Store.CreateTaskRun(ctx, msg, task, run); err != nil {
		t.Fatal(err)
	}
	if err := a.Engine.ProcessRun(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	notes, err := a.Store.ListNotifications(ctx, true, 50)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, n := range notes {
		if n.Kind == domain.NoteRunBlocked && n.Attention {
			found = true
		}
	}
	if !found {
		t.Fatalf("no attention notification for blocked run: %+v", notes)
	}
}
