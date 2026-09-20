//go:build !darwin

package sandbox

import (
	"context"
	"fmt"
	"os/exec"
)

func (DarwinBackend) Compile(p Policy) (Compiled, error) {
	c := Compiled{Backend: "darwin", Unavailable: []string{"seatbelt", "process_group"}}
	if err := compileCommon(p, false); err != nil {
		return c, err
	}
	if p.Required {
		return c, fmt.Errorf("%w: darwin backend not compiled on this OS", ErrRequiredIsolation)
	}
	return c, fmt.Errorf("%w: darwin backend not compiled on this OS", ErrRequiredIsolation)
}

func (DarwinBackend) Apply(ctx context.Context, p Policy) (Cleanup, error) {
	_ = ctx
	_, err := (DarwinBackend{}).Compile(p)
	return nil, err
}

func (DarwinBackend) Constrain(cmd *exec.Cmd, p Policy) error {
	_ = cmd
	_, err := (DarwinBackend{}).Compile(p)
	return err
}

func (DarwinBackend) Attach(*exec.Cmd, Policy) (Cleanup, error) {
	return func() {}, fmt.Errorf("%w: darwin backend not compiled on this OS", ErrRequiredIsolation)
}

func (DarwinBackend) KillTree(*exec.Cmd) error { return nil }

func (DarwinBackend) Report() IsolationReport {
	return IsolationReport{Backend: "darwin", Available: false, Mode: "unavailable", Missing: []string{"seatbelt"}, Detail: "darwin backend not compiled on this OS"}
}
