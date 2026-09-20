package harness

import (
	"context"
	"time"

	"github.com/Wayshard/wayshard/internal/domain"
	"github.com/Wayshard/wayshard/internal/process"
	"github.com/Wayshard/wayshard/internal/storage"
)

// ProbeLease is a durable ownership record for one discovery probe process tree.
// The raw token is passed to the probe through its environment (and inherited by
// descendants); only its hash is persisted. On server death, startup
// reconciliation terminates any surviving descendants by token hash.
type ProbeLease interface {
	Token() string
	// SetPGID records the process group once the probe has started. It is a
	// secondary hint; reconciliation is keyed on the token hash.
	SetPGID(pgid int)
	// Done records that the probe tree has been proven terminated.
	Done()
}

// ProbeOwnerSink begins durable ownership for a probe before it launches. A nil
// sink means the caller is responsible for cleanup outside the durable store
// (used by isolated tests).
type ProbeOwnerSink interface {
	BeginProbe(ctx context.Context, kind string) (ProbeLease, error)
}

// StoreProbeOwnerSink persists probe ownership in SQLite.
type StoreProbeOwnerSink struct{ Store *storage.Store }

func (s StoreProbeOwnerSink) BeginProbe(ctx context.Context, kind string) (ProbeLease, error) {
	if s.Store == nil {
		return nil, nil
	}
	token, err := process.NewToken()
	if err != nil {
		return nil, err
	}
	o := &domain.ProbeOwner{
		Kind:      kind,
		TokenHash: process.HashToken(token),
		State:     domain.ProcessOwnerActive,
	}
	if err := s.Store.InsertProbeOwner(ctx, o); err != nil {
		return nil, err
	}
	return &storeProbeLease{store: s.Store, id: o.ID, token: token}, nil
}

type storeProbeLease struct {
	store *storage.Store
	id    string
	token string
	pgid  int
	done  bool
}

func (l *storeProbeLease) Token() string { return l.token }

func (l *storeProbeLease) SetPGID(pgid int) {
	if l == nil || pgid <= 0 {
		return
	}
	l.pgid = pgid
	if l.store == nil {
		return
	}
	_ = l.store.SetProbeOwnerPGID(context.WithoutCancel(context.Background()), l.id, pgid)
}

// Done terminates any descendant the probe left behind and only marks the
// record reconciled when none remains. A daemonized probe grandchild is killed
// by token here; if it cannot be proven gone the record stays active so startup
// reconciliation retries after a crash.
func (l *storeProbeLease) Done() {
	if l == nil || l.done {
		return
	}
	l.done = true
	if l.store == nil {
		return
	}
	bg := context.WithoutCancel(context.Background())
	_, remaining, supported, _ := process.ReconcileTokenHash(process.HashToken(l.token), l.pgid, 3*time.Second)
	if !supported {
		// Cannot verify: leave active for a platform that can.
		return
	}
	if remaining == 0 {
		_ = l.store.MarkProbeOwnerReconciled(bg, l.id)
	}
}
