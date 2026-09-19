package scheduler

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/Wayshard/wayshard/internal/domain"
	"github.com/Wayshard/wayshard/internal/orchestrator"
	"github.com/Wayshard/wayshard/internal/storage"
)

type Scheduler struct {
	Store      *storage.Store
	Engine     *orchestrator.Engine
	Log        *slog.Logger
	inflight   map[string]struct{}
	sourceLock map[string]*sync.Mutex
	cancels    map[string]context.CancelFunc
	mu         sync.Mutex
	wg         sync.WaitGroup
	stop       chan struct{}
}

func New(store *storage.Store, eng *orchestrator.Engine, log *slog.Logger) *Scheduler {
	if log == nil {
		log = slog.Default()
	}
	return &Scheduler{
		Store:      store,
		Engine:     eng,
		Log:        log,
		inflight:   map[string]struct{}{},
		sourceLock: map[string]*sync.Mutex{},
		cancels:    map[string]context.CancelFunc{},
		stop:       make(chan struct{}),
	}
}

func (s *Scheduler) Start(ctx context.Context) {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		t := time.NewTicker(200 * time.Millisecond)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-s.stop:
				return
			case <-t.C:
				s.tick(ctx)
			}
		}
	}()
}

func (s *Scheduler) Stop() {
	close(s.stop)
	// Cancel all in-flight execution so process trees are cleaned up.
	s.mu.Lock()
	for _, cancel := range s.cancels {
		cancel()
	}
	s.mu.Unlock()
	s.wg.Wait()
}

// Cancel interrupts the active execution of a run, if any. It reports whether
// an active execution was found.
func (s *Scheduler) Cancel(runID string) bool {
	s.mu.Lock()
	cancel := s.cancels[runID]
	s.mu.Unlock()
	if cancel == nil {
		return false
	}
	cancel()
	return true
}

// Enqueue starts a run, locking on its concrete source identity so concurrent
// integrations into the same repository serialize.
func (s *Scheduler) Enqueue(runID string) {
	src := s.sourceIDFor(context.Background(), runID)
	go s.runOne(context.Background(), runID, src)
}

func (s *Scheduler) tick(ctx context.Context) {
	allowed, _ := s.Store.WriteHeavyAllowed()
	runs, err := s.Store.ListActiveRuns(ctx)
	if err != nil {
		s.Log.Warn("list active runs", "err", err)
		return
	}
	for _, r := range runs {
		if r.Status == domain.RunBlocked || r.Status == domain.RunIntegrationBlocked {
			continue
		}
		if !allowed {
			// Do not start write-heavy work under critical disk pressure.
			// Move the run to a recoverable blocked state instead of proceeding.
			_ = s.Store.UpdateRunStatus(ctx, r.ID, domain.RunBlocked, domain.BlockedStorage, "low disk: write-heavy work suspended")
			continue
		}
		s.mu.Lock()
		_, busy := s.inflight[r.ID]
		s.mu.Unlock()
		if busy {
			continue
		}
		src := s.sourceIDFor(ctx, r.ID)
		id := r.ID
		s.runOne(ctx, id, src)
	}
}

func (s *Scheduler) sourceIDFor(ctx context.Context, runID string) string {
	run, err := s.Store.GetRun(ctx, runID)
	if err != nil {
		return ""
	}
	proj, err := s.Store.GetProject(ctx, run.ProjectID)
	if err != nil {
		return ""
	}
	if proj.RepoIdentity != "" {
		return proj.RepoIdentity
	}
	return proj.Path
}

func (s *Scheduler) runOne(ctx context.Context, runID, sourceID string) {
	s.mu.Lock()
	if _, ok := s.inflight[runID]; ok {
		s.mu.Unlock()
		return
	}
	s.inflight[runID] = struct{}{}
	if sourceID != "" {
		if s.sourceLock[sourceID] == nil {
			s.sourceLock[sourceID] = &sync.Mutex{}
		}
	}
	s.mu.Unlock()

	runCtx, cancel := context.WithCancel(ctx)
	s.mu.Lock()
	s.cancels[runID] = cancel
	s.mu.Unlock()

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		defer cancel()
		defer func() {
			s.mu.Lock()
			delete(s.inflight, runID)
			delete(s.cancels, runID)
			s.mu.Unlock()
		}()
		if sourceID != "" {
			s.mu.Lock()
			lk := s.sourceLock[sourceID]
			s.mu.Unlock()
			// Serialize publication against the same concrete source identity.
			run, err := s.Store.GetRun(runCtx, runID)
			if err == nil && (run.Status == domain.RunReadyToIntegrate || run.Status == domain.RunIntegrating) {
				lk.Lock()
				defer lk.Unlock()
			}
		}
		if err := s.Engine.ProcessRun(runCtx, runID); err != nil {
			s.Log.Warn("process run", "run", runID, "err", err)
		}
	}()
}