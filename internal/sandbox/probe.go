package sandbox

import (
	"context"
	"os/exec"
	"path/filepath"
	"time"
)

// ProbePolicy confines harness discovery, version and ACP-initialize probes.
//
// It grants no project or SourceWorkspace access, no Wayshard runtime/database/
// vault access, no SSH agent, no D-Bus/display sockets and no network. Reads are
// limited to the system runtime roots needed to launch a binary plus the
// executable's own directory; a synthetic HOME/TEMP is provided so a probe
// cannot read the real user home. It is Required, so a platform that cannot
// enforce it reports the probe unavailable rather than running unrestricted.
func ProbePolicy(exePath, syntheticHome, syntheticTemp string) Policy {
	roots := systemReadOnlyRoots()
	// Grant read/execute on the executable's own directory. Resolve symlinks so
	// a symlinked launcher still exposes its real target (Landlock checks the
	// resolved inode).
	addExeRoot := func(p string) {
		if p == "" {
			return
		}
		if rp, err := filepath.EvalSymlinks(p); err == nil {
			p = rp
		}
		if d := filepath.Dir(p); d != "" && d != "." {
			roots = append(roots, d)
		}
	}
	addExeRoot(exePath)
	return Policy{
		ReadOnlyRoots:  roots,
		ReadWriteRoots: append([]string{syntheticHome, syntheticTemp}, systemDeviceRoots()...),
		SyntheticHome:  syntheticHome,
		SyntheticTemp:  syntheticTemp,
		Network:        NetNone,
		MemoryBytes:    512 << 20,
		WallTimeSec:    30,
		MaxProcesses:   64,
		MaxOutputBytes: 1 << 20,
		Required:       true,
	}
}

// boundedBuffer captures at most limit bytes and discards the rest.
type boundedBuffer struct {
	buf   []byte
	limit int
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	if b.limit <= 0 {
		b.limit = 1 << 20
	}
	if len(b.buf) < b.limit {
		remain := b.limit - len(b.buf)
		if len(p) > remain {
			b.buf = append(b.buf, p[:remain]...)
		} else {
			b.buf = append(b.buf, p...)
		}
	}
	return len(p), nil
}

func (b *boundedBuffer) Bytes() []byte { return b.buf }

// RunConstrainedOutput runs name under policy p with a bounded combined output
// and a timeout, killing the whole owned process tree on timeout or
// cancellation. It returns the captured output even when the command fails.
func RunConstrainedOutput(ctx context.Context, p Policy, timeout time.Duration, maxOutput int, name string, args, env []string) ([]byte, error) {
	return RunConstrainedOutputWithStart(ctx, p, timeout, maxOutput, name, args, env, nil)
}

// RunConstrainedOutputWithStart is RunConstrainedOutput with an onStart hook that
// receives the started process group id (the process is a group leader because
// the sandbox constrainer sets Setpgid). It is used to record durable probe
// ownership once the probe has actually started.
func RunConstrainedOutputWithStart(ctx context.Context, p Policy, timeout time.Duration, maxOutput int, name string, args, env []string, onStart func(pgid int)) ([]byte, error) {
	c := AsConstrainer(DefaultBackend())
	if _, err := c.Compile(p); err != nil {
		return nil, err
	}
	if timeout <= 0 {
		timeout = 8 * time.Second
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(cctx, name, args...)
	cmd.Env = env
	cmd.Stdin = nil
	buf := &boundedBuffer{limit: maxOutput}
	cmd.Stdout = buf
	cmd.Stderr = buf
	if err := c.Constrain(cmd, p); err != nil {
		return buf.Bytes(), err
	}
	cmd.Cancel = func() error { return c.KillTree(cmd) }
	cmd.WaitDelay = 2 * time.Second
	if err := cmd.Start(); err != nil {
		return buf.Bytes(), err
	}
	if onStart != nil && cmd.Process != nil {
		onStart(cmd.Process.Pid)
	}
	err := cmd.Wait()
	return buf.Bytes(), err
}
