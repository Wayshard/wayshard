package domain

import "time"

// WorkspaceCheckpoint is a durable, restorable snapshot of the authoritative
// run workspace taken before a write attempt. It is not merely metadata: the
// TreePath holds a materializable snapshot tree.
type WorkspaceCheckpoint struct {
	ID          string    `json:"id"`
	WorkspaceID string    `json:"workspaceId"`
	RunID       string    `json:"runId"`
	StageID     string    `json:"stageId"`
	AttemptID   string    `json:"attemptId"`
	Name        string    `json:"name"`
	TreeHash    string    `json:"treeHash"`
	TreePath    string    `json:"treePath"`
	CreatedAt   time.Time `json:"createdAt"`
}
