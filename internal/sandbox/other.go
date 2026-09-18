//go:build !linux

package sandbox

import (
	"context"
	"fmt"
	"os/exec"
)

func (LinuxBackend) Apply(ctx context.Context, p Policy) (Cleanup, error) {
	_ = ctx
	return nil, fmt.Errorf("%w: linux backend not compiled on this OS", ErrRequiredIsolation)
}

func (DarwinBackend) Constrain(cmd *exec.Cmd, p Policy) error {
	if p.Required && !seatbeltAvailable() {
		return fmt.Errorf("%w: macOS sandbox_init/seatbelt unavailable", ErrRequiredIsolation)
	}
	cmd.Env = append(cmd.Env, "HOME="+p.SyntheticHome, "TMPDIR="+p.SyntheticTemp)
	return nil
}

func (WindowsBackend) Constrain(cmd *exec.Cmd, p Policy) error {
	if p.Required && !jobObjectsAvailable() {
		return fmt.Errorf("%w: Windows job-object sandbox unavailable", ErrRequiredIsolation)
	}
	return nil
}

func seatbeltAvailable() bool   { return false }
func jobObjectsAvailable() bool { return true }

func RunConstrained(ctx context.Context, p Policy, name string, args ...string) (*exec.Cmd, error) {
	b := DefaultBackend()
	if p.Required && !b.Available() {
		return nil, fmt.Errorf("%w: backend %s unavailable", ErrRequiredIsolation, b.Name())
	}
	cmd := exec.CommandContext(ctx, name, args...)
	return cmd, nil
}
