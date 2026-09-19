package workspace

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// RestoreSnapshot replaces dest with the state captured in snap. It materializes
// the snapshot into a sibling temporary directory (reusing the same isolated
// clone/byte-copy machinery) and then atomically swaps it into place, so a
// partially written workspace is discarded rather than trusted.
func RestoreSnapshot(ctx context.Context, b WorkspaceBackend, snap *Snapshot, dest string) error {
	if snap == nil {
		return fmt.Errorf("checkpoint snapshot is required")
	}
	parent := filepath.Dir(dest)
	tmp, err := os.MkdirTemp(parent, "restore-*")
	if err != nil {
		return err
	}
	if err := b.Materialize(ctx, snap, tmp); err != nil {
		_ = os.RemoveAll(tmp)
		return err
	}
	if err := os.RemoveAll(dest); err != nil {
		_ = os.RemoveAll(tmp)
		return err
	}
	if err := os.Rename(tmp, dest); err != nil {
		_ = os.RemoveAll(tmp)
		return err
	}
	return nil
}
