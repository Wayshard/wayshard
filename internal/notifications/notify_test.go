package notifications

import (
	"context"
	"testing"

	"github.com/Wayshard/wayshard/internal/domain"
	"github.com/Wayshard/wayshard/internal/storage"
)

func TestDeriveRunBlocked(t *testing.T) {
	ctx := context.Background()
	st, err := storage.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	p := &domain.Project{Name: "p", Path: t.TempDir(), SourceKind: "filesystem"}
	if err := st.InsertProject(ctx, p); err != nil {
		t.Fatal(err)
	}
	c := &domain.Conversation{ProjectID: p.ID, Title: "t"}
	_ = st.InsertConversation(ctx, c)
	msg := &domain.Message{ConversationID: c.ID, Role: domain.RoleUser, Body: "x"}
	task := &domain.Task{ProjectID: p.ID, ConversationID: c.ID, Objective: "x"}
	run := &domain.Run{ProjectID: p.ID, ConversationID: c.ID}
	if err := st.CreateTaskRun(ctx, msg, task, run); err != nil {
		t.Fatal(err)
	}
	var seen []string
	st.EventHook = func(ev domain.Event) {
		seen = append(seen, ev.Type)
		Derive(ctx, st, nil, ev)
	}
	if err := st.UpdateRunStatus(ctx, run.ID, domain.RunBlocked, domain.BlockedNoViableRoute, "no route"); err != nil {
		t.Fatal(err)
	}
	if len(seen) == 0 {
		t.Fatal("event hook not invoked")
	}
	notes, _ := st.ListNotifications(ctx, false, 10)
	if len(notes) == 0 {
		t.Fatalf("no notification derived; events=%v", seen)
	}
	if notes[0].Kind != domain.NoteRunBlocked || !notes[0].Attention {
		t.Fatalf("unexpected notification: %+v", notes[0])
	}
}
