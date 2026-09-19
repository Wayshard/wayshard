package app

import (
	"context"
	"strings"
	"testing"

	"github.com/Wayshard/wayshard/internal/domain"
	"github.com/Wayshard/wayshard/internal/routing"
)

type providerCandidates struct{}

func (providerCandidates) Candidates(context.Context) ([]routing.Candidate, error) {
	return []routing.Candidate{{
		Harness:   domain.HarnessInstallation{ID: "real", DisplayName: "real", Health: domain.HarnessReady, Compatibility: domain.CompatRoutable},
		Isolation: domain.IsolationOuterOnly,
		Network:   domain.NetworkProvider,
	}}, nil
}

// TestProviderHarnessRouteFailsClosed proves a harness requiring provider
// network is blocked before launch rather than run under raw host networking.
func TestProviderHarnessRouteFailsClosed(t *testing.T) {
	ctx := context.Background()
	a, err := Open(ctx, Config{DataDir: t.TempDir(), Listen: "127.0.0.1:0"})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	a.Engine.Candidates = providerCandidates{}

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
	got, _ := a.Store.GetRun(ctx, run.ID)
	if got.Status != domain.RunBlocked || got.BlockedReason != domain.BlockedNoViableRoute {
		t.Fatalf("status=%s reason=%s detail=%s", got.Status, got.BlockedReason, got.BlockedDetail)
	}
	if !strings.Contains(got.BlockedDetail, "provider network") {
		t.Fatalf("blocked detail should explain provider isolation: %q", got.BlockedDetail)
	}
}
