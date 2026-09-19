package orchestrator

import (
	"context"

	"github.com/Wayshard/wayshard/internal/domain"
)

// Budgets are deterministic Go-owned stopping conditions. Model output can
// never raise them. They bound stage creation and attempts so a permanently
// failing task reaches a terminal outcome instead of looping forever.
type Budgets struct {
	// MaxStages caps the total number of stages a single run may create.
	MaxStages int
	// MaxRepair caps repair stages.
	MaxRepair int
	// MaxReplan caps replan stages.
	MaxReplan int
	// MaxStageAttempts caps attempts within one stage (primary + fallbacks +
	// output correction). Infrastructure retry and output correction share
	// this bound so neither can grow without limit.
	MaxStageAttempts int
}

func DefaultBudgets() Budgets {
	return Budgets{
		MaxStages:        32,
		MaxRepair:        3,
		MaxReplan:        2,
		MaxStageAttempts: 3,
	}
}

func (b Budgets) withDefaults() Budgets {
	d := DefaultBudgets()
	if b.MaxStages <= 0 {
		b.MaxStages = d.MaxStages
	}
	if b.MaxRepair <= 0 {
		b.MaxRepair = d.MaxRepair
	}
	if b.MaxReplan <= 0 {
		b.MaxReplan = d.MaxReplan
	}
	if b.MaxStageAttempts <= 0 {
		b.MaxStageAttempts = d.MaxStageAttempts
	}
	return b
}

// budgetExhausted reports whether a run has reached a deterministic stop.
func (e *Engine) budget(ctx context.Context, kind domain.StageKind, runID string) (domain.BlockedReason, string, bool) {
	b := e.Budget.withDefaults()
	total, err := e.Store.CountStages(ctx, runID)
	if err != nil {
		total = 0
	}
	if total >= b.MaxStages {
		return domain.BlockedBudget, "stage budget exhausted", true
	}
	switch kind {
	case domain.StageRepair:
		n, _ := e.Store.CountStagesByKind(ctx, runID, domain.StageRepair)
		if n >= b.MaxRepair {
			return domain.BlockedBudget, "repair budget exhausted", true
		}
	case domain.StageReplan:
		n, _ := e.Store.CountStagesByKind(ctx, runID, domain.StageReplan)
		if n >= b.MaxReplan {
			return domain.BlockedBudget, "replan budget exhausted", true
		}
	}
	return "", "", false
}