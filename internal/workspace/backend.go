package workspace

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Wayshard/wayshard/internal/gitutil"
)

const (
	KindGit        = "git"
	KindFilesystem = "filesystem"
)

// WorkspaceBackend captures, materializes, and inspects a source tree without
// writing to it. Git and non-Git projects share this interface.
type WorkspaceBackend interface {
	Kind() string
	SourcePath() string
	Identity() SourceIdentity
	CaptureSnapshot(ctx context.Context, opts CaptureOptions) (*Snapshot, error)
	Materialize(ctx context.Context, snap *Snapshot, dest string) error
	CurrentState(ctx context.Context) (*SourceState, error)
}

// SourceIdentity is the concrete source/repository identity used to serialize
// integrations. It is not merely a ProjectID.
type SourceIdentity struct {
	Kind         string
	Path         string
	RepoIdentity string
}

// Key uniquely identifies the live source tree being published into.
func (id SourceIdentity) Key() string {
	return id.Kind + ":" + id.Path
}

type SourceState struct {
	Kind     string
	Branch   string
	HEAD     string
	Detached bool
	Files    map[string]FileMeta
}

// Open selects GitWorkspaceBackend when path is inside a git work tree and a
// git binary is available; otherwise FilesystemWorkspaceBackend.
func Open(path string) (WorkspaceBackend, error) {
	abs, err := absClean(path)
	if err != nil {
		return nil, err
	}
	fi, err := os.Stat(abs)
	if err != nil {
		return nil, err
	}
	if !fi.IsDir() {
		return nil, fmt.Errorf("%w: %s", ErrNotDir, abs)
	}
	if gitutil.Available() {
		repo, err := gitutil.Discover(abs)
		if err == nil {
			return NewGitBackend(repo, abs), nil
		}
	}
	return NewFilesystemBackend(abs), nil
}

// GitWorkspaceBackend is a git work tree. Inspection uses read-only git
// commands plus a copied index; the user's tree is never mutated.
type GitWorkspaceBackend struct {
	repo *gitutil.Repo
	root string
}

func NewGitBackend(repo *gitutil.Repo, root string) *GitWorkspaceBackend {
	if root == "" {
		root = repo.WorkTree
	}
	return &GitWorkspaceBackend{repo: repo, root: root}
}

func (b *GitWorkspaceBackend) Kind() string        { return KindGit }
func (b *GitWorkspaceBackend) SourcePath() string  { return b.root }
func (b *GitWorkspaceBackend) Repo() *gitutil.Repo { return b.repo }

func (b *GitWorkspaceBackend) Identity() SourceIdentity {
	id := SourceIdentity{Kind: KindGit, Path: b.root}
	if ident, err := b.repo.Identity(context.Background()); err == nil {
		id.RepoIdentity = ident
	}
	return id
}

func (b *GitWorkspaceBackend) CaptureSnapshot(ctx context.Context, opts CaptureOptions) (*Snapshot, error) {
	return captureGit(ctx, b, opts)
}

func (b *GitWorkspaceBackend) Materialize(ctx context.Context, snap *Snapshot, dest string) error {
	return materializeGit(ctx, b, snap, dest)
}

func (b *GitWorkspaceBackend) CurrentState(ctx context.Context) (*SourceState, error) {
	files, err := scanGitFiles(ctx, b)
	if err != nil {
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
	return &SourceState{
		Kind:     KindGit,
		Branch:   branch,
		HEAD:     head,
		Detached: branch == "",
		Files:    files,
	}, nil
}

func (b *GitWorkspaceBackend) inRoot(workTreeRel string) (string, bool) {
	abs := filepath.Join(b.repo.WorkTree, filepath.FromSlash(workTreeRel))
	rel, err := filepath.Rel(b.root, abs)
	if err != nil {
		return "", false
	}
	rel = filepath.ToSlash(rel)
	if rel == "." || rel == "" {
		return "", false
	}
	if _, err := SafeRel(rel); err != nil {
		return "", false
	}
	return rel, true
}

// FilesystemWorkspaceBackend is a non-git directory tree.
type FilesystemWorkspaceBackend struct {
	root string
}

func NewFilesystemBackend(root string) *FilesystemWorkspaceBackend {
	return &FilesystemWorkspaceBackend{root: root}
}

func (b *FilesystemWorkspaceBackend) Kind() string       { return KindFilesystem }
func (b *FilesystemWorkspaceBackend) SourcePath() string { return b.root }

func (b *FilesystemWorkspaceBackend) Identity() SourceIdentity {
	return SourceIdentity{Kind: KindFilesystem, Path: b.root}
}

func (b *FilesystemWorkspaceBackend) CaptureSnapshot(ctx context.Context, opts CaptureOptions) (*Snapshot, error) {
	return captureFilesystem(ctx, b, opts)
}

func (b *FilesystemWorkspaceBackend) Materialize(ctx context.Context, snap *Snapshot, dest string) error {
	return materializeFilesystem(ctx, snap, dest)
}

func (b *FilesystemWorkspaceBackend) CurrentState(ctx context.Context) (*SourceState, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	files, err := walkMeaningful(b.root)
	if err != nil {
		return nil, err
	}
	return &SourceState{Kind: KindFilesystem, Files: files}, nil
}
