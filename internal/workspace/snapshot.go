package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/Wayshard/wayshard/internal/gitutil"
	"github.com/Wayshard/wayshard/internal/id"
)

// CaptureOptions control where the immutable snapshot tree is stored.
type CaptureOptions struct {
	SnapshotDir  string
	KnowledgeRev string
	ID           string
}

// DirtyEntry records a path that differed from HEAD at intake. These files
// are user baseline, never agent output.
type DirtyEntry struct {
	Path      string `json:"path"`
	OrigPath  string `json:"origPath,omitempty"`
	Staged    bool   `json:"staged"`
	Unstaged  bool   `json:"unstaged"`
	Untracked bool   `json:"untracked"`
	Index     string `json:"index,omitempty"`
	WorkTree  string `json:"workTree,omitempty"`
}

// Snapshot is the immutable starting source state for a run.
type Snapshot struct {
	ID           string              `json:"id"`
	Kind         string              `json:"kind"`
	SourcePath   string              `json:"sourcePath"`
	RepoRoot     string              `json:"repoRoot,omitempty"`
	Branch       string              `json:"branch,omitempty"`
	HEAD         string              `json:"head,omitempty"`
	Detached     bool                `json:"detached,omitempty"`
	RepoIdentity string              `json:"repoIdentity,omitempty"`
	KnowledgeRev string              `json:"knowledgeRev,omitempty"`
	CapturedAt   time.Time           `json:"capturedAt"`
	TreePath     string              `json:"treePath"`
	StagedPath   string              `json:"stagedPath,omitempty"`
	TreeHash     string              `json:"treeHash"`
	Files        map[string]FileMeta `json:"files"`
	Dirty        []DirtyEntry        `json:"dirty,omitempty"`
}

func (s *Snapshot) DirtySet() map[string]DirtyEntry {
	out := make(map[string]DirtyEntry, len(s.Dirty))
	for _, d := range s.Dirty {
		out[d.Path] = d
	}
	return out
}

func (s *Snapshot) WriteMeta() error {
	if s.TreePath == "" {
		return fmt.Errorf("snapshot tree path is empty")
	}
	dir := filepath.Dir(s.TreePath)
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return writeBytes(filepath.Join(dir, "meta.json"), data, ModeFile)
}

func LoadSnapshot(dir string) (*Snapshot, error) {
	data, err := os.ReadFile(filepath.Join(dir, "meta.json"))
	if err != nil {
		return nil, err
	}
	var s Snapshot
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

func captureGit(ctx context.Context, b *GitWorkspaceBackend, opts CaptureOptions) (*Snapshot, error) {
	if opts.SnapshotDir == "" {
		return nil, fmt.Errorf("snapshot dir is required")
	}
	if err := ensureOutside(b.root, opts.SnapshotDir); err != nil {
		return nil, err
	}
	head, err := b.repo.HEAD(ctx)
	if err != nil {
		return nil, err
	}
	branch, err := b.repo.Branch(ctx)
	if err != nil {
		return nil, err
	}
	ident, _ := b.repo.Identity(ctx)
	files, dirty, err := scanGit(ctx, b)
	if err != nil {
		return nil, err
	}
	sid := opts.ID
	if sid == "" {
		sid = id.New()
	}
	tree := filepath.Join(opts.SnapshotDir, "tree")
	staged := filepath.Join(opts.SnapshotDir, "staged")
	if err := os.MkdirAll(tree, 0o755); err != nil {
		return nil, err
	}
	for rel := range files {
		if err := copyRel(b.root, tree, rel); err != nil {
			return nil, fmt.Errorf("snapshot copy %s: %w", rel, err)
		}
	}
	if err := captureStagedBlobs(ctx, b, dirty, staged); err != nil {
		return nil, err
	}
	snap := &Snapshot{
		ID:           sid,
		Kind:         KindGit,
		SourcePath:   b.root,
		RepoRoot:     b.repo.WorkTree,
		Branch:       branch,
		HEAD:         head,
		Detached:     branch == "",
		RepoIdentity: ident,
		KnowledgeRev: opts.KnowledgeRev,
		CapturedAt:   time.Now().UTC(),
		TreePath:     tree,
		StagedPath:   staged,
		Files:        files,
		Dirty:        dirty,
	}
	snap.TreeHash = treeHash(files)
	if err := snap.WriteMeta(); err != nil {
		return nil, err
	}
	return snap, nil
}

func scanGitFiles(ctx context.Context, b *GitWorkspaceBackend) (map[string]FileMeta, error) {
	files, _, err := scanGit(ctx, b)
	return files, err
}

func scanGit(ctx context.Context, b *GitWorkspaceBackend) (map[string]FileMeta, []DirtyEntry, error) {
	status, err := b.repo.Status(ctx)
	if err != nil {
		return nil, nil, err
	}
	tracked, err := b.repo.Tracked(ctx)
	if err != nil {
		return nil, nil, err
	}
	untracked, err := b.repo.Untracked(ctx)
	if err != nil {
		return nil, nil, err
	}
	files := make(map[string]FileMeta)
	add := func(workTreeRel string) error {
		rel, ok := b.inRoot(workTreeRel)
		if !ok {
			return nil
		}
		m, err := lstatMeta(b.root, rel)
		if err != nil {
			return err
		}
		if m.Missing {
			return nil
		}
		files[rel] = m
		return nil
	}
	for _, p := range tracked {
		if err := add(p); err != nil {
			return nil, nil, err
		}
	}
	for _, p := range untracked {
		if err := add(p); err != nil {
			return nil, nil, err
		}
	}
	var dirty []DirtyEntry
	seen := make(map[string]struct{})
	for _, e := range status {
		rel, ok := b.inRoot(e.Path)
		if !ok {
			continue
		}
		if _, dup := seen[rel]; dup {
			continue
		}
		seen[rel] = struct{}{}
		d := DirtyEntry{
			Path:      rel,
			Staged:    e.Staged(),
			Unstaged:  e.Unstaged(),
			Untracked: e.Untracked(),
		}
		if e.OrigPath != "" {
			if orig, ok := b.inRoot(e.OrigPath); ok {
				d.OrigPath = orig
			}
		}
		if e.Index != 0 {
			d.Index = string(e.Index)
		}
		if e.WorkTree != 0 {
			d.WorkTree = string(e.WorkTree)
		}
		dirty = append(dirty, d)
	}
	slices.SortFunc(dirty, func(a, b DirtyEntry) int {
		return strings.Compare(a.Path, b.Path)
	})
	return files, dirty, nil
}

func captureStagedBlobs(ctx context.Context, b *GitWorkspaceBackend, dirty []DirtyEntry, dest string) error {
	if dest == "" {
		return nil
	}
	for _, d := range dirty {
		if !d.Staged || d.Index == "D" {
			continue
		}
		blob, err := b.repo.ShowIndex(ctx, d.Path)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return err
		}
		wp := filepath.Join(b.root, filepath.FromSlash(d.Path))
		if sameWorktreeContent(wp, blob) {
			continue
		}
		if err := os.MkdirAll(dest, 0o755); err != nil {
			return err
		}
		if err := writeBytes(filepath.Join(dest, filepath.FromSlash(d.Path)), blob, ModeFile); err != nil {
			return err
		}
	}
	return nil
}

func sameWorktreeContent(path string, blob []byte) bool {
	fi, err := os.Lstat(path)
	if err != nil {
		return false
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		t, err := os.Readlink(path)
		return err == nil && t == string(blob)
	}
	if !fi.Mode().IsRegular() {
		return false
	}
	data, err := os.ReadFile(path)
	return err == nil && string(data) == string(blob)
}

func materializeGit(ctx context.Context, b *GitWorkspaceBackend, snap *Snapshot, dest string) error {
	if snap == nil || snap.TreePath == "" {
		return fmt.Errorf("snapshot tree is missing")
	}
	abs, err := absClean(dest)
	if err != nil {
		return err
	}
	if err := ensureOutside(b.root, abs); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return err
	}
	if _, err := os.Stat(abs); err == nil {
		ents, _ := os.ReadDir(abs)
		if len(ents) > 0 {
			return fmt.Errorf("materialize dest is not empty: %s", abs)
		}
	}
	if snap.HEAD != "" {
		if _, err := gitutil.CloneIsolated(ctx, b.repo.WorkTree, abs, snap.HEAD, snap.Branch); err != nil {
			return err
		}
	} else if _, err := gitutil.Init(ctx, abs, snap.Branch); err != nil {
		return err
	}
	if err := overlaySnapshot(snap, abs); err != nil {
		return err
	}
	return restoreIndex(ctx, snap, abs)
}

func overlaySnapshot(snap *Snapshot, dest string) error {
	have, err := walkMeaningful(dest)
	if err != nil {
		return err
	}
	for rel := range have {
		if _, ok := snap.Files[rel]; ok {
			continue
		}
		p := filepath.Join(dest, filepath.FromSlash(rel))
		if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		pruneEmpty(dest, rel)
	}
	for rel := range snap.Files {
		if err := copyRel(snap.TreePath, dest, rel); err != nil {
			return err
		}
	}
	return nil
}

func restoreIndex(ctx context.Context, snap *Snapshot, dest string) error {
	repo, err := gitutil.DiscoverContext(ctx, dest)
	if err != nil {
		return nil
	}
	var add []string
	var rm []string
	var restoreWT []string
	for _, d := range snap.Dirty {
		if !d.Staged {
			continue
		}
		if d.Index == "D" {
			rm = append(rm, d.Path)
			continue
		}
		stagedFile := filepath.Join(snap.StagedPath, filepath.FromSlash(d.Path))
		if _, err := os.Lstat(stagedFile); err == nil {
			wt := filepath.Join(dest, filepath.FromSlash(d.Path))
			if err := copyPath(stagedFile, wt); err != nil {
				return err
			}
			restoreWT = append(restoreWT, d.Path)
		}
		add = append(add, d.Path)
	}
	if len(rm) > 0 {
		if err := repo.RmCached(ctx, rm...); err != nil {
			return err
		}
	}
	if len(add) > 0 {
		if err := repo.Add(ctx, add...); err != nil {
			return err
		}
	}
	for _, p := range restoreWT {
		if _, ok := snap.Files[p]; ok {
			if err := copyRel(snap.TreePath, dest, p); err != nil {
				return err
			}
			continue
		}
		if err := os.Remove(filepath.Join(dest, filepath.FromSlash(p))); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

func captureFilesystem(ctx context.Context, b *FilesystemWorkspaceBackend, opts CaptureOptions) (*Snapshot, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if opts.SnapshotDir == "" {
		return nil, fmt.Errorf("snapshot dir is required")
	}
	if err := ensureOutside(b.root, opts.SnapshotDir); err != nil {
		return nil, err
	}
	files, err := walkMeaningful(b.root)
	if err != nil {
		return nil, err
	}
	sid := opts.ID
	if sid == "" {
		sid = id.New()
	}
	tree := filepath.Join(opts.SnapshotDir, "tree")
	if err := os.MkdirAll(tree, 0o755); err != nil {
		return nil, err
	}
	for rel := range files {
		if err := copyRel(b.root, tree, rel); err != nil {
			return nil, fmt.Errorf("snapshot copy %s: %w", rel, err)
		}
	}
	snap := &Snapshot{
		ID:           sid,
		Kind:         KindFilesystem,
		SourcePath:   b.root,
		KnowledgeRev: opts.KnowledgeRev,
		CapturedAt:   time.Now().UTC(),
		TreePath:     tree,
		Files:        files,
	}
	snap.TreeHash = treeHash(files)
	if err := snap.WriteMeta(); err != nil {
		return nil, err
	}
	return snap, nil
}

func materializeFilesystem(ctx context.Context, snap *Snapshot, dest string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if snap == nil || snap.TreePath == "" {
		return fmt.Errorf("snapshot tree is missing")
	}
	abs, err := absClean(dest)
	if err != nil {
		return err
	}
	if err := ensureOutside(snap.SourcePath, abs); err != nil {
		return err
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return err
	}
	return copyTree(snap.TreePath, abs)
}

func treeHash(files map[string]FileMeta) string {
	keys := make([]string, 0, len(files))
	for p := range files {
		keys = append(keys, p)
	}
	slices.Sort(keys)
	var b strings.Builder
	for _, p := range keys {
		m := files[p]
		fmt.Fprintf(&b, "%s %s %s %s %s\n", p, m.Type, m.Mode, m.Hash, m.Target)
	}
	return HashString(b.String())
}
