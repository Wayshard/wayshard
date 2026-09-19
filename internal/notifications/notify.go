// Package notifications derives high-value, durable user notifications from
// committed durable domain events. It deliberately ignores low-level events.
package notifications

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/Wayshard/wayshard/internal/domain"
	"github.com/Wayshard/wayshard/internal/storage"
)

// Derive inspects a committed event and creates at most one durable
// notification. It is invoked from the storage commit hook, after commit.
func Derive(ctx context.Context, st *storage.Store, log *slog.Logger, ev domain.Event) {
	n := derive(st, ev)
	if n == nil {
		return
	}
	if err := st.InsertNotification(ctx, n); err != nil && log != nil {
		log.Warn("notification insert", "err", err, "event", ev.Type)
	}
}

func derive(st *storage.Store, ev domain.Event) *domain.Notification {
	var payload map[string]any
	if ev.Payload != "" {
		_ = json.Unmarshal([]byte(ev.Payload), &payload)
	}
	projectID := ev.ProjectID
	runID := ev.RunID
	if runID != "" && projectID == "" {
		if r, err := st.GetRun(context.Background(), runID); err == nil {
			projectID = r.ProjectID
		}
	}
	switch ev.Type {
	case "run.status":
		status, _ := payload["status"].(string)
		switch domain.RunStatus(status) {
		case domain.RunComplete:
			return &domain.Notification{RunID: runID, ProjectID: projectID, Kind: domain.NoteRunComplete,
				Title: "Run complete", Body: "A run finished and is ready for review."}
		case domain.RunBlocked:
			return &domain.Notification{RunID: runID, ProjectID: projectID, Kind: domain.NoteRunBlocked, Attention: true,
				Title: "Run blocked", Body: detailOr(payload, "A run is blocked and needs attention.")}
		case domain.RunFailed:
			return &domain.Notification{RunID: runID, ProjectID: projectID, Kind: domain.NoteRunFailed, Attention: true,
				Title: "Run failed", Body: detailOr(payload, "A run failed.")}
		}
	case "integration.updated":
		status, _ := payload["status"].(string)
		if status == "blocked" || status == "diverged" || status == "incomplete" {
			return &domain.Notification{RunID: runID, ProjectID: projectID, Kind: domain.NoteIntegrationConflict, Attention: true,
				Title: "Integration conflict", Body: "A validated run could not be integrated safely; the source was left untouched."}
		}
	case "approval.requested":
		return &domain.Notification{RunID: runID, ProjectID: projectID, Kind: domain.NoteApprovalRequired, Attention: true,
			Title: "Approval required", Body: "A stage is waiting for an approval decision."}
	case "approval.resolved":
		if runID != "" {
			_ = st.ClearAttentionForRunKind(context.Background(), runID, string(domain.NoteApprovalRequired))
		}
	}
	return nil
}

func detailOr(payload map[string]any, fallback string) string {
	if d, ok := payload["detail"].(string); ok && d != "" {
		return d
	}
	return fallback
}