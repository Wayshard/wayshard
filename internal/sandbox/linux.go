//go:build linux

package sandbox

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

func (LinuxBackend) Compile(p Policy) (Compiled, error) {
	if err := compileCommon(p); err != nil {
		return Compiled{Backend: "linux"}, err
	}
	c := Compiled{Backend: "linux", Features: []string{"process_group", "pdeathsig", "env_filter"}}
	if _, err := landlockABI(); err != nil {
		c.Unavailable = append(c.Unavailable, "landlock")
		if p.Required {
			return c, fmt.Errorf("%w: landlock unavailable: %v", ErrRequiredIsolation, err)
		}
	} else {
		c.Features = append(c.Features, "landlock")
	}
	if p.Network == NetNone {
		abi, err := landlockABI()
		if err != nil || abi < 4 {
			c.Unavailable = append(c.Unavailable, "network_deny")
			if p.Required {
				return c, fmt.Errorf("%w: network denial requires landlock ABI>=4", ErrRequiredIsolation)
			}
		} else {
			c.Features = append(c.Features, "network_deny")
		}
	}
	if p.Network == NetAllowlist || p.Network == NetBrokered {
		c.Unavailable = append(c.Unavailable, "network_allowlist")
		if p.Required {
			return c, fmt.Errorf("%w: network %q is not implemented; refusing to pretend support", ErrRequiredIsolation, p.Network)
		}
	}
	if p.SyntheticHome != "" || p.SyntheticTemp != "" {
		c.Features = append(c.Features, "synthetic_home")
	}
	return c, nil
}

func (b LinuxBackend) Apply(ctx context.Context, p Policy) (Cleanup, error) {
	_ = ctx
	if _, err := b.Compile(p); err != nil {
		return nil, err
	}
	return func() {}, nil
}

// Constrain wraps the target command with the in-binary sandbox helper so the
// filesystem/network policy is applied to the spawned process and inherited by
// all of its descendants.
func (b LinuxBackend) Constrain(cmd *exec.Cmd, p Policy) error {
	if _, err := b.Compile(p); err != nil {
		return err
	}
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
	cmd.SysProcAttr.Pdeathsig = syscall.SIGKILL

	helper := os.Getenv("WAYSHARD_SANDBOX_HELPER")
	if helper == "" {
		if exe, err := os.Executable(); err == nil {
			helper = exe
		}
	}
	if helper == "" {
		if p.Required {
			return fmt.Errorf("%w: sandbox helper binary unavailable", ErrRequiredIsolation)
		}
		return nil
	}

	dir := p.SyntheticTemp
	if dir == "" {
		dir = os.TempDir()
	}
	_ = os.MkdirAll(dir, 0o700)
	f, err := os.CreateTemp(dir, "wayshard-policy-*.json")
	if err != nil {
		return fmt.Errorf("sandbox policy file: %w", err)
	}
	payload, err := json.Marshal(p)
	if err != nil {
		_ = f.Close()
		_ = os.Remove(f.Name())
		return err
	}
	if _, err := f.Write(payload); err != nil {
		_ = f.Close()
		_ = os.Remove(f.Name())
		return err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(f.Name())
		return err
	}

	origPath := cmd.Path
	origArgs := cmd.Args
	cmd.Path = helper
	cmd.Args = append([]string{helper, HelperArg, f.Name(), "--", origPath}, origArgs[1:]...)
	return nil
}

func (LinuxBackend) Attach(cmd *exec.Cmd, p Policy) (Cleanup, error) {
	_ = cmd
	_ = p
	return func() {}, nil
}

func (LinuxBackend) KillTree(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
}

func (LinuxBackend) Report() IsolationReport {
	r := IsolationReport{Backend: "linux", Available: true, Mode: "landlock"}
	if _, err := landlockABI(); err != nil {
		r.Missing = []string{"landlock"}
		r.Available = false
		r.Mode = "unavailable"
		r.Detail = "linux landlock unavailable; refusing silent unrestricted execution"
		return r
	}
	r.Features = []string{"landlock_fs", "process_group", "pdeathsig", "env_allowlist"}
	abi, _ := landlockABI()
	if abi >= 4 {
		r.Features = append(r.Features, "landlock_net")
	}
	r.Detail = "linux landlock filesystem confinement + process group + pdeathsig + env allowlist"
	return r
}
