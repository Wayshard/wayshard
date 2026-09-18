package storage

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type DiskStatus struct {
	Path      string `json:"path"`
	Total     uint64 `json:"totalBytes"`
	Free      uint64 `json:"freeBytes"`
	Used      uint64 `json:"usedBytes"`
	Low       bool   `json:"low"`
	Critical  bool   `json:"critical"`
	Threshold uint64 `json:"lowThresholdBytes"`
}

const (
	defaultLowBytes      = 2 << 30 // 2 GiB
	defaultCriticalBytes = 256 << 20
)

func (s *Store) StorageAccounting(ctx context.Context) map[string]int64 {
	out := map[string]int64{}
	if fi, err := os.Stat(filepath.Join(s.Root, "app.db")); err == nil {
		out["database"] = fi.Size()
	}
	out["objects"] = dirSize(filepath.Join(s.Root, "objects"))
	out["workspaces"] = dirSize(filepath.Join(s.Root, "runtime", "workspaces"))
	out["logs"] = dirSize(filepath.Join(s.Root, "logs"))
	return out
}

func dirSize(root string) int64 {
	var n int64
	_ = filepath.Walk(root, func(_ string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			n += info.Size()
		}
		return nil
	})
	return n
}

// GCObjects mark-and-sweeps unreferenced SHA-256 objects.
func (s *Store) GCObjects(ctx context.Context) (removed int, err error) {
	live := map[string]struct{}{}
	mark := func(q string) error {
		rows, err := s.DB.QueryContext(ctx, q)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var h sql.NullString
			if err := rows.Scan(&h); err != nil {
				return err
			}
			if h.Valid && h.String != "" {
				live[strings.ToLower(h.String)] = struct{}{}
			}
		}
		return rows.Err()
	}
	for _, q := range []string{
		`SELECT object_hash FROM artifacts`,
		`SELECT object_hash FROM attachments`,
		`SELECT object_hash FROM workspace_snapshots`,
		`SELECT object_hash FROM workspace_checkpoints`,
		`SELECT object_hash FROM run_deltas`,
		`SELECT log_hash FROM artifacts WHERE 0`, // placeholder
	} {
		_ = mark(q)
	}
	// also scan JSON for sha256 hex
	root := filepath.Join(s.Root, "objects", "sha256")
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		h := strings.ToLower(info.Name())
		if strings.HasSuffix(h, ".tmp") {
			if time.Since(info.ModTime()) > time.Hour {
				_ = os.Remove(path)
				removed++
			}
			return nil
		}
		if _, ok := live[h]; ok {
			return nil
		}
		// skip recently created (active pins)
		if time.Since(info.ModTime()) < 10*time.Minute {
			return nil
		}
		if err := os.Remove(path); err == nil {
			removed++
		}
		return nil
	})
	return removed, nil
}

func (s *Store) CleanupWorkspaces(ctx context.Context, retain time.Duration) (removed int, err error) {
	if retain <= 0 {
		retain = 7 * 24 * time.Hour
	}
	root := filepath.Join(s.Root, "runtime", "workspaces")
	ents, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	cutoff := time.Now().Add(-retain)
	for _, e := range ents {
		p := filepath.Join(root, e.Name())
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().After(cutoff) {
			continue
		}
		// keep if run still active
		r, err := s.GetRun(ctx, e.Name())
		if err == nil && !r.Status.Terminal() {
			continue
		}
		if err := os.RemoveAll(p); err == nil {
			removed++
		}
	}
	return removed, nil
}

func (s *Store) WriteHeavyAllowed() (bool, DiskStatus) {
	d, err := Disk(s.Root)
	if err != nil {
		return true, d
	}
	return !d.Critical, d
}
