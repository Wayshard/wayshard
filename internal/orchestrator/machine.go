package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"path/filepath"
	"time"

	"github.com/Wayshard/wayshard/internal/artifacts"
	"github.com/Wayshard/wayshard/internal/ctxengine"
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
	Store         *storage.Store
	Jev           jev.DecisionEngine
	Router        *routing.Router
	Exec          StageExec
	Workspace     WorkspacePrep
	Integrate     Integrator
	Candidates    CandidateSource
	Validate      *validation.Runner
	Context       *ctxengine.Engine
	ContextBudget int
	Log           *slog.Logger
	Budget        Budgets
}

func (e *Engine) budgets() Budgets { return e.Budget.withDefaults() }

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
		if ctx.Err() != nil {
			return e.markCancelled(ctx, run.ID)
		}
		switch run.Status {
		case domain.RunPlanning, domain.RunExecuting, domain.RunValidating,
			domain.RunReviewing, domain.RunRepairing, domain.RunReplanning, domain.RunExploring:
			if run.Status == domain.RunExecuting {
				// Establish the baseline on the untouched snapshot before the
				// first write stage, so pre-existing failures are not mistaken
				// for agent regressions.
				e.ensureBaseline(ctx, run, task)
			}
			if err := e.runStage(ctx, run, task, stageForStatus(run.Status)); err != nil {
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

func (e *Engine) markCancelled(ctx context.Context, runID string) error {
	// A cancellation may already have been recorded by the API path. CancelRun
	// is transactional: run status, running attempts, running stages and
	// pending approvals move together, so a crash cannot leave a cancelled run
	// with a running attempt.
	_, _ = e.Store.CancelRun(context.WithoutCancel(ctx), runID, "cancelled")
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

func stageForStatus(s domain.RunStatus) domain.StageKind {
	switch s {
	case domain.RunPlanning:
		return domain.StagePlan
	case domain.RunExploring:
		return domain.StageExplore
	case domain.RunExecuting:
		return domain.StageExecute
	case domain.RunValidating:
		return domain.StageValidate
	case domain.RunReviewing:
		return domain.StageReview
	case domain.RunRepairing:
		return domain.StageRepair
	case domain.RunReplanning:
		return domain.StageReplan
	default:
		return domain.StagePlan
	}
}

// ensureBaseline runs the discovered mandatory checks once against the
// untouched run workspace and persists the outcome as a baseline validation
// artifact. Discovery stays passive; execution routes through the validation
// runner (Tool Sandbox).
func (e *Engine) ensureBaseline(ctx context.Context, run *domain.Run, task *domain.Task) {
	if task.ArtifactOnly || run.Status != domain.RunExecuting {
		return
	}
	if e.hasBaseline(ctx, run.ID) {
		return
	}
	ws, err := e.Store.GetWorkspaceByRun(ctx, run.ID)
	if err != nil || ws == nil {
		return
	}
	if e.Workspace != nil {
		if proj, err := e.Store.GetProject(ctx, run.ProjectID); err == nil {
			if _, _, err := e.Workspace.Prepare(ctx, *proj, *run); err == nil {
				if w2, err := e.Store.GetWorkspaceByRun(ctx, run.ID); err == nil {
					ws = w2
				}
			}
		}
	}
	runner := e.Validate
	if runner == nil {
		runner = &validation.Runner{Store: e.Store}
	}
	// Baseline runs in a disposable copy of the task-start snapshot so its
	// side effects cannot reach the authoritative run workspace or RunDelta.
	vw, err := e.baselineValidationWorkspace(ctx, run.ID)
	if err != nil {
		return
	}
	defer vw.Cleanup()
	checks := validation.Discover(vw.Dir)
	if len(checks) == 0 {
		return
	}
	art := runner.Run(ctx, vw.Dir, checks, nil)
	art.Baseline = true
	body, err := artifacts.Marshal(art)
	if err != nil {
		return
	}
	stages, _ := e.Store.ListStages(ctx, run.ID)
	stageID := ""
	if len(stages) > 0 {
		stageID = stages[0].ID
	}
	_ = e.Store.InsertArtifact(ctx, &domain.Artifact{
		RunID: run.ID, StageID: stageID, Kind: domain.ArtifactValidation,
		SchemaVer: artifacts.SchemaVersion, JSON: body, Valid: true,
	})
}

func (e *Engine) hasBaseline(ctx context.Context, runID string) bool {
	arts, err := e.Store.ListArtifacts(ctx, runID)
	if err != nil {
		return false
	}
	for _, a := range arts {
		if a.Kind != domain.ArtifactValidation {
			continue
		}
		if v, err := artifacts.ParseAndValidate(domain.ArtifactValidation, a.JSON); err == nil {
			if va, ok := v.(artifacts.ValidationArtifact); ok && va.Baseline {
				return true
			}
		}
	}
	return false
}

func (e *Engine) baselineChecks(ctx context.Context, runID string) map[string]string {
	arts, err := e.Store.ListArtifacts(ctx, runID)
	if err != nil {
		return nil
	}
	for _, a := range arts {
		if a.Kind != domain.ArtifactValidation {
			continue
		}
		v, err := artifacts.ParseAndValidate(domain.ArtifactValidation, a.JSON)
		if err != nil {
			continue
		}
		va, ok := v.(artifacts.ValidationArtifact)
		if !ok || !va.Baseline {
			continue
		}
		out := map[string]string{}
		for _, c := range va.Checks {
			out[c.Name] = c.Status
		}
		return out
	}
	return nil
}

// runStage appends one stage and, within its attempt budget, executes it. The
// stage always leaves the running state when this returns.
func (e *Engine) runStage(ctx context.Context, run *domain.Run, task *domain.Task, kind domain.StageKind) error {
	if reason, detail, over := e.budget(ctx, kind, run.ID); over {
		return e.stopBlocked(ctx, run, reason, detail)
	}
	stages, err := e.Store.ListStages(ctx, run.ID)
	if err != nil {
		return err
	}
	st := &domain.Stage{RunID: run.ID, Kind: kind, Ordinal: len(stages) + 1, Status: domain.AttemptRunning}
	if err := e.Store.AppendStage(ctx, st); err != nil {
		return err
	}
	if ctx.Err() != nil {
		_ = e.Store.UpdateStageStatus(ctx, st.ID, domain.AttemptCancelled, domain.FailUser, "cancelled")
		return e.markCancelled(ctx, run.ID)
	}

	cands := []routing.Candidate{}
	if e.Candidates != nil {
		cands, _ = e.Candidates.Candidates(ctx)
	}
	cfg := routing.Config{
		Profile: run.Profile,
	}
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
			_ = e.Store.UpdateStageStatus(ctx, st.ID, domain.AttemptFailed, domain.FailPolicy, dec.Detail)
			return e.Store.UpdateRunStatus(ctx, run.ID, domain.RunBlocked, dec.Blocked, dec.Detail)
		}
	}

	if kind != domain.StageValidate && e.Workspace != nil {
		proj, err := e.Store.GetProject(ctx, run.ProjectID)
		if err != nil {
			_ = e.Store.UpdateStageStatus(ctx, st.ID, domain.AttemptFailed, domain.FailData, err.Error())
			return e.Store.UpdateRunStatus(ctx, run.ID, domain.RunFailed, "", err.Error())
		}
		if _, _, err := e.Workspace.Prepare(ctx, *proj, *run); err != nil {
			_ = e.Store.UpdateStageStatus(ctx, st.ID, domain.AttemptFailed, domain.FailInfrastructure, err.Error())
			return e.Store.UpdateRunStatus(ctx, run.ID, domain.RunFailed, "", err.Error())
		}
	}

	if kind == domain.StageValidate {
		return e.runValidate(ctx, run, task, st)
	}

	// Build the stage-specific context bundle actually delivered to the
	// harness (planner/executor/reviewer/repair/explore differ).
	bundle := e.stageBundle(ctx, run, task, st)

	// Build the attempt ladder: primary then explicit infrastructure fallbacks.
	candidates := []routing.Candidate{dec.Candidate}
	candidates = append(candidates, dec.Fallbacks...)
	maxAttempts := e.budgets().MaxStageAttempts

	attemptsMade := 0
	corrections := 0
	for _, cand := range candidates {
		if attemptsMade >= maxAttempts {
			break
		}
		if ctx.Err() != nil {
			_ = e.Store.UpdateStageStatus(ctx, st.ID, domain.AttemptCancelled, domain.FailUser, "cancelled")
			return e.markCancelled(ctx, run.ID)
		}
		att := e.appendAttempt(ctx, st, run, cand)
		if att == nil {
			return e.Store.UpdateRunStatus(ctx, run.ID, domain.RunFailed, "", "cannot append attempt")
		}
		if st.Kind.WritesWorkspace() {
			if err := e.checkpointBeforeWrite(ctx, run, st, att); err != nil {
				_ = e.Store.UpdateAttemptStatus(ctx, att.ID, domain.AttemptFailed, domain.FailInfrastructure, err.Error())
				_ = e.Store.UpdateStageStatus(ctx, st.ID, domain.AttemptFailed, domain.FailInfrastructure, err.Error())
				return e.Store.UpdateRunStatus(ctx, run.ID, domain.RunFailed, "", "checkpoint: "+err.Error())
			}
		}
		e.insertRouteDecision(ctx, run, st, att, cand, dec, assess)
		res, adec := e.execAttempt(ctx, run, task, st, att, cand, bundle)
		attemptsMade++
		if res.Usage != nil {
			res.Usage.RunID = run.ID
			res.Usage.StageID = st.ID
			res.Usage.AttemptID = att.ID
			_ = e.Store.InsertUsage(ctx, res.Usage)
		}
		if ctx.Err() != nil {
			_ = e.Store.UpdateAttemptStatus(ctx, att.ID, domain.AttemptCancelled, domain.FailUser, "cancelled")
			_ = e.Store.UpdateStageStatus(ctx, st.ID, domain.AttemptCancelled, domain.FailUser, "cancelled")
			return e.markCancelled(ctx, run.ID)
		}
		if res.Err != nil {
			class := res.Class
			if class == "" {
				class = domain.FailInfrastructure
			}
			_ = e.Store.UpdateAttemptStatus(ctx, att.ID, domain.AttemptFailed, class, res.Err.Error())
			// Infrastructure failure may fall back to the next viable route.
			if class == domain.FailInfrastructure && attemptsMade < maxAttempts {
				continue
			}
			dec2 := adec
			return e.failStage(ctx, run, st, kind, class, res.Err, dec2)
		}

		parsed, perr := e.persistArtifact(ctx, run.ID, st.ID, att.ID, kind, res.ArtifactJSON)
		if perr == nil {
			_ = e.Store.UpdateAttemptStatus(ctx, att.ID, domain.AttemptSucceeded, "", "")
			_ = e.Store.UpdateStageStatus(ctx, st.ID, domain.AttemptSucceeded, "", "")
			if kind.WritesWorkspace() {
				e.recordDelta(ctx, run.ID)
			}
			return e.advance(ctx, run, task, kind, parsed)
		}

		// Invalid structured output: one bounded correction attempt.
		_ = e.Store.UpdateAttemptStatus(ctx, att.ID, domain.AttemptInvalid, domain.FailTask, perr.Error())
		if corrections < 1 && attemptsMade < maxAttempts {
			corrections++
			corr := e.appendAttempt(ctx, st, run, cand)
			if corr == nil {
				break
			}
			if st.Kind.WritesWorkspace() {
				if err := e.checkpointBeforeWrite(ctx, run, st, corr); err != nil {
					_ = e.Store.UpdateAttemptStatus(ctx, corr.ID, domain.AttemptFailed, domain.FailInfrastructure, err.Error())
					_ = e.Store.UpdateStageStatus(ctx, st.ID, domain.AttemptFailed, domain.FailInfrastructure, err.Error())
					return e.Store.UpdateRunStatus(ctx, run.ID, domain.RunFailed, "", "checkpoint: "+err.Error())
				}
			}
			res2, _ := e.execAttempt(ctx, run, task, st, corr, cand, bundle)
			attemptsMade++
			if res2.Err == nil {
				if parsed2, perr2 := e.persistArtifact(ctx, run.ID, st.ID, corr.ID, kind, res2.ArtifactJSON); perr2 == nil {
					_ = e.Store.UpdateAttemptStatus(ctx, corr.ID, domain.AttemptSucceeded, "", "")
					_ = e.Store.UpdateStageStatus(ctx, st.ID, domain.AttemptSucceeded, "", "")
					if kind.WritesWorkspace() {
						e.recordDelta(ctx, run.ID)
					}
					return e.advance(ctx, run, task, kind, parsed2)
				}
			}
			_ = e.Store.UpdateAttemptStatus(ctx, corr.ID, domain.AttemptInvalid, domain.FailTask, "invalid stage output")
		}
		_ = e.Store.UpdateStageStatus(ctx, st.ID, domain.AttemptFailed, domain.FailTask, "invalid stage output")
		return e.Store.UpdateRunStatus(ctx, run.ID, domain.RunFailed, "", "invalid stage output")
	}

	// No viable attempt completed: infrastructure retry budget exhausted.
	_ = e.Store.UpdateStageStatus(ctx, st.ID, domain.AttemptFailed, domain.FailInfrastructure, "attempt budget exhausted")
	return e.Store.UpdateRunStatus(ctx, run.ID, domain.RunFailed, "", "infrastructure attempt budget exhausted")
}

func (e *Engine) runValidate(ctx context.Context, run *domain.Run, task *domain.Task, st *domain.Stage) error {
	att := e.appendAttempt(ctx, st, run, routing.Candidate{})
	if att == nil {
		return e.Store.UpdateRunStatus(ctx, run.ID, domain.RunFailed, "", "cannot append attempt")
	}
	// Final validation inspects an isolated copy of the authoritative candidate
	// so its writes can never alter the run workspace, RunDelta or integration.
	vw, verr := e.finalValidationWorkspace(ctx, run.ID)
	if verr != nil {
		_ = e.Store.UpdateAttemptStatus(ctx, att.ID, domain.AttemptFailed, domain.FailInfrastructure, verr.Error())
		_ = e.Store.UpdateStageStatus(ctx, st.ID, domain.AttemptFailed, domain.FailInfrastructure, verr.Error())
		return e.Store.UpdateRunStatus(ctx, run.ID, domain.RunFailed, "", "validation workspace unavailable: "+verr.Error())
	}
	defer vw.Cleanup()
	checks := validation.Discover(vw.Dir)
	plan, _ := loadPlan(ctx, e.Store, run.ID)
	if plan != nil {
		for _, cmd := range plan.ValidationPlan {
			checks = append(checks, artifacts.ValidationCheck{Name: cmd, Kind: "task", Command: cmd, Required: false, Status: string(domain.CheckNotVerified)})
		}
	}
	baseline := e.baselineChecks(ctx, run.ID)
	runner := e.Validate
	if runner == nil {
		runner = &validation.Runner{Store: e.Store}
	}
	art := runner.Run(ctx, vw.Dir, checks, baseline)
	body, err := artifacts.Marshal(art)
	if err != nil {
		return err
	}
	res := StageResult{ArtifactJSON: body}
	if ctx.Err() != nil {
		_ = e.Store.UpdateAttemptStatus(ctx, att.ID, domain.AttemptCancelled, domain.FailUser, "cancelled")
		_ = e.Store.UpdateStageStatus(ctx, st.ID, domain.AttemptCancelled, domain.FailUser, "cancelled")
		return e.markCancelled(ctx, run.ID)
	}
	parsed, perr := e.persistArtifact(ctx, run.ID, st.ID, att.ID, domain.StageValidate, res.ArtifactJSON)
	if perr != nil {
		_ = e.Store.UpdateAttemptStatus(ctx, att.ID, domain.AttemptInvalid, domain.FailTask, perr.Error())
		_ = e.Store.UpdateStageStatus(ctx, st.ID, domain.AttemptFailed, domain.FailTask, perr.Error())
		return e.Store.UpdateRunStatus(ctx, run.ID, domain.RunFailed, "", "invalid validation artifact")
	}
	_ = e.Store.UpdateAttemptStatus(ctx, att.ID, domain.AttemptSucceeded, "", "")
	_ = e.Store.UpdateStageStatus(ctx, st.ID, domain.AttemptSucceeded, "", "")
	return e.advance(ctx, run, task, domain.StageValidate, parsed)
}

func (e *Engine) appendAttempt(ctx context.Context, st *domain.Stage, run *domain.Run, cand routing.Candidate) *domain.StageAttempt {
	attempts, _ := e.Store.ListAttempts(ctx, st.ID)
	now := time.Now().UTC()
	att := &domain.StageAttempt{StageID: st.ID, RunID: run.ID, Ordinal: len(attempts) + 1, Status: domain.AttemptRunning, StartedAt: &now}
	att.HarnessID = cand.Harness.ID
	att.ModelID = cand.ModelID
	if err := e.Store.AppendAttempt(ctx, att); err != nil {
		return nil
	}
	return att
}

func (e *Engine) insertRouteDecision(ctx context.Context, run *domain.Run, st *domain.Stage, att *domain.StageAttempt, cand routing.Candidate, dec routing.Decision, assess *jev.Assessment) {
	rd := &domain.RouteDecision{
		RunID:         run.ID,
		StageID:       st.ID,
		HarnessID:     cand.Harness.ID,
		ModelID:       cand.ModelID,
		Profile:       run.Profile,
		FallbacksJSON: routing.FallbacksJSON(dec.Fallbacks),
		PolicyVersion: jev.PolicyVersion,
		Reason:        dec.Reason,
		Degraded:      dec.Degraded || run.DegradedRouting,
	}
	_ = e.Store.InsertRouteDecision(ctx, rd)
}

func (e *Engine) execAttempt(ctx context.Context, run *domain.Run, task *domain.Task, st *domain.Stage, att *domain.StageAttempt, cand routing.Candidate, bundle string) (StageResult, routing.Decision) {
	if e.Exec == nil {
		body, err := syntheticArtifact(st.Kind, task.Objective)
		return StageResult{ArtifactJSON: body, Err: err}, routing.Decision{Candidate: cand}
	}
	ws, _ := e.Store.GetWorkspaceByRun(ctx, run.ID)
	return mustExec(ctx, e.Exec, StageRequest{Run: *run, Task: *task, Stage: *st, Attempt: *att, Route: cand, Workspace: ws, Bundle: bundle}), routing.Decision{Candidate: cand}
}

func (e *Engine) persistArtifact(ctx context.Context, runID, stageID, attemptID string, kind domain.StageKind, raw string) (any, error) {
	akind := artifacts.KindForStage(kind)
	parsed, perr := artifacts.ParseAndValidate(akind, raw)
	if perr != nil {
		return nil, perr
	}
	body := raw
	if s, err := artifacts.Marshal(parsed); err == nil {
		body = s
	}
	if err := e.Store.InsertArtifact(ctx, &domain.Artifact{RunID: runID, StageID: stageID, AttemptID: attemptID, Kind: akind, SchemaVer: artifacts.SchemaVersion, JSON: body, Valid: true}); err != nil {
		return nil, err
	}
	return parsed, nil
}

func (e *Engine) failStage(ctx context.Context, run *domain.Run, st *domain.Stage, kind domain.StageKind, class domain.FailureClass, err error, dec routing.Decision) error {
	_ = e.Store.UpdateStageStatus(ctx, st.ID, domain.AttemptFailed, class, err.Error())
	if class == domain.FailTask {
		if kind == domain.StageExecute {
			return e.Store.UpdateRunStatus(ctx, run.ID, domain.RunRepairing, "", err.Error())
		}
		return e.Store.UpdateRunStatus(ctx, run.ID, domain.RunFailed, "", err.Error())
	}
	if class == domain.FailPolicy {
		return e.Store.UpdateRunStatus(ctx, run.ID, domain.RunBlocked, domain.BlockedPolicy, err.Error())
	}
	return e.Store.UpdateRunStatus(ctx, run.ID, domain.RunFailed, "", err.Error())
}

func (e *Engine) stopBlocked(ctx context.Context, run *domain.Run, reason domain.BlockedReason, detail string) error {
	_ = e.Store.ResetStaleRunningStages(ctx, run.ID)
	return e.Store.UpdateRunStatus(ctx, run.ID, domain.RunBlocked, reason, detail)
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
		if p, ok := parsed.(artifacts.PlanArtifact); ok && p.ArtifactOnly {
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
		pending, _ := e.Store.PendingApprovalCount(ctx, run.ID)
		outcome := CompletionPolicy(task.ArtifactOnly, plan, val, &rev, pending, false)
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
	if ctx.Err() != nil {
		return e.markCancelled(ctx, run.ID)
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

func mustExec(ctx context.Context, x StageExec, req StageRequest) StageResult {
	res, err := x.Execute(ctx, req)
	if err != nil && res.Err == nil {
		res.Err = err
	}
	if res.Class == "" && res.Err != nil {
		res.Class = domain.FailInfrastructure
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
	arts, err := st.ListArtifacts(ctx, runID)
	if err != nil {
		return nil, err
	}
	for i := len(arts) - 1; i >= 0; i-- {
		a := arts[i]
		if a.Kind != domain.ArtifactValidation || !a.Valid {
			continue
		}
		v, err := artifacts.ParseAndValidate(domain.ArtifactValidation, a.JSON)
		if err != nil {
			continue
		}
		va, ok := v.(artifacts.ValidationArtifact)
		if !ok || va.Baseline {
			continue
		}
		return &va, nil
	}
	return nil, storage.ErrNotFound
}

func NewID() string { return id.New() }
