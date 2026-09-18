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
	s.wg.Wait()
}

func (s *Scheduler) Enqueue(runID string) {
	go s.runOne(context.Background(), runID, "")
}

func (s *Scheduler) tick(ctx context.Context) {
	if ok, disk := s.Store.WriteHeavyAllowed(); !ok {
		s.Log.Warn("critical disk; refusing new write-heavy work", "free", disk.Free)
	}
	runs, err := s.Store.ListActiveRuns(ctx)
	if err != nil {
		s.Log.Warn("list active runs", "err", err)
		return
	}
	for _, r := range runs {
		if r.Status == domain.RunBlocked || r.Status == domain.RunIntegrationBlocked {
			continue
		}
		s.mu.Lock()
		_, busy := s.inflight[r.ID]
		s.mu.Unlock()
		if busy {
			continue
		}
		proj, err := s.Store.GetProject(ctx, r.ProjectID)
		src := ""
		if err == nil {
			src = proj.RepoIdentity
			if src == "" {
				src = proj.Path
			}
		}
		id := r.ID
		s.runOne(ctx, id, src)
	}
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
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		defer func() {
			s.mu.Lock()
			delete(s.inflight, runID)
			s.mu.Unlock()
		}()
		if sourceID != "" {
			s.mu.Lock()
			lk := s.sourceLock[sourceID]
			s.mu.Unlock()
			// serialize integration against the same source identity
			run, err := s.Store.GetRun(ctx, runID)
			if err == nil && (run.Status == domain.RunReadyToIntegrate || run.Status == domain.RunIntegrating) {
				lk.Lock()
				defer lk.Unlock()
			}
		}
		if err := s.Engine.ProcessRun(ctx, runID); err != nil {
			s.Log.Warn("process run", "run", runID, "err", err)
		}
	}()
}
