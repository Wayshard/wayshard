package harness

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Wayshard/wayshard/internal/acp"
	"github.com/Wayshard/wayshard/internal/artifacts"
	"github.com/Wayshard/wayshard/internal/domain"
	"github.com/Wayshard/wayshard/internal/orchestrator"
	"github.com/Wayshard/wayshard/internal/storage"
)

// ACPExec runs a stage against an ACP harness. Validate and Integrate are not
// executed here. The harness runs as the Wayshard server OS user with its normal
// configuration, authentication, environment, filesystem and network access.
type ACPExec struct {
	Store *storage.Store
	Log   *slog.Logger
	// Catalog is the effective harness catalog used to resolve a route's
	// DefinitionID. When nil it is loaded lazily from the shipped + user catalog.
	Catalog *Catalog

	catalogOnce sync.Once
	catalogVal  *Catalog
}

// definition resolves a harness definition from the effective catalog.
func (e *ACPExec) definition(defID string) (Definition, bool) {
	e.catalogOnce.Do(func() {
		if e.Catalog != nil {
			e.catalogVal = e.Catalog
			return
		}
		cat, err := LoadCatalog("")
		if err != nil {
			cat = &Catalog{}
		}
		e.catalogVal = cat
	})
	return e.catalogVal.ByID(defID)
}

func (e *ACPExec) Execute(ctx context.Context, req orchestrator.StageRequest) (orchestrator.StageResult, error) {
	if req.Stage.Kind == domain.StageValidate || req.Stage.Kind == domain.StageIntegrate {
		return orchestrator.StageResult{Err: fmt.Errorf("stage %s is server-owned", req.Stage.Kind)}, nil
	}
	cwd := ""
	if req.Workspace != nil {
		cwd = req.Workspace.RunPath
	}
	// Resolve the harness definition from the effective catalog. A route whose
	// definition is missing cannot be launched and fails as a policy outcome.
	def, ok := e.definition(req.Route.Harness.DefinitionID)
	if !ok {
		err := fmt.Errorf("no harness definition %q in the effective catalog", req.Route.Harness.DefinitionID)
		return orchestrator.StageResult{Class: domain.FailPolicy, Err: err}, err
	}
	exe := req.Route.Harness.Executable
	if exe == "" {
		exe = req.Route.Harness.DisplayName
	}
	spec := acp.Spec{Command: exe, Args: definitionACPArgs(def), Dir: cwd}
	spec.Env = mergeEnv(os.Environ(), harnessExtras(req, exe))

	hooks := acp.Hooks{
		RequestPermission: e.permissionHook(ctx, req),
		ReadTextFile:      e.readHook(req),
		WriteTextFile:     e.writeHook(req),
	}
	if e.Log != nil {
		// Bounded, already-redacted harness diagnostics; debug-level only.
		hooks.OnDiagnostic = func(d acp.Diagnostic) {
			e.Log.Debug("harness diagnostic", "source", d.Source, "text", d.Text)
		}
	}
	tm := newToolManager(req, cwd)
	hooks.CreateTerminal = tm.Create
	hooks.TerminalOutput = tm.Output
	hooks.ReleaseTerminal = tm.Release
	hooks.WaitTerminalExit = tm.WaitExit
	hooks.KillTerminal = tm.Kill
	defer tm.CloseAll()

	drv, err := acp.Launch(ctx, spec, acp.DefaultClientConfig(), hooks, acp.Limits{})
	if err != nil {
		return orchestrator.StageResult{Class: domain.FailInfrastructure, Err: err}, err
	}
	defer drv.Close()
	init, err := drv.Handshake(ctx)
	if err != nil {
		return orchestrator.StageResult{Class: domain.FailInfrastructure, Err: err}, err
	}
	if init != nil && len(init.AuthMethods) > 0 && os.Getenv("WAYSHARD_FAKE_SCENARIO") == "auth_required" {
		return orchestrator.StageResult{Class: domain.FailPolicy, Err: fmt.Errorf("harness auth required")}, fmt.Errorf("harness auth required")
	}
	sess, err := drv.NewSession(ctx, acp.NewSessionRequest{CWD: cwd, MCPServers: []acp.MCPServer{}})
	if err != nil {
		return orchestrator.StageResult{Class: classifyHarnessError(err), Err: err}, err
	}
	if req.Route.ModelID != "" {
		if merr := selectSessionModel(ctx, drv, sess.SessionID, def, req.Route.ModelID); merr != nil {
			if e.Log != nil {
				e.Log.Debug("harness model selection", "model", req.Route.ModelID, "err", merr)
			}
		}
	}
	prompt := req.Task.Objective
	if req.Bundle != "" {
		prompt = req.Bundle + "\n\n" + prompt
	}
	if instr := artifactInstruction(req.Stage.Kind); instr != "" {
		prompt += "\n\n" + instr
	}
	_, err = drv.Prompt(ctx, acp.PromptRequest{
		SessionID: sess.SessionID,
		Prompt:    []acp.ContentBlock{acp.TextBlock(prompt)},
	})
	if err != nil {
		class := classifyHarnessError(err)
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			class = domain.FailUser
		}
		return orchestrator.StageResult{Class: class, Err: err}, err
	}
	text := drv.MessageText()
	if text == "" {
		text = `{"kind":"failure","class":"task","message":"empty harness output"}`
	}
	_ = artifacts.KindForStage(req.Stage.Kind)
	return orchestrator.StageResult{ArtifactJSON: text}, nil
}

// harnessExtras builds the extra environment for a harness launch: the fake
// harness knobs (test fixtures) and any required interpreter PATH directories.
func harnessExtras(req orchestrator.StageRequest, exe string) map[string]string {
	m := map[string]string{
		"WAYSHARD_STAGE": fmt.Sprintf("%d", req.Stage.Ordinal),
	}
	// Test/fixture knobs for the deterministic fake harness only.
	for _, k := range []string{
		"WAYSHARD_FAKE_SCENARIO", "WAYSHARD_FAKE_WRITE_FILE", "WAYSHARD_FAKE_READ_FILE", "WAYSHARD_FAKE_STAGE",
		"WAYSHARD_FAKE_HANG_STAGE", "WAYSHARD_FAKE_SIGNAL_FILE", "WAYSHARD_FAKE_SUCCESS_MARKER",
		"WAYSHARD_FAKE_PARTIAL_TRACKED", "WAYSHARD_FAKE_PARTIAL_FILE", "WAYSHARD_FAKE_TOOL_WRITE_FILE", "WAYSHARD_FAKE_REVIEW_REJECT",
		"WAYSHARD_FAKE_TOOL_CMD",
		"WAYSHARD_FAKE_PERMISSION_STAGE", "WAYSHARD_FAKE_PERMISSION_CANARY",
	} {
		if v := os.Getenv(k); v != "" {
			m[k] = v
		}
	}
	m["WAYSHARD_FAKE_STAGE"] = fakeStage(req.Stage.Kind)
	if p := harnessEnvPATH(exe, os.Getenv("PATH")); p != os.Getenv("PATH") {
		m["PATH"] = p
	}
	return m
}

// permissionHook surfaces an ACP permission request as a durable, server-owned
// approval that any authorized client may resolve. The requesting operation
// pauses until the approval is resolved or the context ends.
func (e *ACPExec) permissionHook(ctx context.Context, req orchestrator.StageRequest) func(context.Context, acp.RequestPermissionParams) (acp.RequestPermissionResult, error) {
	return func(cbctx context.Context, p acp.RequestPermissionParams) (acp.RequestPermissionResult, error) {
		if e.Store == nil {
			return acp.CancelledPermission(), nil
		}
		resource := p.ToolCall.Title
		if resource == "" {
			resource = p.ToolCall.Kind
		}
		a := &domain.Approval{
			RunID:    req.Run.ID,
			StageID:  req.Stage.ID,
			Kind:     "acp_permission",
			Resource: resource,
			Reason:   "harness requested permission for " + resource,
			Status:   "pending",
		}
		if err := e.Store.InsertApproval(cbctx, a); err != nil {
			return acp.CancelledPermission(), err
		}
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		deadline := time.NewTimer(30 * time.Minute)
		defer deadline.Stop()
		for {
			select {
			case <-cbctx.Done():
				_ = e.Store.ResolveApproval(context.WithoutCancel(cbctx), a.ID, "cancelled", "cancellation")
				return acp.CancelledPermission(), cbctx.Err()
			case <-ctx.Done():
				_ = e.Store.ResolveApproval(context.WithoutCancel(ctx), a.ID, "cancelled", "cancellation")
				return acp.CancelledPermission(), ctx.Err()
			case <-deadline.C:
				_ = e.Store.ResolveApproval(context.WithoutCancel(cbctx), a.ID, "cancelled", "timeout")
				return acp.CancelledPermission(), fmt.Errorf("approval timed out")
			case <-ticker.C:
				got, err := e.Store.GetApproval(cbctx, a.ID)
				if err != nil {
					continue
				}
				switch got.Status {
				case "allowed":
					if id := allowOption(p.Options); id != "" {
						return acp.SelectedPermission(id), nil
					}
					return acp.SelectedPermission("allow-once"), nil
				case "denied":
					if id := rejectOption(p.Options); id != "" {
						return acp.SelectedPermission(id), nil
					}
					return acp.CancelledPermission(), nil
				}
			}
		}
	}
}

func allowOption(opts []acp.PermissionOption) string {
	for _, o := range opts {
		if strings.Contains(o.Kind, "allow") {
			return o.OptionID
		}
	}
	return ""
}

func rejectOption(opts []acp.PermissionOption) string {
	for _, o := range opts {
		if strings.Contains(o.Kind, "reject") {
			return o.OptionID
		}
	}
	return ""
}

// readHook serves ACP file reads scoped to the run workspace (or, for read-only
// stages, the read-only project view). Paths outside are refused.
func (e *ACPExec) readHook(req orchestrator.StageRequest) func(context.Context, acp.ReadTextFileParams) (acp.ReadTextFileResult, error) {
	root := ""
	if req.Workspace != nil {
		root = req.Workspace.RunPath
	}
	return func(_ context.Context, p acp.ReadTextFileParams) (acp.ReadTextFileResult, error) {
		full, err := withinRoot(root, p.Path)
		if err != nil {
			return acp.ReadTextFileResult{}, err
		}
		b, err := os.ReadFile(full)
		if err != nil {
			return acp.ReadTextFileResult{}, err
		}
		return acp.ReadTextFileResult{Content: string(b)}, nil
	}
}

// writeHook serves ACP file writes only for write stages and only inside the
// run workspace.
func (e *ACPExec) writeHook(req orchestrator.StageRequest) func(context.Context, acp.WriteTextFileParams) (acp.WriteTextFileResult, error) {
	root := ""
	if req.Workspace != nil {
		root = req.Workspace.RunPath
	}
	readOnly := req.Stage.Kind.ReadOnly()
	return func(_ context.Context, p acp.WriteTextFileParams) (acp.WriteTextFileResult, error) {
		if readOnly {
			return acp.WriteTextFileResult{}, fmt.Errorf("read-only stage: writes are not permitted")
		}
		full, err := withinRoot(root, p.Path)
		if err != nil {
			return acp.WriteTextFileResult{}, err
		}
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return acp.WriteTextFileResult{}, err
		}
		if err := os.WriteFile(full, []byte(p.Content), 0o644); err != nil {
			return acp.WriteTextFileResult{}, err
		}
		return acp.WriteTextFileResult{}, nil
	}
}

// withinRoot resolves p and requires it to remain inside root.
func withinRoot(root, p string) (string, error) {
	if root == "" {
		return "", fmt.Errorf("no workspace available")
	}
	if p == "" {
		return "", fmt.Errorf("empty path")
	}
	abs := p
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(root, p)
	}
	clean := filepath.Clean(abs)
	rootClean := filepath.Clean(root)
	if clean != rootClean && !strings.HasPrefix(clean, rootClean+string(os.PathSeparator)) {
		return "", fmt.Errorf("path escapes workspace")
	}
	// Reject symlink escapes by resolving the existing prefix.
	if resolved, err := filepath.EvalSymlinks(clean); err == nil {
		if resolved != rootClean && !strings.HasPrefix(resolved, rootClean+string(os.PathSeparator)) {
			return "", fmt.Errorf("path escapes workspace via symlink")
		}
	}
	return clean, nil
}

func fakeStage(k domain.StageKind) string {
	switch k {
	case domain.StageExecute:
		return "execute"
	case domain.StageRepair:
		return "repair"
	case domain.StageReview:
		return "review"
	case domain.StageExplore:
		return "explore"
	default:
		return "plan"
	}
}

// Compile-time assertion that storage is referenced (kept for API clarity).
var _ = storage.ErrNotFound

// selectSessionModel applies the route's model to a freshly created session
// using the catalog-declared model selection method. OpenCode exposes a "model"
// config option while Codex uses session/set_model.
func selectSessionModel(ctx context.Context, drv *acp.Driver, sessionID string, def Definition, modelID string) error {
	switch def.ModelSelection {
	case "config_option":
		return drv.SetConfigOption(ctx, sessionID, "model", modelID)
	case "set_model":
		return drv.SetModel(ctx, sessionID, modelID)
	default:
		return nil
	}
}

// artifactInstruction is the universal structured final-response contract for a
// harness that has no native structured submission channel. The server still
// validates the artifact schema.
func artifactInstruction(kind domain.StageKind) string {
	switch kind {
	case domain.StagePlan, domain.StageReplan:
		return "When you are finished, reply with ONLY a single JSON object (no prose, no code fences) of the form:\n" +
			`{"kind":"plan","objective":"<one line>","constraints":["..."],"acceptanceCriteria":["..."],"expectedPaths":["..."],"validationPlan":["..."],"risks":["..."],"assumptions":["..."],"artifactOnly":false}` +
			"\nDo not include any text before or after the JSON."
	case domain.StageExecute, domain.StageRepair:
		return "Perform the work in the current working directory. When you are finished, reply with ONLY a single JSON object (no prose, no code fences) of the form:\n" +
			`{"kind":"implementation","summary":"<what changed>","filesChanged":["relative/path"],"deviations":["..."],"expectedValidation":["..."]}` +
			"\nDo not include any text before or after the JSON."
	case domain.StageReview:
		return "Review the run delta against the acceptance criteria. Reply with ONLY a single JSON object (no prose, no code fences) of the form:\n" +
			`{"kind":"review","verdict":"pass|fail|insufficient","criteria":[{"id":"...","status":"pass|fail|not_verified","evidence":"..."}],"findings":[{"severity":"blocking|major|minor|info","path":"...","explanation":"...","requiredFix":"..."}]}` +
			"\nDo not include any text before or after the JSON."
	case domain.StageExplore:
		return "When you are finished, reply with ONLY a single JSON object (no prose, no code fences) of the form:\n" +
			`{"kind":"investigation","question":"...","findings":["..."],"openQuestions":["..."]}` +
			"\nDo not include any text before or after the JSON."
	default:
		return ""
	}
}

// classifyHarnessError maps an ACP error to a failure class. Authentication and
// provider-account/quota failures are policy outcomes so the orchestrator does
// not retry them as infrastructure; everything else is infrastructure.
func classifyHarnessError(err error) domain.FailureClass {
	var rpc *acp.Error
	if errors.As(err, &rpc) {
		if rpc.AuthRequired() {
			return domain.FailPolicy
		}
		blob := strings.ToLower(rpc.Message + " " + string(rpc.Data))
		for _, s := range []string{"usagelimitexceeded", "usage limit", "insufficient_quota", "quota", "billing"} {
			if strings.Contains(blob, s) {
				return domain.FailPolicy
			}
		}
	}
	return domain.FailInfrastructure
}
