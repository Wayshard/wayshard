package orchestrator

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/Wayshard/wayshard/internal/artifacts"
	"github.com/Wayshard/wayshard/internal/ctxengine"
	"github.com/Wayshard/wayshard/internal/domain"
	"github.com/Wayshard/wayshard/internal/knowledge"
	"github.com/Wayshard/wayshard/internal/workspace"
)

// contextArtifact is the durable record of the exact bundle delivered to a
// stage. The context-inspection API returns this rather than reconstructing a
// hypothetical bundle.
type contextArtifact struct {
	Target   string                    `json:"target"`
	Stage    string                    `json:"stage"`
	Bundle   string                    `json:"bundle"`
	Manifest ctxengine.ContextManifest `json:"manifest"`
}

func contextTarget(stage domain.StageKind) (ctxengine.Target, bool) {
	switch stage {
	case domain.StagePlan, domain.StageReplan:
		return ctxengine.TargetPlanner, true
	case domain.StageExplore:
		return ctxengine.TargetExplore, true
	case domain.StageExecute:
		return ctxengine.TargetExecutor, true
	case domain.StageReview:
		return ctxengine.TargetReviewer, true
	case domain.StageRepair:
		return ctxengine.TargetRepair, true
	default:
		return "", false
	}
}

// stageBundle assembles the stage-specific context bundle and persists its
// manifest as a durable artifact. It returns the rendered text injected into
// the harness prompt.
func (e *Engine) stageBundle(ctx context.Context, run *domain.Run, task *domain.Task, st *domain.Stage) string {
	target, ok := contextTarget(st.Kind)
	if !ok {
		return ""
	}
	proj, err := e.Store.GetProject(ctx, run.ProjectID)
	if err != nil {
		return ""
	}
	idx, _ := knowledge.Discover(ctx, proj.Path)

	arts, _ := e.Store.ListArtifacts(ctx, run.ID)
	var refs []ctxengine.ArtifactRef
	for _, a := range arts {
		refs = append(refs, ctxengine.ArtifactRef{Kind: a.Kind, JSON: a.JSON, Stage: st.Kind})
	}
	msgs, _ := e.Store.ListMessages(ctx, run.ConversationID, 20)
	var window []ctxengine.WindowMessage
	for _, m := range msgs {
		window = append(window, ctxengine.WindowMessage{Role: string(m.Role), Body: m.Body})
	}

	var constraints []string
	if plan, err := loadPlan(ctx, e.Store, run.ID); err == nil && plan != nil {
		constraints = append(constraints, plan.AcceptanceCriteria...)
	}

	var gitFacts []ctxengine.GitFact
	if b, err := workspace.Open(proj.Path); err == nil {
		if cur, err := b.CurrentState(ctx); err == nil {
			if cur.Branch != "" {
				gitFacts = append(gitFacts, ctxengine.GitFact{Key: "branch", Value: cur.Branch})
			}
			if cur.HEAD != "" {
				gitFacts = append(gitFacts, ctxengine.GitFact{Key: "head", Value: cur.HEAD})
			}
		}
	}

	budget := e.ContextBudget
	if budget <= 0 {
		budget = 12000
	}
	eng := e.Context
	if eng == nil {
		eng = ctxengine.New()
	}
	bundle, man, err := eng.Assemble(ctx, ctxengine.ContextRequest{
		ProjectID:    proj.ID,
		TaskID:       task.ID,
		RunID:        run.ID,
		Stage:        st.Kind,
		Target:       target,
		TokenBudget:  budget,
		Request:      task.Objective,
		Constraints:  constraints,
		RecentWindow: window,
		Artifacts:    refs,
		GitFacts:     gitFacts,
		Knowledge:    idx,
	})
	if err != nil {
		return ""
	}
	text := renderBundle(bundle)
	rec := contextArtifact{Target: string(target), Stage: string(st.Kind), Bundle: text, Manifest: man}
	if body, err := artifacts.Marshal(rec); err == nil {
		_ = e.Store.InsertArtifact(ctx, &domain.Artifact{
			RunID: run.ID, StageID: st.ID, Kind: domain.ArtifactContextManifest,
			SchemaVer: artifacts.SchemaVersion, JSON: body, Valid: true,
		})
	}
	return text
}

func renderBundle(b ctxengine.ContextBundle) string {
	var sb strings.Builder
	for _, it := range b.Items {
		header := it.Kind
		if it.Path != "" {
			header += " " + it.Path
		}
		sb.WriteString("## ")
		sb.WriteString(header)
		sb.WriteString("\n")
		sb.WriteString(it.Body)
		sb.WriteString("\n\n")
	}
	return sb.String()
}

// latestContextArtifact returns the most recent persisted context artifact for
// a run, optionally filtered by stage.
func (e *Engine) latestContextArtifact(ctx context.Context, runID string, stage string) (json.RawMessage, bool) {
	arts, err := e.Store.ListArtifacts(ctx, runID)
	if err != nil {
		return nil, false
	}
	for i := len(arts) - 1; i >= 0; i-- {
		if arts[i].Kind != domain.ArtifactContextManifest {
			continue
		}
		var rec contextArtifact
		if json.Unmarshal([]byte(arts[i].JSON), &rec) != nil {
			continue
		}
		if stage != "" && rec.Stage != stage {
			continue
		}
		return json.RawMessage(arts[i].JSON), true
	}
	return nil, false
}
