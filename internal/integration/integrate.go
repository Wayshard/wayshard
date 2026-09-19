// Package integration performs server-controlled three-way publication of a
// run-final tree into the live source workspace.
//
//	base = starting snapshot
//	ours = current source
//	run  = run-final workspace
//
// Conflicts leave the source untouched. Publication is journaled with
// file-level before/after hashes and is not auto-staged or auto-committed.
package integration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/Wayshard/wayshard/internal/domain"
	"github.com/Wayshard/wayshard/internal/id"
	"github.com/Wayshard/wayshard/internal/workspace"
)

const (
	StatusPreparing  = "preparing"
	StatusPublishing = "publishing"
	StatusPublished  = "published"
	StatusBlocked    = "blocked"
	StatusDiverged   = "diverged"
	StatusIncomplete = "incomplete"

	RecoveryCompleted  = "completed"
	RecoveryIncomplete = "incomplete"
	RecoveryDiverged   = "externally_diverged"

	OpCreate = "create"
	OpModify = "modify"
	OpDelete = "delete"

	EntryPending  = "pending"
	EntryWritten  = "written"
	EntryVerified = "verified"
	EntryFailed   = "failed"

	ReasonConflict      = "conflict"
	ReasonBranchChanged = "branch_changed"
	ReasonDiverged      = "source_diverged"
	ReasonCrash         = "publication_interrupted"
)

var (
	ErrBlocked    = errors.New("integration blocked")
	ErrDiverged   = errors.New("source diverged during publication")
	ErrIncomplete = errors.New("publication incomplete")
)

var sourceLocks sync.Map // identity key -> *sync.Mutex

func lockSource(key string) func() {
	v, _ := sourceLocks.LoadOrStore(key, &sync.Mutex{})
	mu := v.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}

// Request is a server-controlled integrate attempt.
type Request struct {
	RunID             string
	ProjectID         string
	Source            workspace.WorkspaceBackend
	Snapshot          *workspace.Snapshot
	RunWorkspace      string
	WorkDir           string
	JournalDir        string
	AllowBranchChange bool
	AutoStage         bool
	AutoCommit        bool
	// CrashAfter is a test hook: stop after N verified writes.
	CrashAfter int
}

type Conflict struct {
	Path   string             `json:"path"`
	Reason string             `json:"reason"`
	Base   workspace.FileMeta `json:"base"`
	Ours   workspace.FileMeta `json:"ours"`
	Run    workspace.FileMeta `json:"run"`
}

type JournalEntry struct {
	Path        string `json:"path"`
	Op          string `json:"op"`
	BeforeHash  string `json:"beforeHash"`
	AfterHash   string `json:"afterHash"`
	BeforeMode  string `json:"beforeMode,omitempty"`
	AfterMode   string `json:"afterMode,omitempty"`
	AfterType   string `json:"afterType,omitempty"`
	AfterTarget string `json:"afterTarget,omitempty"`
	Status      string `json:"status"`
}

type Journal struct {
	IntegrationID string         `json:"integrationId"`
	RunID         string         `json:"runId"`
	ProjectID     string         `json:"projectId"`
	SourcePath    string         `json:"sourcePath"`
	IdentityKey   string         `json:"identityKey"`
	SnapshotID    string         `json:"snapshotId"`
	TargetBranch  string         `json:"targetBranch"`
	CurrentBranch string         `json:"currentBranch"`
	CreatedAt     time.Time      `json:"createdAt"`
	Entries       []JournalEntry `json:"entries"`
	Dir           string         `json:"-"`
}

func (j *Journal) Path() string {
	if j.Dir == "" {
		return ""
	}
	return filepath.Join(j.Dir, "journal.json")
}

func (j *Journal) blobPath(hash string) string {
	return filepath.Join(j.Dir, "blobs", hash)
}

type Result struct {
	Status    string
	Reason    string
	Conflicts []Conflict
	Journal   *Journal
	Record    domain.Integration
	Published []string
}

type FileRecovery struct {
	Path        string
	Status      string
	CurrentHash string
}

type Recovery struct {
	Status  string
	Files   []FileRecovery
	Journal *Journal
}

func (r *Recovery) Incomplete() bool { return r != nil && r.Status == RecoveryIncomplete }
func (r *Recovery) Diverged() bool   { return r != nil && r.Status == RecoveryDiverged }
func (r *Recovery) Completed() bool  { return r != nil && r.Status == RecoveryCompleted }

// Integrate prepares a three-way merge in a temp workspace and, if clean,
// journals and publishes into the live source tree.
func Integrate(ctx context.Context, req Request) (*Result, error) {
	if req.Source == nil || req.Snapshot == nil {
		return nil, fmt.Errorf("source and snapshot are required")
	}
	if req.RunWorkspace == "" {
		return nil, fmt.Errorf("run workspace is required")
	}
	ident := req.Source.Identity()
	unlock := lockSource(ident.Key())
	defer unlock()

	res := &Result{
		Status: StatusPreparing,
		Record: domain.Integration{
			ID:           id.New(),
			RunID:        req.RunID,
			ProjectID:    req.ProjectID,
			Status:       StatusPreparing,
			BaseSnapshot: req.Snapshot.ID,
			TargetBranch: req.Snapshot.Branch,
			CreatedAt:    time.Now().UTC(),
			UpdatedAt:    time.Now().UTC(),
		},
	}

	cur, err := req.Source.CurrentState(ctx)
	if err != nil {
		return nil, err
	}
	res.Record.CurrentBranch = cur.Branch

	if req.Source.Kind() == workspace.KindGit && !req.AllowBranchChange {
		if cur.Branch != req.Snapshot.Branch {
			res.Status = StatusBlocked
			res.Reason = ReasonBranchChanged
			res.Record.Status = StatusBlocked
			res.Record.Error = fmt.Sprintf("branch changed from %q to %q", req.Snapshot.Branch, cur.Branch)
			return res, nil
		}
	}

	runFiles, err := walkRun(ctx, req.RunWorkspace)
	if err != nil {
		return nil, err
	}
	conflicts, plan := threeWay(req.Snapshot.Files, cur.Files, runFiles)
	if len(conflicts) > 0 {
		res.Status = StatusBlocked
		res.Reason = ReasonConflict
		res.Conflicts = conflicts
		res.Record.Status = StatusBlocked
		res.Record.Error = fmt.Sprintf("%d conflict(s)", len(conflicts))
		return res, nil
	}

	work := req.WorkDir
	if work == "" {
		work, err = os.MkdirTemp("", "wayshard-integrate-*")
		if err != nil {
			return nil, err
		}
	}
	jdir := req.JournalDir
	if jdir == "" {
		jdir = filepath.Join(work, "journal")
	}
	jdir = filepath.Join(jdir, res.Record.ID)
	if err := os.MkdirAll(filepath.Join(jdir, "blobs"), 0o755); err != nil {
		return nil, err
	}
	prepDir := filepath.Join(work, "prep-"+res.Record.ID)
	if err := os.MkdirAll(prepDir, 0o755); err != nil {
		return nil, err
	}

	journal := &Journal{
		IntegrationID: res.Record.ID,
		RunID:         req.RunID,
		ProjectID:     req.ProjectID,
		SourcePath:    req.Source.SourcePath(),
		IdentityKey:   ident.Key(),
		SnapshotID:    req.Snapshot.ID,
		TargetBranch:  req.Snapshot.Branch,
		CurrentBranch: cur.Branch,
		CreatedAt:     time.Now().UTC(),
		Entries:       plan,
		Dir:           jdir,
	}

	srcRoot := req.Source.SourcePath()
	for i := range journal.Entries {
		e := &journal.Entries[i]
		if e.Op == OpDelete {
			continue
		}
		src := filepath.Join(req.RunWorkspace, filepath.FromSlash(e.Path))
		if e.AfterType == workspace.TypeSymlink {
			if err := os.WriteFile(journal.blobPath(e.AfterHash), []byte(e.AfterTarget), 0o644); err != nil {
				return nil, err
			}
			continue
		}
		if err := workspaceCopyBlob(src, journal.blobPath(e.AfterHash)); err != nil {
			return nil, fmt.Errorf("blob %s: %w", e.Path, err)
		}
		if err := workspaceCopyBlob(src, filepath.Join(prepDir, filepath.FromSlash(e.Path))); err != nil {
			return nil, err
		}
	}
	if err := journal.Save(); err != nil {
		return nil, err
	}
	res.Journal = journal
	res.Record.JournalHash = workspace.HashString(journal.Path())
	res.Record.Status = StatusPublishing

	// Re-check source hashes immediately before the first write.
	if err := recheckSource(srcRoot, journal.Entries); err != nil {
		res.Status = StatusDiverged
		res.Reason = ReasonDiverged
		res.Record.Status = StatusDiverged
		res.Record.Error = err.Error()
		return res, nil
	}

	published, pubErr := publish(ctx, srcRoot, journal, req.CrashAfter)
	res.Published = published
	if pubErr != nil && req.CrashAfter > 0 && errors.Is(pubErr, ErrIncomplete) {
		res.Status = StatusIncomplete
		res.Reason = ReasonCrash
		res.Record.Status = StatusIncomplete
		res.Record.Error = pubErr.Error()
		return res, nil
	}
	if pubErr != nil {
		rec, _ := ClassifyJournal(ctx, srcRoot, journal)
		if rec != nil && rec.Diverged() {
			res.Status = StatusDiverged
			res.Reason = ReasonDiverged
		} else {
			res.Status = StatusIncomplete
			res.Reason = ReasonCrash
		}
		res.Record.Status = res.Status
		res.Record.Error = pubErr.Error()
		return res, nil
	}

	if req.AutoStage || req.AutoCommit {
		if err := maybeGitStage(ctx, req, journal); err != nil {
			res.Status = StatusIncomplete
			res.Record.Status = StatusIncomplete
			res.Record.Error = err.Error()
			return res, err
		}
	}

	res.Status = StatusPublished
	res.Record.Status = StatusPublished
	res.Record.UpdatedAt = time.Now().UTC()
	return res, nil
}

func walkRun(ctx context.Context, dir string) (map[string]workspace.FileMeta, error) {
	cur, err := workspace.Open(dir)
	if err != nil {
		return nil, err
	}
	st, err := cur.CurrentState(ctx)
	if err != nil {
		return nil, err
	}
	return st.Files, nil
}

func metaOrMissing(files map[string]workspace.FileMeta, path string) workspace.FileMeta {
	if m, ok := files[path]; ok {
		return m
	}
	return workspace.FileMeta{Path: path, Missing: true}
}

func threeWay(base, ours, run map[string]workspace.FileMeta) ([]Conflict, []JournalEntry) {
	paths := make(map[string]struct{})
	for p := range base {
		paths[p] = struct{}{}
	}
	for p := range ours {
		paths[p] = struct{}{}
	}
	for p := range run {
		paths[p] = struct{}{}
	}
	keys := make([]string, 0, len(paths))
	for p := range paths {
		keys = append(keys, p)
	}
	slices.Sort(keys)

	var conflicts []Conflict
	var plan []JournalEntry
	for _, p := range keys {
		b := metaOrMissing(base, p)
		o := metaOrMissing(ours, p)
		r := metaOrMissing(run, p)
		switch {
		case r.Equal(b):
			// Agent made no change: keep ours.
		case o.Equal(b):
			// User made no change: take run.
			if !o.Equal(r) {
				plan = append(plan, entryFor(p, o, r))
			}
		case o.Equal(r):
			// Already matches run-final.
		default:
			conflicts = append(conflicts, Conflict{
				Path:   p,
				Reason: conflictReason(b, o, r),
				Base:   b,
				Ours:   o,
				Run:    r,
			})
		}
	}
	return conflicts, plan
}

func conflictReason(b, o, r workspace.FileMeta) string {
	switch {
	case !b.Missing && o.Missing && !r.Missing && !r.Equal(b):
		return "delete_modify"
	case !b.Missing && r.Missing && !o.Missing && !o.Equal(b):
		return "modify_delete"
	case b.Missing && !o.Missing && !r.Missing:
		return "add_add"
	case !o.Missing && !r.Missing && o.Type != r.Type:
		return "type_change"
	default:
		return "both_modified"
	}
}

func entryFor(path string, before, after workspace.FileMeta) JournalEntry {
	e := JournalEntry{
		Path:        path,
		BeforeHash:  before.Hash,
		AfterHash:   after.Hash,
		BeforeMode:  before.Mode,
		AfterMode:   after.Mode,
		AfterType:   after.Type,
		AfterTarget: after.Target,
		Status:      EntryPending,
	}
	switch {
	case before.Missing && !after.Missing:
		e.Op = OpCreate
	case !before.Missing && after.Missing:
		e.Op = OpDelete
		e.AfterHash = ""
	default:
		e.Op = OpModify
	}
	return e
}

func recheckSource(root string, entries []JournalEntry) error {
	for _, e := range entries {
		cur, err := hashAt(root, e.Path)
		if err != nil {
			return err
		}
		want := e.BeforeHash
		if e.Op == OpCreate {
			if !cur.Missing {
				return fmt.Errorf("%w: %s appeared before publication", ErrDiverged, e.Path)
			}
			continue
		}
		if cur.Missing {
			if want != "" {
				return fmt.Errorf("%w: %s disappeared before publication", ErrDiverged, e.Path)
			}
			continue
		}
		if cur.Hash != want {
			return fmt.Errorf("%w: %s changed before publication", ErrDiverged, e.Path)
		}
	}
	return nil
}

func hashAt(root, rel string) (workspace.FileMeta, error) {
	rel, err := workspace.SafeRel(rel)
	if err != nil {
		return workspace.FileMeta{}, err
	}
	p := filepath.Join(root, filepath.FromSlash(rel))
	fi, err := os.Lstat(p)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return workspace.FileMeta{Path: rel, Missing: true}, nil
		}
		return workspace.FileMeta{}, err
	}
	m := workspace.FileMeta{Path: rel}
	if fi.Mode()&os.ModeSymlink != 0 {
		t, err := os.Readlink(p)
		if err != nil {
			return workspace.FileMeta{}, err
		}
		m.Type = workspace.TypeSymlink
		m.Mode = workspace.ModeLink
		m.Target = t
		m.Hash = workspace.HashString(t)
		return m, nil
	}
	if !fi.Mode().IsRegular() {
		return workspace.FileMeta{}, fmt.Errorf("unsupported type at %s", rel)
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return workspace.FileMeta{}, err
	}
	m.Type = workspace.TypeFile
	if fi.Mode()&0o111 != 0 {
		m.Mode = workspace.ModeExec
	} else {
		m.Mode = workspace.ModeFile
	}
	m.Hash = workspace.HashBytes(data)
	m.Size = int64(len(data))
	return m, nil
}

func publish(ctx context.Context, sourceRoot string, journal *Journal, crashAfter int) ([]string, error) {
	var published []string
	verified := 0
	for i := range journal.Entries {
		if err := ctx.Err(); err != nil {
			_ = journal.Save()
			return published, err
		}
		e := &journal.Entries[i]
		if err := applyEntry(sourceRoot, journal, e); err != nil {
			e.Status = EntryFailed
			_ = journal.Save()
			return published, err
		}
		e.Status = EntryWritten
		if err := verifyEntry(sourceRoot, e); err != nil {
			e.Status = EntryFailed
			_ = journal.Save()
			return published, err
		}
		e.Status = EntryVerified
		if err := journal.Save(); err != nil {
			return published, err
		}
		published = append(published, e.Path)
		verified++
		if crashAfter > 0 && verified >= crashAfter {
			return published, fmt.Errorf("%w: crash after %d", ErrIncomplete, crashAfter)
		}
	}
	return published, nil
}

func applyEntry(sourceRoot string, journal *Journal, e *JournalEntry) error {
	rel, err := workspace.SafeRel(e.Path)
	if err != nil {
		return err
	}
	dst := filepath.Join(sourceRoot, filepath.FromSlash(rel))
	switch e.Op {
	case OpDelete:
		if err := os.Remove(dst); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		pruneEmpty(sourceRoot, rel)
		return nil
	case OpCreate, OpModify:
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		_ = os.Remove(dst)
		if e.AfterType == workspace.TypeSymlink {
			return os.Symlink(e.AfterTarget, dst)
		}
		return copyAtomic(journal.blobPath(e.AfterHash), dst, e.AfterMode)
	default:
		return fmt.Errorf("unknown op %s", e.Op)
	}
}

func verifyEntry(sourceRoot string, e *JournalEntry) error {
	cur, err := hashAt(sourceRoot, e.Path)
	if err != nil {
		return err
	}
	if e.Op == OpDelete {
		if !cur.Missing {
			return fmt.Errorf("verify delete %s: still present", e.Path)
		}
		return nil
	}
	if cur.Missing {
		return fmt.Errorf("verify %s: missing after write", e.Path)
	}
	if cur.Hash != e.AfterHash {
		return fmt.Errorf("verify %s: hash %s != %s", e.Path, cur.Hash, e.AfterHash)
	}
	return nil
}

func copyAtomic(src, dst, mode string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	perm := os.FileMode(0o644)
	if mode == workspace.ModeExec {
		perm = 0o755
	}
	tmp, err := os.CreateTemp(filepath.Dir(dst), ".wayshard-pub-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := workspace.ReplaceFile(tmpName, dst); err != nil {
		return err
	}
	return os.Chmod(dst, perm)
}

func workspaceCopyBlob(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp := dst + ".tmp"
	out, err := os.Create(tmp)
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
	return workspace.ReplaceFile(tmp, dst)
}

func pruneEmpty(root, rel string) {
	dir := filepath.Dir(filepath.Join(root, filepath.FromSlash(rel)))
	for {
		if dir == root || !strings.HasPrefix(dir, root) {
			return
		}
		if err := os.Remove(dir); err != nil {
			return
		}
		dir = filepath.Dir(dir)
	}
}

func (j *Journal) Save() error {
	if j.Dir == "" {
		return fmt.Errorf("journal dir is empty")
	}
	if err := os.MkdirAll(j.Dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(j, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(j.Dir, ".journal-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	return workspace.ReplaceFile(tmpName, j.Path())
}

func LoadJournal(path string) (*Journal, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var j Journal
	if err := json.Unmarshal(data, &j); err != nil {
		return nil, err
	}
	j.Dir = filepath.Dir(path)
	return &j, nil
}

// ClassifyJournal inspects source files against a publication journal.
func ClassifyJournal(ctx context.Context, sourceRoot string, j *Journal) (*Recovery, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	rec := &Recovery{Journal: j, Status: RecoveryCompleted}
	anyIncomplete := false
	anyDiverged := false
	for _, e := range j.Entries {
		cur, err := hashAt(sourceRoot, e.Path)
		if err != nil {
			return nil, err
		}
		fr := FileRecovery{Path: e.Path, CurrentHash: cur.Hash}
		switch classifyEntry(e, cur) {
		case RecoveryCompleted:
			fr.Status = RecoveryCompleted
		case RecoveryDiverged:
			fr.Status = RecoveryDiverged
			anyDiverged = true
		default:
			fr.Status = RecoveryIncomplete
			anyIncomplete = true
		}
		rec.Files = append(rec.Files, fr)
	}
	switch {
	case anyDiverged:
		rec.Status = RecoveryDiverged
	case anyIncomplete:
		rec.Status = RecoveryIncomplete
	default:
		rec.Status = RecoveryCompleted
	}
	return rec, nil
}

func classifyEntry(e JournalEntry, cur workspace.FileMeta) string {
	afterMissing := e.Op == OpDelete
	beforeMissing := e.Op == OpCreate
	if afterMissing {
		if cur.Missing {
			return RecoveryCompleted
		}
		if cur.Hash == e.BeforeHash {
			return RecoveryIncomplete
		}
		return RecoveryDiverged
	}
	if !cur.Missing && cur.Hash == e.AfterHash {
		return RecoveryCompleted
	}
	if beforeMissing {
		if cur.Missing {
			return RecoveryIncomplete
		}
		return RecoveryDiverged
	}
	if !cur.Missing && cur.Hash == e.BeforeHash {
		return RecoveryIncomplete
	}
	if cur.Missing && e.BeforeHash != "" {
		return RecoveryDiverged
	}
	return RecoveryDiverged
}

// RecoverPublication classifies a journal and, if incomplete without
// divergence, resumes remaining writes.
func RecoverPublication(ctx context.Context, sourceRoot string, j *Journal) (*Recovery, *Result, error) {
	rec, err := ClassifyJournal(ctx, sourceRoot, j)
	if err != nil {
		return nil, nil, err
	}
	if rec.Status != RecoveryIncomplete {
		return rec, nil, nil
	}
	identKey := j.IdentityKey
	if identKey == "" {
		identKey = workspace.KindFilesystem + ":" + sourceRoot
	}
	unlock := lockSource(identKey)
	defer unlock()

	res := &Result{Status: StatusPublishing, Journal: j, Record: domain.Integration{
		ID:     j.IntegrationID,
		RunID:  j.RunID,
		Status: StatusPublishing,
	}}
	var published []string
	for i := range j.Entries {
		e := &j.Entries[i]
		if e.Status == EntryVerified {
			published = append(published, e.Path)
			continue
		}
		cur, err := hashAt(sourceRoot, e.Path)
		if err != nil {
			return rec, res, err
		}
		if classifyEntry(*e, cur) == RecoveryCompleted {
			e.Status = EntryVerified
			published = append(published, e.Path)
			continue
		}
		if classifyEntry(*e, cur) == RecoveryDiverged {
			res.Status = StatusDiverged
			res.Reason = ReasonDiverged
			res.Record.Status = StatusDiverged
			_ = j.Save()
			rec.Status = RecoveryDiverged
			return rec, res, nil
		}
		if err := applyEntry(sourceRoot, j, e); err != nil {
			e.Status = EntryFailed
			_ = j.Save()
			res.Status = StatusIncomplete
			res.Record.Status = StatusIncomplete
			return rec, res, err
		}
		if err := verifyEntry(sourceRoot, e); err != nil {
			e.Status = EntryFailed
			_ = j.Save()
			res.Status = StatusIncomplete
			return rec, res, err
		}
		e.Status = EntryVerified
		if err := j.Save(); err != nil {
			return rec, res, err
		}
		published = append(published, e.Path)
	}
	res.Status = StatusPublished
	res.Published = published
	res.Record.Status = StatusPublished
	rec.Status = RecoveryCompleted
	return rec, res, nil
}

func maybeGitStage(ctx context.Context, req Request, journal *Journal) error {
	if req.Source.Kind() != workspace.KindGit {
		return nil
	}
	if req.AutoCommit && !req.AutoStage {
		// Optional auto-commit may include only the proved run delta.
		req.AutoStage = true
	}
	if !req.AutoStage {
		return nil
	}
	gb, ok := req.Source.(*workspace.GitWorkspaceBackend)
	if !ok {
		return nil
	}
	var add, rm []string
	for _, e := range journal.Entries {
		if e.Op == OpDelete {
			rm = append(rm, e.Path)
			continue
		}
		add = append(add, e.Path)
	}
	if len(rm) > 0 {
		if err := gb.Repo().RmCached(ctx, rm...); err != nil {
			return err
		}
	}
	if len(add) > 0 {
		if err := gb.Repo().Add(ctx, add...); err != nil {
			return err
		}
	}
	if req.AutoCommit {
		return fmt.Errorf("auto-commit is not enabled by default and requires an explicit commit helper")
	}
	return nil
}
