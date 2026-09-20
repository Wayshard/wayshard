//go:build linux

package app

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Wayshard/wayshard/internal/domain"
	"github.com/Wayshard/wayshard/internal/harness"
	"github.com/Wayshard/wayshard/internal/orchestrator"
	"github.com/Wayshard/wayshard/internal/provider"
	"github.com/Wayshard/wayshard/internal/routing"
	"github.com/Wayshard/wayshard/internal/sandbox"
	"github.com/Wayshard/wayshard/internal/storage"
)

// TestRealHarnessProviderCall is a native/local verification that is skipped in
// CI. It drives an installed real ACP harness through Wayshard's secure
// provider transport for one tiny real model request.
//
//	WAYSHARD_REAL_HARNESS=1
//	WAYSHARD_REAL_HARNESS_EXE=codex-acp
//	WAYSHARD_REAL_PROVIDER_DESTS=chatgpt.com:443,...
//	WAYSHARD_REAL_PROMPT="Reply with exactly WAYSHARD_REAL_ACP_OK."
func TestRealHarnessProviderCall(t *testing.T) {
	if os.Getenv("WAYSHARD_REAL_HARNESS") != "1" {
		t.Skip("set WAYSHARD_REAL_HARNESS=1 to run the real-harness provider test")
	}
	exe := os.Getenv("WAYSHARD_REAL_HARNESS_EXE")
	if exe == "" {
		t.Skip("set WAYSHARD_REAL_HARNESS_EXE")
	}
	if !filepath.IsAbs(exe) {
		p, err := exec.LookPath(exe)
		if err != nil {
			t.Skipf("harness %q not found on PATH: %v", exe, err)
		}
		exe = p
	}
	dests := parseDestsEnv(os.Getenv("WAYSHARD_REAL_PROVIDER_DESTS"))
	if len(dests) == 0 {
		t.Skip("set WAYSHARD_REAL_PROVIDER_DESTS=host:port,...")
	}
	prompt := os.Getenv("WAYSHARD_REAL_PROMPT")
	if prompt == "" {
		prompt = "Reply with exactly WAYSHARD_REAL_ACP_OK."
	}
	adapter := os.Getenv("WAYSHARD_REAL_HARNESS_ADAPTER")
	model := os.Getenv("WAYSHARD_REAL_HARNESS_MODEL")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	dir := t.TempDir()
	st, err := storage.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ws := filepath.Join(dir, "ws")
	_ = os.MkdirAll(ws, 0o755)
	_ = os.WriteFile(filepath.Join(ws, "README.md"), []byte("# real harness test\n"), 0o644)

	var logbuf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&logbuf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	ex := &harness.ACPExec{
		Store:       st,
		Sandbox:     &sandbox.Manager{Backend: sandbox.DefaultBackend()},
		ProviderLog: log,
	}
	req := orchestrator.StageRequest{
		Run:     domain.Run{ID: "real-run"},
		Task:    domain.Task{ID: "real-task", Objective: prompt},
		Stage:   domain.Stage{ID: "real-stage", Kind: domain.StagePlan, Ordinal: 1},
		Attempt: domain.StageAttempt{ID: "real-attempt"},
		Route: routing.Candidate{
			Harness: domain.HarnessInstallation{
				ID: "real", DefinitionID: adapter, DisplayName: "real", Executable: exe, Adapter: adapter,
				Health: domain.HarnessReady, Compatibility: domain.CompatRoutable,
			},
			ModelID:           model,
			Network:           domain.NetworkProvider,
			ProviderTransport: domain.TransportHTTPProxy,
		},
		Workspace:            &domain.WorkspaceRecord{RunPath: ws},
		ProviderDestinations: dests,
	}
	res, err := ex.Execute(ctx, req)
	t.Logf("broker log:\n%s", logbuf.String())
	t.Logf("result class=%s err=%v", res.Class, err)
	t.Logf("artifact (first 2000 bytes):\n%s", truncate(res.ArtifactJSON, 2000))
	if err != nil {
		t.Fatalf("real harness provider call failed: %v", err)
	}
	if strings.Contains(prompt, "WAYSHARD_REAL_ACP_OK") && !strings.Contains(res.ArtifactJSON, "WAYSHARD_REAL_ACP_OK") {
		t.Fatalf("expected WAYSHARD_REAL_ACP_OK in response, got: %s", truncate(res.ArtifactJSON, 500))
	}
}

func parseDestsEnv(s string) []domain.ProviderDestination {
	var out []domain.ProviderDestination
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		host, portStr := part, "443"
		if i := strings.LastIndex(part, ":"); i >= 0 {
			host, portStr = part[:i], part[i+1:]
		}
		port, err := strconv.Atoi(portStr)
		if err != nil || port <= 0 || port > 65535 {
			continue
		}
		out = append(out, domain.ProviderDestination{Host: host, Port: port})
	}
	return out
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

var _ = provider.Detect
