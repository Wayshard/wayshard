package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"

	"encoding/json"

	"github.com/Wayshard/wayshard/internal/api"
	"github.com/Wayshard/wayshard/internal/auth"
	"github.com/Wayshard/wayshard/internal/domain"
	"github.com/Wayshard/wayshard/internal/events"
	"github.com/Wayshard/wayshard/internal/harness"
	"github.com/Wayshard/wayshard/internal/jev"
	"github.com/Wayshard/wayshard/internal/orchestrator"
	"github.com/Wayshard/wayshard/internal/paths"
	"github.com/Wayshard/wayshard/internal/pty"
	"github.com/Wayshard/wayshard/internal/recovery"
	"github.com/Wayshard/wayshard/internal/routing"
	"github.com/Wayshard/wayshard/internal/sandbox"
	"github.com/Wayshard/wayshard/internal/scheduler"
	"github.com/Wayshard/wayshard/internal/secrets"
	"github.com/Wayshard/wayshard/internal/storage"
	"github.com/Wayshard/wayshard/internal/validation"
)

type Config struct {
	DataDir   string
	Listen    string
	Advertise string
	JevURL    string
	JevKey    string
	JevModel  string
	Log       *slog.Logger
}

type App struct {
	Store  *storage.Store
	Vault  *secrets.Vault
	Auth   *auth.Service
	Hub    *events.Hub
	Engine *orchestrator.Engine
	Sched  *scheduler.Scheduler
	API    *api.Server
	Log    *slog.Logger
}

func Open(ctx context.Context, cfg Config) (*App, error) {
	if cfg.Log == nil {
		cfg.Log = slog.Default()
	}
	if cfg.DataDir == "" {
		cfg.DataDir = paths.DataDir()
	}
	if cfg.Listen == "" {
		cfg.Listen = fmt.Sprintf("127.0.0.1:%d", paths.DefaultPort)
	}
	if err := os.MkdirAll(cfg.DataDir, 0o700); err != nil {
		return nil, err
	}
	st, err := storage.Open(ctx, cfg.DataDir)
	if err != nil {
		return nil, err
	}
	if err := st.IntegrityCheck(ctx); err != nil {
		return nil, err
	}
	prov := secrets.DefaultProvider(cfg.DataDir)
	vault, err := secrets.Open(filepath.Join(cfg.DataDir, "vault"), prov)
	if err != nil && vault == nil {
		_ = st.Close()
		return nil, err
	}
	authSvc := &auth.Service{Store: st, Vault: vault, Listen: "http://" + cfg.Listen}
	if _, _, err := authSvc.EnsureIdentity(ctx); err != nil {
		cfg.Log.Warn("identity", "err", err)
	}
	hub := events.NewHub(st)
	var engine jev.DecisionEngine = jev.DeterministicEngine{}
	if cfg.JevKey != "" || os.Getenv("TYPESAFE_API_KEY") != "" {
		key := cfg.JevKey
		if key == "" {
			key = os.Getenv("TYPESAFE_API_KEY")
		}
		engine = jev.NewHTTP(cfg.JevURL, key, cfg.JevModel)
	}
	ptym := pty.New(st)
	orch := &orchestrator.Engine{
		Store:     st,
		Jev:       engine,
		Router:    &routing.Router{Engine: engine},
		Exec:      &harness.ACPExec{Sandbox: &sandbox.Manager{Backend: sandbox.DefaultBackend()}},
		Workspace: &orchestrator.WorkspaceAdapter{Store: st, DataDir: cfg.DataDir},
		Integrate: &orchestrator.IntegrateAdapter{Store: st},
		Validate:  &validation.Runner{Store: st},
		Log:       cfg.Log,
	}
	if os.Getenv("WAYSHARD_SYNTHETIC_ROUTE") == "1" {
		orch.Candidates = syntheticCandidates{}
		orch.Exec = nil // deterministic in-process artifacts
	} else {
		src := &storeCandidates{Store: st}
		_ = src.Refresh(ctx)
		orch.Candidates = src
	}
	sched := scheduler.New(st, orch, cfg.Log)
	apiSrv := &api.Server{
		Store:     st,
		Auth:      authSvc,
		Hub:       hub,
		Sched:     sched,
		Vault:     vault,
		PTY:       ptym,
		Log:       cfg.Log,
		Listen:    cfg.Listen,
		Advertise: cfg.Advertise,
		DataDir:   cfg.DataDir,
	}
	a := &App{Store: st, Vault: vault, Auth: authSvc, Hub: hub, Engine: orch, Sched: sched, API: apiSrv, Log: cfg.Log}
	if err := recovery.Reconcile(ctx, st, cfg.Log); err != nil {
		cfg.Log.Error("recovery", "err", err)
	}
	return a, nil
}

func (a *App) Run(ctx context.Context) error {
	a.Sched.Start(ctx)
	return a.API.ListenAndServe(ctx)
}

func (a *App) Close() error {
	a.Sched.Stop()
	return a.Store.Close()
}

func (a *App) Handler() http.Handler { return a.API.Handler() }

type storeCandidates struct{ Store *storage.Store }

func (s *storeCandidates) Refresh(ctx context.Context) error {
	found, err := harness.Discover(ctx, harness.DiscoverOptions{Probe: true})
	if err != nil {
		return err
	}
	for _, inst := range found {
		caps, _ := json.Marshal(inst.Capabilities)
		h := &domain.HarnessInstallation{
			ID: inst.ID, DefinitionID: inst.DefinitionID, DisplayName: inst.DisplayName,
			Executable: inst.Executable, Version: inst.Version, Adapter: inst.Adapter,
			Health: inst.Health, Compatibility: inst.Compatibility, Isolation: inst.Isolation,
			AuthStatus: inst.AuthStatus, CapabilitiesJSON: string(caps),
		}
		_ = s.Store.UpsertHarnessInstallation(ctx, h)
	}
	return nil
}

func (s *storeCandidates) Candidates(ctx context.Context) ([]routing.Candidate, error) {
	list, err := s.Store.ListHarnessInstallations(ctx)
	if err != nil {
		return nil, err
	}
	var out []routing.Candidate
	for _, h := range list {
		out = append(out, routing.Candidate{Harness: h, Isolation: h.Isolation})
	}
	return out, nil
}

type syntheticCandidates struct{}

func (syntheticCandidates) Candidates(context.Context) ([]routing.Candidate, error) {
	return []routing.Candidate{{
		Harness: domain.HarnessInstallation{
			ID:            "synthetic",
			DisplayName:   "synthetic",
			Health:        domain.HarnessReady,
			Compatibility: domain.CompatRoutable,
			Isolation:     domain.IsolationOuterOnly,
		},
		ModelID: "synthetic",
	}}, nil
}
