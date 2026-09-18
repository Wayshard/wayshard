package orchestrator

import (
	"strings"

	"github.com/Wayshard/wayshard/internal/artifacts"
	"github.com/Wayshard/wayshard/internal/domain"
)

// CompletionPolicy is deterministic Go logic. Model prose cannot override hard gates.
func CompletionPolicy(artifactOnly bool, plan *artifacts.PlanArtifact, val *artifacts.ValidationArtifact, rev *artifacts.ReviewArtifact, pendingApprovals int, integrated bool) domain.CompletionOutcome {
	if pendingApprovals > 0 {
		return domain.OutcomeBlocked
	}
	if val != nil {
		for _, c := range val.Checks {
			if c.Required && (c.Status == string(domain.CheckFail) || c.Status == string(domain.CheckBlocked)) {
				return domain.OutcomeRepair
			}
		}
		if len(val.BlockingFailures) > 0 {
			return domain.OutcomeRepair
		}
	}
	if plan != nil && val != nil {
		for _, crit := range plan.AcceptanceCriteria {
			if unverified(val, crit) && (rev == nil || !criterionPassed(rev, crit)) {
				// unverified is honest; review may still fail the run
			}
		}
	}
	if rev != nil {
		if strings.EqualFold(rev.Verdict, "fail") {
			if hasBlockingFinding(rev) {
				return domain.OutcomeRepair
			}
			return domain.OutcomeReplan
		}
		if strings.EqualFold(rev.Verdict, "insufficient") {
			return domain.OutcomeExplore
		}
	}
	if !artifactOnly && !integrated {
		return domain.OutcomeIntegrate
	}
	return domain.OutcomeComplete
}

func hasBlockingFinding(r *artifacts.ReviewArtifact) bool {
	for _, f := range r.Findings {
		if f.Severity == "blocking" || f.Severity == "major" {
			return true
		}
	}
	return false
}

func criterionPassed(r *artifacts.ReviewArtifact, crit string) bool {
	for _, c := range r.Criteria {
		if strings.EqualFold(c.ID, crit) || strings.Contains(strings.ToLower(c.Evidence), strings.ToLower(crit)) {
			return c.Status == "pass"
		}
	}
	return false
}

func unverified(v *artifacts.ValidationArtifact, crit string) bool {
	for _, u := range v.Unverified {
		if strings.Contains(strings.ToLower(u), strings.ToLower(crit)) {
			return true
		}
	}
	return false
}
