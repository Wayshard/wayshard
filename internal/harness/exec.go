package harness

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/Wayshard/wayshard/internal/acp"
	"github.com/Wayshard/wayshard/internal/artifacts"
	"github.com/Wayshard/wayshard/internal/domain"
	"github.com/Wayshard/wayshard/internal/orchestrator"
	"github.com/Wayshard/wayshard/internal/sandbox"
)

// ACPExec runs a stage against an ACP harness. Validate is not executed here.
type ACPExec struct {
	Sandbox *sandbox.Manager
}

func (e *ACPExec) Execute(ctx context.Context, req orchestrator.StageRequest) (orchestrator.StageResult, error) {
	if req.Stage.Kind == domain.StageValidate || req.Stage.Kind == domain.StageIntegrate {
		return orchestrator.StageResult{Err: fmt.Errorf("stage %s is server-owned", req.Stage.Kind)}, nil
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
		Env: []string{
			"WAYSHARD_FAKE_STAGE=" + fakeStage(req.Stage.Kind),
			"WAYSHARD_FAKE_SCENARIO=" + os.Getenv("WAYSHARD_FAKE_SCENARIO"),
		},
	}
	if inst.Executable == "" {
		inst.Executable = req.Route.Harness.DisplayName
	}
	ad := AdapterFor(req.Route.Harness.Adapter, inst.Executable)
	spec := ad.LaunchSpec(inst)
	if spec.Dir == "" {
		spec.Dir = cwd
	}
	spec.Env = append(os.Environ(), inst.Env...)
	if e.Sandbox != nil {
		pol := sandbox.HarnessPolicy(cwd, os.TempDir())
		if cwd != "" {
			pol.ReadWriteRoots = []string{cwd}
		}
		b := e.Sandbox.Backend
		if b == nil {
			b = sandbox.DefaultBackend()
		}
		con := sandbox.AsConstrainer(b)
		if _, err := con.Compile(pol); err != nil {
			return orchestrator.StageResult{Class: domain.FailInfrastructure, Err: err}, err
		}
		spec.SetupCmd = func(cmd *exec.Cmd) error { return con.Constrain(cmd, pol) }
		spec.AfterStart = func(cmd *exec.Cmd) (func(), error) { return con.Attach(cmd, pol) }
	}
	drv, err := acp.Launch(ctx, spec, acp.DefaultClientConfig(), acp.Hooks{}, acp.Limits{})
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
		msg := err.Error()
		class := domain.FailInfrastructure
		if strings.Contains(msg, "model") {
			class = domain.FailInfrastructure
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

func fakeStage(k domain.StageKind) string {
	switch k {
	case domain.StageExecute, domain.StageRepair:
		return "execute"
	case domain.StageReview:
		return "review"
	case domain.StageExplore:
		return "explore"
	default:
		return "plan"
	}
}
