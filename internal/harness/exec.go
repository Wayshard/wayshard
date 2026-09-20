package harness

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Wayshard/wayshard/internal/acp"
	"github.com/Wayshard/wayshard/internal/artifacts"
	"github.com/Wayshard/wayshard/internal/domain"
	"github.com/Wayshard/wayshard/internal/orchestrator"
	"github.com/Wayshard/wayshard/internal/sandbox"
	"github.com/Wayshard/wayshard/internal/storage"
)

// ACPExec runs a stage against an ACP harness. Validate is not executed here.
type ACPExec struct {
	Store   *storage.Store
	Sandbox *sandbox.Manager
}

func (e *ACPExec) Execute(ctx context.Context, req orchestrator.StageRequest) (orchestrator.StageResult, error) {
	if req.Stage.Kind == domain.StageValidate || req.Stage.Kind == domain.StageIntegrate {
		return orchestrator.StageResult{Err: fmt.Errorf("stage %s is server-owned", req.Stage.Kind)}, nil
	}
	// Defense in depth: a route that requires provider network must never be
	// launched, because secure provider-only isolation is not implemented.
	if req.Route.Network == domain.NetworkProvider {
		err := fmt.Errorf("secure provider network isolation unavailable")
		return orchestrator.StageResult{Class: domain.FailPolicy, Err: err}, err
	}
	cwd := ""
	if req.Workspace != nil {
		cwd = req.Workspace.RunPath
	}
	inst := Installation{
		ID:         req.Route.Harness.ID,
		Executable: req.Route.Harness.Executable,
		Adapter:    req.Route.Harness.Adapter,
		Dir:        cwd,
	}
	if inst.Executable == "" {
		inst.Executable = req.Route.Harness.DisplayName
	}
	ad := AdapterFor(req.Route.Harness.Adapter, inst.Executable)
	spec := ad.LaunchSpec(inst)
	if spec.Dir == "" {
		spec.Dir = cwd
	}

	sandboxRoot := e.sandboxRoot(req)
	home := filepath.Join(sandboxRoot, "home")
	tmp := filepath.Join(sandboxRoot, "tmp")
	_ = os.MkdirAll(home, 0o700)
	_ = os.MkdirAll(tmp, 0o700)
	realHome, _ := os.UserHomeDir()

	// The harness keeps its own HOME for provider auth, but Landlock only
	// permits its owned config dirs plus the workspace and system roots.
	spec.Env = sandbox.HarnessEnv(realHome, tmp, harnessExtras(req))
	if e.Sandbox != nil {
		b := e.Sandbox.Backend
		if b == nil {
			b = sandbox.DefaultBackend()
		}
		con := sandbox.AsConstrainer(b)
		pol := e.policyFor(req, cwd, tmp, realHome)
		if _, err := con.Compile(pol); err != nil {
			return orchestrator.StageResult{Class: domain.FailInfrastructure, Err: err}, err
		}
		// The harness executable itself must remain executable/readable.
		if inst.Executable != "" {
			pol.ReadOnlyRoots = append(pol.ReadOnlyRoots, filepath.Dir(inst.Executable))
		}
		spec.SetupCmd = func(cmd *exec.Cmd) error { return con.Constrain(cmd, pol) }
		spec.AfterStart = func(cmd *exec.Cmd) (func(), error) { return con.Attach(cmd, pol) }
	}

	hooks := acp.Hooks{
		RequestPermission: e.permissionHook(ctx, req),
		ReadTextFile:      e.readHook(req),
		WriteTextFile:     e.writeHook(req),
	}
	var sbe sandbox.Backend
	if e.Sandbox != nil {
		sbe = e.Sandbox.Backend
	}
	tm := newToolManager(req, cwd, home, sbe)
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
		return orchestrator.StageResult{Class: domain.FailInfrastructure, Err: err}, err
	}
	prompt := req.Task.Objective
	if req.Bundle != "" {
		prompt = req.Bundle + "\n\n" + prompt
	}
	_, err = drv.Prompt(ctx, acp.PromptRequest{
		SessionID: sess.SessionID,
		Prompt:    []acp.ContentBlock{acp.TextBlock(prompt)},
	})
	if err != nil {
		class := domain.FailInfrastructure
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

func (e *ACPExec) sandboxRoot(req orchestrator.StageRequest) string {
	base := os.TempDir()
	if e.Store != nil && e.Store.Root != "" {
		base = e.Store.Root
	}
	dir := filepath.Join(base, "runtime", "sandbox", req.Run.ID)
	_ = os.MkdirAll(dir, 0o700)
	return dir
}

// policyFor builds the harness policy. Read-only stages get a read-only view of
// the run workspace; write stages may write it. The harness's own config roots
// are granted so provider auth keeps working, but nothing else under HOME is.
func (e *ACPExec) policyFor(req orchestrator.StageRequest, cwd, tmp, realHome string) sandbox.Policy {
	var pol sandbox.Policy
	if cwd == "" {
		pol = sandbox.ReadOnlyViewPolicy("", tmp)
	} else if req.Stage.Kind.ReadOnly() {
		pol = sandbox.ReadOnlyViewPolicy(cwd, tmp)
	} else {
		pol = sandbox.HarnessPolicy(cwd, tmp)
	}
	for _, r := range harnessConfigRoots(req.Route.Harness.DefinitionID, req.Route.Harness.Adapter, realHome) {
		pol.ReadWriteRoots = append(pol.ReadWriteRoots, r)
	}
	if extra := os.Getenv("WAYSHARD_HARNESS_EXTRA_ROOTS"); extra != "" {
		pol.ReadWriteRoots = append(pol.ReadWriteRoots, filepath.SplitList(extra)...)
	}
	return pol
}

// harnessConfigRoots returns the harness-owned configuration directories that
// may be exposed to the harness process. Never includes the Wayshard data dir.
func harnessConfigRoots(definitionID, adapter, home string) []string {
	if home == "" {
		return nil
	}
	var rel []string
	switch {
	case adapter == "opencode" || definitionID == "opencode":
		rel = []string{".config/opencode", ".local/share/opencode", ".local/state/opencode", ".cache/opencode", ".opencode"}
	case adapter == "codex" || definitionID == "codex":
		rel = []string{".codex", ".config/codex", ".local/share/codex", ".cache/codex"}
	}
	var out []string
	for _, r := range rel {
		p := filepath.Join(home, r)
		if _, err := os.Stat(p); err == nil {
			out = append(out, p)
		}
	}
	return out
}

func harnessExtras(req orchestrator.StageRequest) map[string]string {
	m := map[string]string{
		"WAYSHARD_STAGE": fmt.Sprintf("%d", req.Stage.Ordinal),
	}
	// Test/fixture knobs for the deterministic fake harness only.
	for _, k := range []string{
		"WAYSHARD_FAKE_SCENARIO", "WAYSHARD_FAKE_WRITE_FILE", "WAYSHARD_FAKE_READ_FILE", "WAYSHARD_FAKE_STAGE",
		"WAYSHARD_FAKE_HANG_STAGE", "WAYSHARD_FAKE_SIGNAL_FILE", "WAYSHARD_FAKE_SUCCESS_MARKER",
		"WAYSHARD_FAKE_PARTIAL_TRACKED", "WAYSHARD_FAKE_PARTIAL_FILE", "WAYSHARD_FAKE_TOOL_WRITE_FILE", "WAYSHARD_FAKE_REVIEW_REJECT",
	} {
		if v := os.Getenv(k); v != "" {
			m[k] = v
		}
	}
	m["WAYSHARD_FAKE_STAGE"] = fakeStage(req.Stage.Kind)
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
