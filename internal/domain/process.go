package domain

// Checkpoint material states. A checkpoint row may outlive its material tree
// once retention reclaims it; recovery must never treat a reclaimed checkpoint
// as restorable.
const (
	CheckpointPresent   = "present"
	CheckpointReclaimed = "reclaimed"
)
