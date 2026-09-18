package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"time"

	"github.com/Wayshard/wayshard/internal/artifacts"
	"github.com/Wayshard/wayshard/internal/domain"
	"github.com/Wayshard/wayshard/internal/id"
	"github.com/Wayshard/wayshard/internal/jev"
	"github.com/Wayshard/wayshard/internal/routing"
	"github.com/Wayshard/wayshard/internal/storage"
	"github.com/Wayshard/wayshard/internal/validation"
	"github.com/Wayshard/wayshard/internal/workspace"
)

// StageExec executes a semantic stage against a harness or server-owned subsystem.
type StageExec interface {
	Execute(ctx context.Context, req StageRequest) (StageResult, error)
}

type StageRequest struct {
	Run       domain.Run
	Task      domain.Task
	Stage     domain.Stage
	Attempt   domain.StageAttempt
	Route     routing.Candidate
	Workspace *domain.WorkspaceRecord
	Bundle    string
}

type StageResult struct {
	ArtifactJSON string
	Usage        *domain.Usage
	Class        domain.FailureClass
	Err          error
}

type WorkspacePrep interface {
	Prepare(ctx context.Context, project domain.Project, run domain.Run) (*domain.WorkspaceRecord, *domain.Snapshot, error)
}

type Integrator interface {
	Integrate(ctx context.Context, run domain.Run, ws *domain.WorkspaceRecord) (*domain.Integration, error)
}

type CandidateSource interface {
	Candidates(ctx context.Context) ([]routing.Candidate, error)
}

type Engine struct {
	Store      *storage.Store
	Jev        jev.DecisionEngine
	Router     *routing.Router
	Exec       StageExec
	Workspace  WorkspacePrep
	Integrate  Integrator
	Candidates CandidateSource
	Validate   *validation.Runner
	Log        *slog.Logger
}

func (e *Engine) ProcessRun(ctx context.Context, runID string) error {
	run, err := e.Store.GetRun(ctx, runID)
	if err != nil {
		return err
	}
	task, err := e.Store.GetTask(ctx, run.TaskID)
	if err != nil {
		return err
	}
	if run.Status.Terminal() {
		return nil
	}
	if run.Status == domain.RunQueued {
		_ = e.Store.CompareAndSetRunStatus(ctx, run.ID, domain.RunQueued, domain.RunAssessing, "", "")
		run.Status = domain.RunAssessing
	}
	if run.Status == domain.RunAssessing {
		if err := e.assess(ctx, run, task); err != nil {
			return err
		}
		run, _ = e.Store.GetRun(ctx, run.ID)
	}
	for !run.Status.Terminal() {
		switch run.Status {
		case domain.RunPlanning:
			if err := e.runStage(ctx, run, task, domain.StagePlan); err != nil {
				return err
			}
		case domain.RunExploring:
			if err := e.runStage(ctx, run, task, domain.StageExplore); err != nil {
				return err
			}
		case domain.RunExecuting:
			if err := e.runStage(ctx, run, task, domain.StageExecute); err != nil {
				return err
			}
		case domain.RunValidating:
			if err := e.runStage(ctx, run, task, domain.StageValidate); err != nil {
				return err
			}
		case domain.RunReviewing:
			if err := e.runStage(ctx, run, task, domain.StageReview); err != nil {
				return err
			}
		case domain.RunRepairing:
			if err := e.runStage(ctx, run, task, domain.StageRepair); err != nil {
				return err
			}
		case domain.RunReplanning:
			if err := e.runStage(ctx, run, task, domain.StageReplan); err != nil {
				return err
			}
		case domain.RunReadyToIntegrate, domain.RunIntegrating:
			if err := e.integrate(ctx, run, task); err != nil {
				return err
			}
		case domain.RunBlocked, domain.RunIntegrationBlocked:
			return nil
		default:
			return nil
		}
		run, err = e.Store.GetRun(ctx, run.ID)
		if err != nil {
			return err
		}
	}
	return nil
}

func (e *Engine) assess(ctx context.Context, run *domain.Run, task *domain.Task) error {
	eng := e.Jev
	if eng == nil {
		eng = jev.DeterministicEngine{}
	}
	state := map[string]any{
		"objective":    task.Objective,
		"artifactOnly": task.ArtifactOnly,
		"projectId":    task.ProjectID,
	}
	as, err := eng.Assess(ctx, state, jev.DefaultQuestions())
	degraded := false
	if err != nil {
		as, _ = jev.DeterministicEngine{}.Assess(ctx, state, jev.DefaultQuestions())
		degraded = true
	}
	if as.Degraded {
		degraded = true
	}
	dim, _ := json.Marshal(as.Answers)
	usage, _ := json.Marshal(as.Usage)
	rec := &domain.TaskAssessment{
		RunID:          run.ID,
		JevModel:       as.Model,
		QuestionSet:    as.QuestionSet,
		PolicyVersion:  jev.PolicyVersion,
		DimensionsJSON: string(dim),
		InputHash:      as.InputHash,
		UsageJSON:      string(usage),
		Degraded:       degraded,
	}
	if err := e.Store.InsertAssessment(ctx, rec); err != nil {
		return err
	}
	next := domain.RunPlanning
	if jev.Noul(as, "ambiguity") > 0.7 {
		next = domain.RunExploring
	}
	if jev.Noul(as, "artifact_only") > 0.7 {
		task.ArtifactOnly = true
	}
	return e.Store.CompareAndSetRunStatus(ctx, run.ID, domain.RunAssessing, next, "", "")
}

func (e *Engine) runStage(ctx context.Context, run *domain.Run, task *domain.Task, kind domain.StageKind) error {
	stages, err := e.Store.ListStages(ctx, run.ID)
	if err != nil {
		return err
	}
	st := &domain.Stage{RunID: run.ID, Kind: kind, Ordinal: len(stages) + 1, Status: domain.AttemptRunning}
	if err := e.Store.AppendStage(ctx, st); err != nil {
		return err
	}
	cands := []routing.Candidate{}
	if e.Candidates != nil {
		cands, _ = e.Candidates.Candidates(ctx)
	}
	cfg := routing.Config{Profile: run.Profile}
	var assess *jev.Assessment
	if list, err := e.Store.ListAssessments(ctx, run.ID); err == nil && len(list) > 0 {
		a := list[len(list)-1]
		var answers map[string]json.RawMessage
		_ = json.Unmarshal([]byte(a.DimensionsJSON), &answers)
		assess = &jev.Assessment{Model: a.JevModel, Answers: answers, InputHash: a.InputHash, QuestionSet: a.QuestionSet, Degraded: a.Degraded}
	}
	router := e.Router
	if router == nil {
		router = &routing.Router{}
	}
	var dec routing.Decision
	if kind != domain.StageValidate {
		dec = router.Route(ctx, kind, cfg, cands, assess)
		if dec.Blocked != "" {
			return e.Store.UpdateRunStatus(ctx, run.ID, domain.RunBlocked, dec.Blocked, dec.Detail)
		}
	}
	attempts, _ := e.Store.ListAttempts(ctx, st.ID)
	now := time.Now().UTC()
	att := &domain.StageAttempt{StageID: st.ID, RunID: run.ID, Ordinal: len(attempts) + 1, Status: domain.AttemptRunning, StartedAt: &now}
	att.HarnessID = dec.Candidate.Harness.ID
	att.ModelID = dec.Candidate.ModelID
	if err := e.Store.AppendAttempt(ctx, att); err != nil {
		return err
	}
	rd := &domain.RouteDecision{
		RunID:         run.ID,
		StageID:       st.ID,
		HarnessID:     dec.Candidate.Harness.ID,
		ModelID:       dec.Candidate.ModelID,
		Profile:       run.Profile,
		Isolation:     dec.Candidate.Isolation,
		FallbacksJSON: routing.FallbacksJSON(dec.Fallbacks),
		PolicyVersion: jev.PolicyVersion,
		Reason:        dec.Reason,
		Degraded:      dec.Degraded || run.DegradedRouting,
	}
	if assess != nil {
		// assessment id is not threaded; reason still recorded
	}
	_ = e.Store.InsertRouteDecision(ctx, rd)

	if kind.WritesWorkspace() && e.Workspace != nil {
		proj, err := e.Store.GetProject(ctx, run.ProjectID)
		if err != nil {
			return err
		}
		if _, _, err := e.Workspace.Prepare(ctx, *proj, *run); err != nil {
			_ = e.Store.UpdateAttemptStatus(ctx, att.ID, domain.AttemptFailed, domain.FailInfrastructure, err.Error())
			return e.retryOrFail(ctx, run, st, att, kind, dec, domain.FailInfrastructure, err)
		}
	}

	res := StageResult{}
	if kind == domain.StageValidate {
		wsPath := ""
		if ws, err := e.Store.GetWorkspaceByRun(ctx, run.ID); err == nil {
			wsPath = ws.RunPath
		} else {
			if proj, err := e.Store.GetProject(ctx, run.ProjectID); err == nil {
				wsPath = proj.Path
			}
		}
		checks := validation.Discover(wsPath)
		plan, _ := loadPlan(ctx, e.Store, run.ID)
		if plan != nil {
			for _, cmd := range plan.ValidationPlan {
				checks = append(checks, artifacts.ValidationCheck{Name: cmd, Kind: "task", Command: cmd, Required: false, Status: string(domain.CheckNotVerified)})
			}
		}
		runner := e.Validate
		if runner == nil {
			runner = &validation.Runner{Store: e.Store}
		}
		art := runner.Run(ctx, wsPath, checks, nil)
		res.ArtifactJSON, _ = artifacts.Marshal(art)
	} else if e.Exec != nil {
		ws, _ := e.Store.GetWorkspaceByRun(ctx, run.ID)
		res = mustExec(ctx, e.Exec, StageRequest{Run: *run, Task: *task, Stage: *st, Attempt: *att, Route: dec.Candidate, Workspace: ws})
	} else {
		res.ArtifactJSON, res.Err = syntheticArtifact(kind, task.Objective)
	}
	if res.Usage != nil {
		res.Usage.RunID = run.ID
		res.Usage.StageID = st.ID
		res.Usage.AttemptID = att.ID
		_ = e.Store.InsertUsage(ctx, res.Usage)
	}
	if res.Err != nil {
		class := res.Class
		if class == "" {
			class = domain.FailInfrastructure
		}
		_ = e.Store.UpdateAttemptStatus(ctx, att.ID, domain.AttemptFailed, class, res.Err.Error())
		return e.retryOrFail(ctx, run, st, att, kind, dec, class, res.Err)
	}
	akind := artifacts.KindForStage(kind)
	parsed, perr := artifacts.ParseAndValidate(akind, res.ArtifactJSON)
	valid := perr == nil
	body := res.ArtifactJSON
	if valid {
		if s, err := artifacts.Marshal(parsed); err == nil {
			body = s
		}
	}
	if !valid {
		// bounded correction: one retry of the same attempt lineage as a NEW attempt
		_ = e.Store.UpdateAttemptStatus(ctx, att.ID, domain.AttemptInvalid, domain.FailTask, perr.Error())
		corr := &domain.StageAttempt{StageID: st.ID, RunID: run.ID, Ordinal: att.Ordinal + 1, Status: domain.AttemptRunning, HarnessID: att.HarnessID, ModelID: att.ModelID}
		now := time.Now().UTC()
		corr.StartedAt = &now
		if err := e.Store.AppendAttempt(ctx, corr); err != nil {
			return err
		}
		var res2 StageResult
		if e.Exec != nil {
			ws, _ := e.Store.GetWorkspaceByRun(ctx, run.ID)
			res2 = mustExec(ctx, e.Exec, StageRequest{Run: *run, Task: *task, Stage: *st, Attempt: *corr, Route: dec.Candidate, Workspace: ws})
		} else {
			res2.ArtifactJSON, res2.Err = syntheticArtifact(kind, task.Objective)
		}
		parsed, perr = artifacts.ParseAndValidate(akind, res2.ArtifactJSON)
		if perr != nil {
			_ = e.Store.UpdateAttemptStatus(ctx, corr.ID, domain.AttemptInvalid, domain.FailTask, perr.Error())
			return e.Store.UpdateRunStatus(ctx, run.ID, domain.RunFailed, "", "invalid stage output")
		}
		body, _ = artifacts.Marshal(parsed)
		valid = true
		_ = e.Store.UpdateAttemptStatus(ctx, corr.ID, domain.AttemptSucceeded, "", "")
		att = corr
	} else {
		_ = e.Store.UpdateAttemptStatus(ctx, att.ID, domain.AttemptSucceeded, "", "")
	}
	art := &domain.Artifact{RunID: run.ID, StageID: st.ID, AttemptID: att.ID, Kind: akind, SchemaVer: artifacts.SchemaVersion, JSON: body, Valid: valid}
	if err := e.Store.InsertArtifact(ctx, art); err != nil {
		return err
	}
	if kind.WritesWorkspace() {
		e.recordDelta(ctx, run.ID)
	}
	return e.advance(ctx, run, task, kind, parsed)
}

func (e *Engine) recordDelta(ctx context.Context, runID string) {
	ws, err := e.Store.GetWorkspaceByRun(ctx, runID)
	if err != nil {
		return
	}
	snap, err := workspace.LoadSnapshot(filepath.Join(filepath.Dir(ws.RunPath), "snapshot"))
	if err != nil {
		return
	}
	d, err := workspace.ComputeDelta(snap, ws.RunPath)
	if err != nil {
		return
	}
	b, _ := json.Marshal(d)
	_ = e.Store.InsertRunDelta(ctx, runID, string(b), "")
}

func (e *Engine) advance(ctx context.Context, run *domain.Run, task *domain.Task, finished domain.StageKind, parsed any) error {
	from := statusFor(finished)
	switch finished {
	case domain.StageExplore:
		return e.Store.CompareAndSetRunStatus(ctx, run.ID, from, domain.RunPlanning, "", "")
	case domain.StagePlan, domain.StageReplan:
		p, _ := parsed.(artifacts.PlanArtifact)
		if p.ArtifactOnly {
			task.ArtifactOnly = true
		}
		return e.Store.CompareAndSetRunStatus(ctx, run.ID, from, domain.RunExecuting, "", "")
	case domain.StageExecute, domain.StageRepair:
		return e.Store.CompareAndSetRunStatus(ctx, run.ID, from, domain.RunValidating, "", "")
	case domain.StageValidate:
		return e.Store.CompareAndSetRunStatus(ctx, run.ID, from, domain.RunReviewing, "", "")
	case domain.StageReview:
		plan, _ := loadPlan(ctx, e.Store, run.ID)
		val, _ := loadVal(ctx, e.Store, run.ID)
		rev, _ := parsed.(artifacts.ReviewArtifact)
		outcome := CompletionPolicy(task.ArtifactOnly, plan, val, &rev, 0, false)
		return e.applyOutcome(ctx, run, from, outcome)
	default:
		return nil
	}
}

func (e *Engine) applyOutcome(ctx context.Context, run *domain.Run, from domain.RunStatus, o domain.CompletionOutcome) error {
	switch o {
	case domain.OutcomeComplete:
		return e.Store.CompareAndSetRunStatus(ctx, run.ID, from, domain.RunComplete, "", "")
	case domain.OutcomeRepair:
		return e.Store.CompareAndSetRunStatus(ctx, run.ID, from, domain.RunRepairing, "", "")
	case domain.OutcomeReplan:
		return e.Store.CompareAndSetRunStatus(ctx, run.ID, from, domain.RunReplanning, "", "")
	case domain.OutcomeExplore:
		return e.Store.CompareAndSetRunStatus(ctx, run.ID, from, domain.RunExploring, "", "")
	case domain.OutcomeIntegrate:
		return e.Store.CompareAndSetRunStatus(ctx, run.ID, from, domain.RunReadyToIntegrate, "", "")
	case domain.OutcomeBlocked:
		return e.Store.CompareAndSetRunStatus(ctx, run.ID, from, domain.RunBlocked, domain.BlockedPolicy, "completion policy blocked")
	case domain.OutcomeFailed:
		return e.Store.CompareAndSetRunStatus(ctx, run.ID, from, domain.RunFailed, "", "")
	default:
		return e.Store.CompareAndSetRunStatus(ctx, run.ID, from, domain.RunFailed, "", string(o))
	}
}

func (e *Engine) integrate(ctx context.Context, run *domain.Run, task *domain.Task) error {
	if task.ArtifactOnly {
		return e.Store.UpdateRunStatus(ctx, run.ID, domain.RunComplete, "", "")
	}
	_ = e.Store.CompareAndSetRunStatus(ctx, run.ID, domain.RunReadyToIntegrate, domain.RunIntegrating, "", "")
	if e.Integrate == nil {
		return e.Store.UpdateRunStatus(ctx, run.ID, domain.RunComplete, "", "no integrator (artifact treated complete)")
	}
	ws, err := e.Store.GetWorkspaceByRun(ctx, run.ID)
	if err != nil {
		return e.Store.UpdateRunStatus(ctx, run.ID, domain.RunIntegrationBlocked, domain.BlockedIntegration, err.Error())
	}
	in, err := e.Integrate.Integrate(ctx, *run, ws)
	if err != nil {
		_ = e.Store.UpdateRunStatus(ctx, run.ID, domain.RunIntegrationBlocked, domain.BlockedIntegration, err.Error())
		return nil
	}
	if in != nil && (in.Status == "blocked" || in.Status == "diverged" || in.Status == "incomplete") {
		return e.Store.UpdateRunStatus(ctx, run.ID, domain.RunIntegrationBlocked, domain.BlockedIntegration, in.Error)
	}
	return e.Store.UpdateRunStatus(ctx, run.ID, domain.RunComplete, "", "")
}

func (e *Engine) retryOrFail(ctx context.Context, run *domain.Run, st *domain.Stage, att *domain.StageAttempt, kind domain.StageKind, dec routing.Decision, class domain.FailureClass, err error) error {
	if class == domain.FailInfrastructure && len(dec.Fallbacks) > 0 {
		fb := dec.Fallbacks[0]
		now := time.Now().UTC()
		natt := &domain.StageAttempt{StageID: st.ID, RunID: run.ID, Ordinal: att.Ordinal + 1, Status: domain.AttemptRunning, HarnessID: fb.Harness.ID, ModelID: fb.ModelID, StartedAt: &now}
		if err := e.Store.AppendAttempt(ctx, natt); err != nil {
			return err
		}
		return fmt.Errorf("fallback scheduled: %w", err)
	}
	if class == domain.FailTask {
		if kind == domain.StageExecute {
			return e.Store.UpdateRunStatus(ctx, run.ID, domain.RunRepairing, "", err.Error())
		}
		return e.Store.UpdateRunStatus(ctx, run.ID, domain.RunFailed, "", err.Error())
	}
	return e.Store.UpdateRunStatus(ctx, run.ID, domain.RunFailed, "", err.Error())
}

func mustExec(ctx context.Context, x StageExec, req StageRequest) StageResult {
	res, err := x.Execute(ctx, req)
	if err != nil && res.Err == nil {
		res.Err = err
	}
	return res
}

func syntheticArtifact(kind domain.StageKind, objective string) (string, error) {
	switch kind {
	case domain.StagePlan, domain.StageReplan:
		p := artifacts.PlanArtifact{Kind: "plan", Objective: objective, AcceptanceCriteria: []string{"satisfies request"}, ValidationPlan: []string{"go test ./..."}}
		return artifacts.Marshal(p)
	case domain.StageExplore:
		return artifacts.Marshal(artifacts.InvestigationArtifact{Kind: "investigation", Question: objective, Findings: []string{"synthetic"}})
	case domain.StageExecute, domain.StageRepair:
		return artifacts.Marshal(artifacts.ImplementationReport{Kind: "implementation_report", Summary: "synthetic", ExpectedValidation: []string{"go test ./..."}})
	case domain.StageValidate:
		return artifacts.Marshal(artifacts.ValidationArtifact{Kind: "validation", Checks: []artifacts.ValidationCheck{{Name: "synthetic", Required: false, Status: string(domain.CheckSkipped)}}})
	case domain.StageReview:
		return artifacts.Marshal(artifacts.ReviewArtifact{Kind: "review", Verdict: "pass"})
	default:
		return "", errors.New("no synthetic artifact")
	}
}

func statusFor(k domain.StageKind) domain.RunStatus {
	switch k {
	case domain.StagePlan:
		return domain.RunPlanning
	case domain.StageExplore:
		return domain.RunExploring
	case domain.StageExecute:
		return domain.RunExecuting
	case domain.StageValidate:
		return domain.RunValidating
	case domain.StageReview:
		return domain.RunReviewing
	case domain.StageRepair:
		return domain.RunRepairing
	case domain.StageReplan:
		return domain.RunReplanning
	case domain.StageIntegrate:
		return domain.RunIntegrating
	default:
		return domain.RunQueued
	}
}

func loadPlan(ctx context.Context, st *storage.Store, runID string) (*artifacts.PlanArtifact, error) {
	a, err := st.LatestArtifact(ctx, runID, domain.ArtifactPlan)
	if err != nil {
		return nil, err
	}
	v, err := artifacts.ParseAndValidate(domain.ArtifactPlan, a.JSON)
	if err != nil {
		return nil, err
	}
	p := v.(artifacts.PlanArtifact)
	return &p, nil
}

func loadVal(ctx context.Context, st *storage.Store, runID string) (*artifacts.ValidationArtifact, error) {
	a, err := st.LatestArtifact(ctx, runID, domain.ArtifactValidation)
	if err != nil {
		return nil, err
	}
	v, err := artifacts.ParseAndValidate(domain.ArtifactValidation, a.JSON)
	if err != nil {
		return nil, err
	}
	p := v.(artifacts.ValidationArtifact)
	return &p, nil
}

func NewID() string { return id.New() }
