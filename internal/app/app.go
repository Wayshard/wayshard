package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"encoding/json"

	"github.com/Wayshard/wayshard/internal/api"
	"github.com/Wayshard/wayshard/internal/auth"
	"github.com/Wayshard/wayshard/internal/credentials"
	"github.com/Wayshard/wayshard/internal/domain"
	"github.com/Wayshard/wayshard/internal/events"
	"github.com/Wayshard/wayshard/internal/harness"
	"github.com/Wayshard/wayshard/internal/jev"
	"github.com/Wayshard/wayshard/internal/notifications"
	"github.com/Wayshard/wayshard/internal/orchestrator"
	"github.com/Wayshard/wayshard/internal/paths"
	"github.com/Wayshard/wayshard/internal/pty"
	"github.com/Wayshard/wayshard/internal/recovery"
	"github.com/Wayshard/wayshard/internal/routing"
	"github.com/Wayshard/wayshard/internal/scheduler"
	"github.com/Wayshard/wayshard/internal/storage"
	"github.com/Wayshard/wayshard/internal/validation"
)

// startupObserver is nil in production. Tests set it to observe the durable
// startup step ordering (recovery before discovery) without timing sleeps.
var startupObserver func(step string)

func observeStartup(step string) {
	if startupObserver != nil {
		startupObserver(step)
	}
}

type Config struct {
	DataDir   string
	Listen    string
	Advertise string
	JevURL    string
	JevKey    string
	JevModel  string
	Log       *slog.Logger
	// HarnessCatalogPath overrides the user harness catalog path. Empty uses the
	// platform config location.
	HarnessCatalogPath string
}

type App struct {
	Store       *storage.Store
	Credentials *credentials.Store
	Auth        *auth.Service
	Hub         *events.Hub
	Engine      *orchestrator.Engine
	Sched       *scheduler.Scheduler
	API         *api.Server
	Log         *slog.Logger
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
	creds, err := credentials.Open(filepath.Join(cfg.DataDir, "credentials"))
	if err != nil {
		_ = st.Close()
		return nil, err
	}
	authSvc := &auth.Service{Store: st, Credentials: creds, Listen: "http://" + cfg.Listen}
	if _, _, err := authSvc.EnsureIdentity(ctx); err != nil {
		cfg.Log.Warn("identity", "err", err)
	}
	hub := events.NewHub(st)
	st.EventHook = func(ev domain.Event) {
		// Live projection of a committed durable event.
		hub.Broadcast(ev)
		notifications.Derive(context.Background(), st, cfg.Log, ev)
	}
	var engine jev.DecisionEngine = jev.DeterministicEngine{}
	if cfg.JevKey != "" || os.Getenv("TYPESAFE_API_KEY") != "" {
		key := cfg.JevKey
		if key == "" {
			key = os.Getenv("TYPESAFE_API_KEY")
		}
		engine = jev.NewHTTP(cfg.JevURL, key, cfg.JevModel)
	}
	ptym := pty.New(st)
	cat := loadHarnessCatalog(cfg.Log, cfg.HarnessCatalogPath)
	exec := &harness.ACPExec{Store: st, Log: cfg.Log, Catalog: cat}
	orch := &orchestrator.Engine{
		Store:     st,
		Jev:       engine,
		Router:    &routing.Router{Engine: engine},
		Exec:      exec,
		Workspace: &orchestrator.WorkspaceAdapter{Store: st, DataDir: cfg.DataDir},
		Integrate: &orchestrator.IntegrateAdapter{Store: st},
		Validate:  &validation.Runner{Store: st},
		Budget:    orchestrator.DefaultBudgets(),
		Log:       cfg.Log,
	}
	if os.Getenv("WAYSHARD_SYNTHETIC_ROUTE") == "1" {
		orch.Candidates = syntheticCandidates{}
		orch.Exec = nil // deterministic in-process artifacts
	} else {
		orch.Candidates = &storeCandidates{Store: st, Catalog: cat}
	}
	sched := scheduler.New(st, orch, cfg.Log)
	apiSrv := &api.Server{
		Store:       st,
		Auth:        authSvc,
		Hub:         hub,
		Sched:       sched,
		Credentials: creds,
		PTY:         ptym,
		Log:         cfg.Log,
		Listen:      cfg.Listen,
		Advertise:   cfg.Advertise,
		DataDir:     cfg.DataDir,
		Catalog:     cat,
	}
	a := &App{Store: st, Credentials: creds, Auth: authSvc, Hub: hub, Engine: orch, Sched: sched, API: apiSrv, Log: cfg.Log}
	// Startup recovery is a hard gate: the scheduler must never dispatch work
	// before interrupted runs/workspaces are reconciled. A failure here leaves
	// the server unopened rather than running against unrecovered state.
	if err := recovery.Reconcile(ctx, st, cfg.Log); err != nil {
		cfg.Log.Error("recovery", "err", err)
		_ = st.Close()
		return nil, fmt.Errorf("startup recovery: %w", err)
	}
	observeStartup("recovery")
	// Harness discovery runs after recovery so interrupted state is consistent
	// before probing executes installed executables.
	if src, ok := orch.Candidates.(*storeCandidates); ok {
		observeStartup("discovery.start")
		if err := src.Refresh(ctx); err != nil {
			cfg.Log.Warn("harness discovery", "err", err)
		}
		observeStartup("discovery.done")
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

type storeCandidates struct {
	Store   *storage.Store
	Catalog *harness.Catalog
}

func (s *storeCandidates) Refresh(ctx context.Context) error {
	var defs []harness.Definition
	if s.Catalog != nil {
		defs = s.Catalog.EnabledForPlatform(runtime.GOOS)
	}
	opts := harness.DefaultDiscoverOptions()
	opts.Probe = true
	opts.Definitions = defs
	found, err := harness.Discover(ctx, opts)
	if err != nil {
		return err
	}
	next := make([]domain.HarnessInstallation, 0, len(found))
	for _, inst := range found {
		caps, _ := json.Marshal(inst.Capabilities)
		next = append(next, domain.HarnessInstallation{
			ID: inst.ID, DefinitionID: inst.DefinitionID, DisplayName: inst.DisplayName,
			Executable: inst.Executable, Version: inst.Version, Adapter: "generic",
			Health: inst.Health, Compatibility: inst.Compatibility,
			AuthStatus: inst.AuthStatus, CapabilitiesJSON: string(caps),
			Notes:                 strings.Join(inst.Notes, "; "),
			DefinitionSource:      string(inst.DefinitionSource),
			DefinitionFingerprint: inst.DefinitionFingerprint,
			BridgeExecutable:      inst.BridgeExecutable,
			BridgePresent:         inst.BridgePresent,
			ACPStatus:             inst.ACPStatus,
			BlockingReason:        inst.BlockingReason,
			ModelSelection:        inst.ModelSelection,
		})
	}
	// Replace the persisted set atomically so routing can never observe an
	// installation from a previous catalog or discovery run.
	return s.Store.ReplaceHarnessInstallations(ctx, next)
}

func (s *storeCandidates) Candidates(ctx context.Context) ([]routing.Candidate, error) {
	list, err := s.Store.ListHarnessInstallations(ctx)
	if err != nil {
		return nil, err
	}
	var out []routing.Candidate
	for _, h := range list {
		// Current candidate state comes only from the latest effective catalog.
		// A row whose definition is missing, disabled, or whose execution
		// fingerprint no longer matches is stale and is never routable.
		if s.Catalog != nil {
			def, ok := s.Catalog.ByID(h.DefinitionID)
			if !ok || !def.Enabled || def.ExecutionFingerprint() == "" || def.ExecutionFingerprint() != h.DefinitionFingerprint {
				continue
			}
		}
		out = append(out, routing.Candidate{Harness: h})
	}
	return out, nil
}

// loadHarnessCatalog loads the effective catalog, logging diagnostics. A
// malformed user catalog is reported and the shipped defaults are used so a
// bad user file cannot make the server unusable.
func loadHarnessCatalog(log *slog.Logger, path string) *harness.Catalog {
	if path == "" {
		// Environment equivalent of --harness-catalog; also lets tests pin a
		// deterministic catalog regardless of ambient installed harnesses.
		path = os.Getenv("WAYSHARD_HARNESS_CATALOG")
	}
	cat, err := harness.LoadCatalog(path)
	if err != nil {
		if log != nil {
			log.Error("harness catalog", "err", err)
		}
		cat = harness.ShippedCatalog()
		// Surface the failure through the diagnostics API rather than log-only
		// fallback, so a malformed/unsupported user catalog is visible.
		cat.Diagnostics = append(cat.Diagnostics, harness.CatalogDiagnostic{
			Source:  "user",
			Message: "user harness catalog rejected: " + err.Error(),
		})
	}
	if log != nil {
		for _, d := range cat.Diagnostics {
			log.Warn("harness catalog diagnostic", "detail", d.String())
		}
	}
	return cat
}

type syntheticCandidates struct{}

func (syntheticCandidates) Candidates(context.Context) ([]routing.Candidate, error) {
	return []routing.Candidate{{
		Harness: domain.HarnessInstallation{
			ID:            "synthetic",
			DisplayName:   "synthetic",
			Health:        domain.HarnessReady,
			Compatibility: domain.CompatRoutable,
		},
		ModelID: "synthetic",
	}}, nil
}
