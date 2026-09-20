//go:build !linux

package sandbox

import (
	"context"
	"fmt"
	"os/exec"
)

func (LinuxBackend) Compile(p Policy) (Compiled, error) {
	c := Compiled{Backend: "linux", Unavailable: []string{"namespaces", "process_group"}}
	if err := compileCommon(p, false); err != nil {
		return c, err
	}
	return c, fmt.Errorf("%w: linux backend not compiled on this OS", ErrRequiredIsolation)
}

func (LinuxBackend) Apply(ctx context.Context, p Policy) (Cleanup, error) {
	_ = ctx
	_, err := (LinuxBackend{}).Compile(p)
	return nil, err
}

func (LinuxBackend) Constrain(cmd *exec.Cmd, p Policy) error {
	_ = cmd
	_, err := (LinuxBackend{}).Compile(p)
	return err
}

func (LinuxBackend) Attach(*exec.Cmd, Policy) (Cleanup, error) {
	return func() {}, fmt.Errorf("%w: linux backend not compiled on this OS", ErrRequiredIsolation)
}

func (LinuxBackend) KillTree(*exec.Cmd) error { return nil }

func (LinuxBackend) Report() IsolationReport {
	return IsolationReport{Backend: "linux", Available: false, Mode: "unavailable", Missing: []string{"namespaces"}, Detail: "linux backend not compiled on this OS"}
}
