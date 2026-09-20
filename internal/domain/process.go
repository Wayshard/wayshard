package domain

import "time"

// Process ownership states for server-launched process trees.
const (
	ProcessOwnerActive     = "active"
	ProcessOwnerReconciled = "reconciled"
)

// ProcessOwner records that a server-launched process tree belongs to one stage
// attempt. After an unexpected server death the tree may outlive the server
// (direct children are killed by Pdeathsig, but backgrounded descendants are
// not). Startup reconciliation uses the recorded token to identify descendants
// robustly against PID reuse and terminates them before any workspace restore.
//
// Only the token hash is persisted; the raw token is passed to the supervised
// process through its environment and never stored.
type ProcessOwner struct {
	ID           string     `json:"id"`
	RunID        string     `json:"runId"`
	StageID      string     `json:"stageId"`
	AttemptID    string     `json:"attemptId"`
	TokenHash    string     `json:"tokenHash"`
	PGID         int        `json:"pgid"`
	State        string     `json:"state"`
	CreatedAt    time.Time  `json:"createdAt"`
	ReconciledAt *time.Time `json:"reconciledAt,omitempty"`
}

// ProbeOwner records that a server-launched discovery probe process tree (for
// example a version, ACP-initialize or login-shell PATH probe) belongs to the
// server. Probes are not tied to a run/stage/attempt, so they have their own
// ownership table. Like ProcessOwner, only the token hash is persisted and
// startup reconciliation terminates surviving descendants by token.
type ProbeOwner struct {
	ID           string     `json:"id"`
	Kind         string     `json:"kind"`
	TokenHash    string     `json:"tokenHash"`
	PGID         int        `json:"pgid"`
	State        string     `json:"state"`
	CreatedAt    time.Time  `json:"createdAt"`
	ReconciledAt *time.Time `json:"reconciledAt,omitempty"`
}

// Checkpoint material states. A checkpoint row may outlive its material tree
// once retention reclaims it; recovery must never treat a reclaimed checkpoint
// as restorable.
const (
	CheckpointPresent   = "present"
	CheckpointReclaimed = "reclaimed"
)
