package orchestrator

import (
	"testing"

	"github.com/Wayshard/wayshard/internal/artifacts"
	"github.com/Wayshard/wayshard/internal/domain"
)

func TestValidationFailureForcesRepairNotComplete(t *testing.T) {
	val := &artifacts.ValidationArtifact{
		Checks:           []artifacts.ValidationCheck{{Name: "tests", Required: true, Status: string(domain.CheckFail)}},
		BlockingFailures: []string{"tests"},
	}
	rev := &artifacts.ReviewArtifact{Verdict: "pass"}
	got := CompletionPolicy(false, nil, val, rev, 0, false)
	if got != domain.OutcomeRepair {
		t.Fatalf("got %s", got)
	}
}

func TestReviewPassDoesNotCompleteWithoutIntegration(t *testing.T) {
	val := &artifacts.ValidationArtifact{Checks: []artifacts.ValidationCheck{{Name: "t", Required: true, Status: string(domain.CheckPass)}}}
	rev := &artifacts.ReviewArtifact{Verdict: "pass"}
	got := CompletionPolicy(false, nil, val, rev, 0, false)
	if got != domain.OutcomeIntegrate {
		t.Fatalf("got %s want ready_to_integrate", got)
	}
}

func TestArtifactOnlyCompletesWithoutIntegration(t *testing.T) {
	got := CompletionPolicy(true, nil, nil, &artifacts.ReviewArtifact{Verdict: "pass"}, 0, false)
	if got != domain.OutcomeComplete {
		t.Fatalf("got %s", got)
	}
}

func TestPendingApprovalBlocks(t *testing.T) {
	got := CompletionPolicy(true, nil, nil, &artifacts.ReviewArtifact{Verdict: "pass"}, 1, false)
	if got != domain.OutcomeBlocked {
		t.Fatalf("got %s", got)
	}
}
