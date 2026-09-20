//go:build !windows

package sandbox

import (
	"context"
	"fmt"
	"os/exec"
)

func (WindowsBackend) Compile(p Policy) (Compiled, error) {
	c := Compiled{Backend: "windows", Unavailable: []string{"job_object", "appcontainer"}}
	if err := compileCommon(p, false); err != nil {
		return c, err
	}
	return c, fmt.Errorf("%w: windows backend not compiled on this OS", ErrRequiredIsolation)
}

func (WindowsBackend) Apply(ctx context.Context, p Policy) (Cleanup, error) {
	_ = ctx
	_, err := (WindowsBackend{}).Compile(p)
	return nil, err
}

func (WindowsBackend) Constrain(cmd *exec.Cmd, p Policy) error {
	_ = cmd
	_, err := (WindowsBackend{}).Compile(p)
	return err
}

func (WindowsBackend) Attach(*exec.Cmd, Policy) (Cleanup, error) {
	return func() {}, fmt.Errorf("%w: windows backend not compiled on this OS", ErrRequiredIsolation)
}

func (WindowsBackend) KillTree(*exec.Cmd) error { return nil }

func (WindowsBackend) Report() IsolationReport {
	return IsolationReport{Backend: "windows", Available: false, Mode: "unavailable", Missing: []string{"job_object"}, Detail: "windows backend not compiled on this OS"}
}
