package workspace

import (
	"context"
	"fmt"
	"slices"
	"strings"
)

const (
	ChangeAdded    = "added"
	ChangeModified = "modified"
	ChangeDeleted  = "deleted"
)

// FileDelta is one path in snapshot -> run-final. PreExisting marks files that
// already differed from HEAD at intake (user baseline). AgentModified marks
// snapshot -> final divergence.
type FileDelta struct {
	Path          string   `json:"path"`
	Kind          string   `json:"kind"`
	AgentModified bool     `json:"agentModified"`
	PreExisting   bool     `json:"preExisting"`
	Before        FileMeta `json:"before"`
	After         FileMeta `json:"after"`
}

type Delta struct {
	Files []FileDelta `json:"files"`
}

func (d *Delta) AgentFiles() []FileDelta {
	var out []FileDelta
	for _, f := range d.Files {
		if f.AgentModified {
			out = append(out, f)
		}
	}
	return out
}

// ComputeDelta diffs the immutable starting snapshot against a run-final tree.
// Pre-existing dirty files that the agent did not touch are omitted from the
// agent delta and recorded only as PreExisting when they also appear here.
func ComputeDelta(snap *Snapshot, finalDir string) (*Delta, error) {
	if snap == nil {
		return nil, fmt.Errorf("snapshot is required")
	}
	b, err := Open(finalDir)
	if err != nil {
		return nil, err
	}
	st, err := b.CurrentState(context.Background())
	if err != nil {
		return nil, err
	}
	final := st.Files
	dirty := snap.DirtySet()
	paths := unionPaths(snap.Files, final)
	slices.Sort(paths)
	var files []FileDelta
	for _, p := range paths {
		before, bok := snap.Files[p]
		if !bok {
			before = missingMeta(p)
		}
		after, aok := final[p]
		if !aok {
			after = missingMeta(p)
		}
		if before.Equal(after) {
			continue
		}
		kind := ChangeModified
		switch {
		case before.Missing && !after.Missing:
			kind = ChangeAdded
		case !before.Missing && after.Missing:
			kind = ChangeDeleted
		}
		_, pre := dirty[p]
		files = append(files, FileDelta{
			Path:          p,
			Kind:          kind,
			AgentModified: true,
			PreExisting:   pre,
			Before:        before,
			After:         after,
		})
	}
	return &Delta{Files: files}, nil
}

// ClassifyPath reports whether path is agent-modified and/or pre-existing
// relative to snap and an already-computed delta.
func ClassifyPath(snap *Snapshot, delta *Delta, path string) (agent, pre bool) {
	if snap != nil {
		_, pre = snap.DirtySet()[path]
	}
	if delta != nil {
		for _, f := range delta.Files {
			if f.Path == path {
				return f.AgentModified, pre || f.PreExisting
			}
		}
	}
	return false, pre
}

func (d *Delta) String() string {
	if d == nil {
		return ""
	}
	var b strings.Builder
	for _, f := range d.Files {
		fmt.Fprintf(&b, "%s %s agent=%t pre=%t\n", f.Kind, f.Path, f.AgentModified, f.PreExisting)
	}
	return b.String()
}
