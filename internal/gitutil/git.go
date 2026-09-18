// Package gitutil wraps the git CLI for repository inspection and isolated
// workspace materialization. It is not a Git GUI: there is no staging,
// branching, or remotes product surface here.
package gitutil

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var (
	ErrGitNotFound = errors.New("git binary not found")
	ErrNotRepo     = errors.New("not a git work tree")
)

var (
	gitOnce sync.Once
	gitBin  string
	gitErr  error
)

// Bin returns the git executable path.
func Bin() (string, error) {
	gitOnce.Do(func() {
		gitBin, gitErr = exec.LookPath("git")
		if gitErr != nil {
			gitErr = ErrGitNotFound
		}
	})
	return gitBin, gitErr
}

// Available reports whether a git binary is on PATH.
func Available() bool {
	_, err := Bin()
	return err == nil
}

// Repo is a detected git work tree. Commands that inspect the source tree
// use a copied index so they do not refresh the user's index file.
type Repo struct {
	WorkTree  string
	GitDir    string
	CommonDir string
}

// Discover locates the git work tree containing path.
func Discover(path string) (*Repo, error) {
	return DiscoverContext(context.Background(), path)
}

func DiscoverContext(ctx context.Context, path string) (*Repo, error) {
	if _, err := Bin(); err != nil {
		return nil, err
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	fi, err := os.Stat(abs)
	if err != nil {
		return nil, err
	}
	if !fi.IsDir() {
		abs = filepath.Dir(abs)
	}
	inside, err := run(ctx, abs, nil, "rev-parse", "--is-inside-work-tree")
	if err != nil || strings.TrimSpace(string(inside)) != "true" {
		return nil, ErrNotRepo
	}
	top, err := run(ctx, abs, nil, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNotRepo, err)
	}
	gitDir, err := run(ctx, abs, nil, "rev-parse", "--absolute-git-dir")
	if err != nil {
		return nil, err
	}
	common, err := run(ctx, abs, nil, "rev-parse", "--absolute-git-common-dir")
	if err != nil {
		common = gitDir
	}
	return &Repo{
		WorkTree:  strings.TrimSpace(string(top)),
		GitDir:    strings.TrimSpace(string(gitDir)),
		CommonDir: strings.TrimSpace(string(common)),
	}, nil
}

// HEAD returns the current commit SHA, or "" if the repository has no commits.
func (r *Repo) HEAD(ctx context.Context) (string, error) {
	out, err := r.run(ctx, nil, "rev-parse", "HEAD")
	if err != nil {
		if isNoCommits(err) {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// Branch returns the current branch name. Empty means detached HEAD.
func (r *Repo) Branch(ctx context.Context) (string, error) {
	out, err := r.run(ctx, nil, "symbolic-ref", "--quiet", "--short", "HEAD")
	if err != nil {
		return "", nil
	}
	return strings.TrimSpace(string(out)), nil
}

// Identity is a stable repository identity: origin URL if set, otherwise the
// absolute git common directory.
func (r *Repo) Identity(ctx context.Context) (string, error) {
	out, err := r.run(ctx, nil, "config", "--get", "remote.origin.url")
	if err == nil {
		if u := strings.TrimSpace(string(out)); u != "" {
			return u, nil
		}
	}
	if r.CommonDir != "" {
		return r.CommonDir, nil
	}
	return r.WorkTree, nil
}

// StatusEntry is one porcelain v1 status line.
type StatusEntry struct {
	Path     string
	OrigPath string
	Index    byte
	WorkTree byte
}

func (e StatusEntry) Untracked() bool { return e.Index == '?' && e.WorkTree == '?' }

func (e StatusEntry) Ignored() bool { return e.Index == '!' && e.WorkTree == '!' }

func (e StatusEntry) Staged() bool {
	return !e.Untracked() && !e.Ignored() && e.Index != ' ' && e.Index != '?'
}

func (e StatusEntry) Unstaged() bool {
	return !e.Untracked() && !e.Ignored() && e.WorkTree != ' ' && e.WorkTree != '?'
}

func (e StatusEntry) Unmerged() bool {
	return e.Index == 'U' || e.WorkTree == 'U' ||
		(e.Index == 'A' && e.WorkTree == 'A') ||
		(e.Index == 'D' && e.WorkTree == 'D')
}

// Status returns porcelain v1 entries, including untracked files that are not
// gitignored. The source index is not refreshed.
func (r *Repo) Status(ctx context.Context) ([]StatusEntry, error) {
	env, cleanup, err := r.frozenIndexEnv(ctx)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	out, err := r.run(ctx, env, "status", "--porcelain=v1", "-z", "--untracked-files=all")
	if err != nil {
		return nil, err
	}
	return parsePorcelainZ(out)
}

// Tracked lists index paths (relative to the work tree).
func (r *Repo) Tracked(ctx context.Context) ([]string, error) {
	env, cleanup, err := r.frozenIndexEnv(ctx)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	out, err := r.run(ctx, env, "ls-files", "-z")
	if err != nil {
		return nil, err
	}
	return splitZ(out), nil
}

// Untracked lists non-ignored untracked files.
func (r *Repo) Untracked(ctx context.Context) ([]string, error) {
	env, cleanup, err := r.frozenIndexEnv(ctx)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	out, err := r.run(ctx, env, "ls-files", "-z", "--others", "--exclude-standard")
	if err != nil {
		return nil, err
	}
	paths := splitZ(out)
	var keep []string
	for _, p := range paths {
		if r.Ignored(ctx, p) {
			continue
		}
		keep = append(keep, p)
	}
	return keep, nil
}

func (r *Repo) Ignored(ctx context.Context, rel string) bool {
	_, err := r.run(ctx, nil, "check-ignore", "-q", "--", rel)
	return err == nil
}

// ShowHEAD returns the blob at HEAD:path. Missing path yields os.ErrNotExist.
func (r *Repo) ShowHEAD(ctx context.Context, rel string) ([]byte, error) {
	rel = filepath.ToSlash(rel)
	out, err := r.run(ctx, nil, "cat-file", "-p", "HEAD:"+rel)
	if err != nil {
		return nil, os.ErrNotExist
	}
	return out, nil
}

// ShowIndex returns the blob staged at path. Missing path yields os.ErrNotExist.
func (r *Repo) ShowIndex(ctx context.Context, rel string) ([]byte, error) {
	rel = filepath.ToSlash(rel)
	env, cleanup, err := r.frozenIndexEnv(ctx)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	out, err := r.run(ctx, env, "show", ":"+rel)
	if err != nil {
		return nil, os.ErrNotExist
	}
	return out, nil
}

// DirtyCapture is a stash-like copy of dirty worktree and staged-only blobs.
// The source repository is not mutated.
type DirtyCapture struct {
	Entries  []StatusEntry
	WorkDir  string
	StageDir string
}

// CaptureDirty copies dirty worktree files into workDest. Staged blobs that
// differ from the worktree (or exist only in the index) are written to
// stageDest. Neither destination is required; pass "" to skip that side.
func (r *Repo) CaptureDirty(ctx context.Context, workDest, stageDest string) (*DirtyCapture, error) {
	entries, err := r.Status(ctx)
	if err != nil {
		return nil, err
	}
	cap := &DirtyCapture{Entries: entries, WorkDir: workDest, StageDir: stageDest}
	for _, e := range entries {
		if e.Ignored() {
			continue
		}
		if workDest != "" && e.WorkTree != 'D' && e.WorkTree != ' ' && e.Path != "" {
			src := filepath.Join(r.WorkTree, filepath.FromSlash(e.Path))
			dst := filepath.Join(workDest, filepath.FromSlash(e.Path))
			if err := copyPath(src, dst); err != nil && !errors.Is(err, os.ErrNotExist) {
				return nil, fmt.Errorf("capture %s: %w", e.Path, err)
			}
		}
		if stageDest != "" && e.Staged() && e.Index != 'D' {
			blob, err := r.ShowIndex(ctx, e.Path)
			if err != nil {
				if errors.Is(err, os.ErrNotExist) {
					continue
				}
				return nil, fmt.Errorf("staged %s: %w", e.Path, err)
			}
			var work []byte
			wp := filepath.Join(r.WorkTree, filepath.FromSlash(e.Path))
			if fi, statErr := os.Lstat(wp); statErr == nil && fi.Mode()&os.ModeSymlink == 0 && fi.Mode().IsRegular() {
				work, _ = os.ReadFile(wp)
			}
			if bytes.Equal(blob, work) {
				continue
			}
			dst := filepath.Join(stageDest, filepath.FromSlash(e.Path))
			if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
				return nil, err
			}
			if err := os.WriteFile(dst, blob, 0o644); err != nil {
				return nil, err
			}
		}
	}
	return cap, nil
}

// Init creates a new git repository at dest. Used only for isolated workspaces
// when the source has no commits.
func Init(ctx context.Context, dest, branch string) (*Repo, error) {
	bin, err := Bin()
	if err != nil {
		return nil, err
	}
	args := []string{"-c", "core.hooksPath=/dev/null", "init"}
	if branch != "" {
		args = append(args, "-b", branch)
	}
	args = append(args, dest)
	if _, err := runBin(ctx, bin, "", nil, args...); err != nil {
		return nil, err
	}
	repo, err := DiscoverContext(ctx, dest)
	if err != nil {
		return nil, err
	}
	_, _ = repo.run(ctx, nil, "config", "core.hooksPath", "/dev/null")
	return repo, nil
}

// CloneIsolated clones src into dest at the given HEAD without sharing object
// hardlinks or remotes. dest must not exist. Hooks are disabled.
func CloneIsolated(ctx context.Context, srcWorkTree, dest, head, branch string) (*Repo, error) {
	bin, err := Bin()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return nil, err
	}
	args := []string{
		"-c", "core.hooksPath=/dev/null",
		"-c", "advice.detachedHead=false",
		"clone", "--local", "--no-hardlinks", "--no-checkout", "--template=",
		srcWorkTree, dest,
	}
	if _, err := runBin(ctx, bin, "", nil, args...); err != nil {
		return nil, err
	}
	repo, err := DiscoverContext(ctx, dest)
	if err != nil {
		return nil, err
	}
	if _, err := repo.run(ctx, nil, "config", "core.hooksPath", "/dev/null"); err != nil {
		return nil, err
	}
	remotes, _ := repo.run(ctx, nil, "remote")
	for _, name := range strings.Fields(string(remotes)) {
		_, _ = repo.run(ctx, nil, "remote", "remove", name)
	}
	if head == "" {
		return repo, nil
	}
	if branch != "" {
		_, err = repo.run(ctx, nil, "checkout", "-B", branch, head)
	} else {
		_, err = repo.run(ctx, nil, "checkout", "--detach", head)
	}
	if err != nil {
		return nil, err
	}
	return repo, nil
}

// Add stages paths in this repo (used only on isolated run workspaces).
func (r *Repo) Add(ctx context.Context, rels ...string) error {
	if len(rels) == 0 {
		return nil
	}
	args := append([]string{"add", "-A", "--"}, rels...)
	_, err := r.run(ctx, nil, args...)
	return err
}

// RmCached removes paths from the index only.
func (r *Repo) RmCached(ctx context.Context, rels ...string) error {
	if len(rels) == 0 {
		return nil
	}
	args := append([]string{"rm", "--cached", "-f", "--"}, rels...)
	_, err := r.run(ctx, nil, args...)
	return err
}

func (r *Repo) run(ctx context.Context, extraEnv []string, args ...string) ([]byte, error) {
	return run(ctx, r.WorkTree, extraEnv, args...)
}

func (r *Repo) frozenIndexEnv(ctx context.Context) ([]string, func(), error) {
	out, err := r.run(ctx, nil, "rev-parse", "--git-path", "index")
	if err != nil {
		return baseEnv(), func() {}, nil
	}
	idx := strings.TrimSpace(string(out))
	if idx == "" {
		return baseEnv(), func() {}, nil
	}
	if !filepath.IsAbs(idx) {
		idx = filepath.Join(r.WorkTree, idx)
	}
	if _, err := os.Stat(idx); err != nil {
		return baseEnv(), func() {}, nil
	}
	tmp, err := os.CreateTemp("", "wayshard-index-*")
	if err != nil {
		return nil, nil, err
	}
	tmpName := tmp.Name()
	src, err := os.Open(idx)
	if err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return nil, nil, err
	}
	_, copyErr := io.Copy(tmp, src)
	src.Close()
	closeErr := tmp.Close()
	if copyErr != nil || closeErr != nil {
		os.Remove(tmpName)
		if copyErr != nil {
			return nil, nil, copyErr
		}
		return nil, nil, closeErr
	}
	env := append(baseEnv(), "GIT_INDEX_FILE="+tmpName)
	return env, func() { os.Remove(tmpName) }, nil
}

func run(ctx context.Context, dir string, extraEnv []string, args ...string) ([]byte, error) {
	bin, err := Bin()
	if err != nil {
		return nil, err
	}
	full := append([]string{"-c", "core.hooksPath=/dev/null", "-c", "core.quotepath=false"}, args...)
	return runBin(ctx, bin, dir, extraEnv, full...)
}

func runBin(ctx context.Context, bin, dir string, extraEnv []string, args ...string) ([]byte, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Env = mergeEnv(extraEnv)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if len(msg) > 2048 {
			msg = msg[:2048]
		}
		if msg == "" {
			msg = strings.TrimSpace(stdout.String())
		}
		return stdout.Bytes(), fmt.Errorf("git %s: %w (%s)", strings.Join(args, " "), err, msg)
	}
	return stdout.Bytes(), nil
}

func baseEnv() []string {
	return append(os.Environ(),
		"GIT_TERMINAL_PROMPT=0",
		"GIT_OPTIONAL_LOCKS=0",
		"GIT_ASKPASS=echo",
		"GCM_INTERACTIVE=never",
		"GIT_CONFIG_NOSYSTEM=1",
	)
}

func mergeEnv(extra []string) []string {
	if len(extra) == 0 {
		return baseEnv()
	}
	return extra
}

func parsePorcelainZ(data []byte) ([]StatusEntry, error) {
	if len(data) == 0 {
		return nil, nil
	}
	parts := bytes.Split(data, []byte{0})
	var out []StatusEntry
	for i := 0; i < len(parts); i++ {
		p := parts[i]
		if len(p) == 0 {
			continue
		}
		if len(p) < 3 {
			return nil, fmt.Errorf("invalid git status entry %q", p)
		}
		e := StatusEntry{Index: p[0], WorkTree: p[1]}
		path := string(p[3:])
		if e.Index == 'R' || e.Index == 'C' || e.WorkTree == 'R' || e.WorkTree == 'C' {
			i++
			if i >= len(parts) {
				return nil, fmt.Errorf("rename status missing destination for %q", path)
			}
			e.OrigPath = path
			e.Path = string(parts[i])
		} else {
			e.Path = path
		}
		e.Path = filepath.ToSlash(e.Path)
		e.OrigPath = filepath.ToSlash(e.OrigPath)
		out = append(out, e)
	}
	return out, nil
}

func splitZ(data []byte) []string {
	if len(data) == 0 {
		return nil
	}
	parts := bytes.Split(data, []byte{0})
	var out []string
	for _, p := range parts {
		if len(p) == 0 {
			continue
		}
		out = append(out, filepath.ToSlash(string(p)))
	}
	return out
}

func isNoCommits(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "ambiguous argument") ||
		strings.Contains(s, "unknown revision") ||
		strings.Contains(s, "bad revision") ||
		strings.Contains(s, "needed a single revision") ||
		strings.Contains(s, "does not have any commits") ||
		strings.Contains(s, "not a valid object name")
}

func copyPath(src, dst string) error {
	fi, err := os.Lstat(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(src)
		if err != nil {
			return err
		}
		_ = os.Remove(dst)
		return os.Symlink(target, dst)
	}
	if !fi.Mode().IsRegular() {
		return nil
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp := dst + ".tmp"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, fi.Mode().Perm())
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		os.Remove(tmp)
		return copyErr
	}
	if closeErr != nil {
		os.Remove(tmp)
		return closeErr
	}
	if err := os.Chmod(tmp, fi.Mode().Perm()); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, dst); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}
