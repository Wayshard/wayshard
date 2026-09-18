//go:build linux

package sandbox

import (
	"context"
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
	if p.SyntheticHome != "" || p.SyntheticTemp != "" {
		c.Features = append(c.Features, "synthetic_home")
	}
	if namespacesAvailable() {
		c.Features = append(c.Features, "namespaces")
	} else {
		c.Unavailable = append(c.Unavailable, "namespaces")
		if p.Required {
			return c, fmt.Errorf("%w: linux user namespaces unavailable", ErrRequiredIsolation)
		}
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

func (LinuxBackend) Constrain(cmd *exec.Cmd, p Policy) error {
	if _, err := (LinuxBackend{}).Compile(p); err != nil {
		return err
	}
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
	cmd.SysProcAttr.Pdeathsig = syscall.SIGKILL
	if namespacesAvailable() {
		cmd.SysProcAttr.Cloneflags = syscall.CLONE_NEWUSER | syscall.CLONE_NEWNET | syscall.CLONE_NEWNS | syscall.CLONE_NEWPID | syscall.CLONE_NEWUTS
		uid := os.Getuid()
		gid := os.Getgid()
		cmd.SysProcAttr.UidMappings = []syscall.SysProcIDMap{{ContainerID: 0, HostID: uid, Size: 1}}
		cmd.SysProcAttr.GidMappings = []syscall.SysProcIDMap{{ContainerID: 0, HostID: gid, Size: 1}}
		cmd.SysProcAttr.GidMappingsEnableSetgroups = false
	}
	cmd.Env = filterEnv(p, cmd.Env)
	if p.SyntheticHome != "" {
		_ = os.MkdirAll(p.SyntheticHome, 0o700)
		cmd.Env = append(cmd.Env, "HOME="+p.SyntheticHome)
	}
	if p.SyntheticTemp != "" {
		_ = os.MkdirAll(p.SyntheticTemp, 0o700)
		cmd.Env = append(cmd.Env, "TMPDIR="+p.SyntheticTemp, "TEMP="+p.SyntheticTemp)
	}
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
	r := IsolationReport{Backend: "linux", Available: true, Mode: "process_group"}
	if namespacesAvailable() {
		r.Mode = "namespaces"
		r.Features = []string{"namespaces", "process_group", "pdeathsig", "env_filter"}
		r.Detail = "linux user/net/mount/pid/uts namespaces + process group + pdeathsig"
		return r
	}
	r.Missing = []string{"namespaces"}
	r.Available = false
	r.Mode = "unavailable"
	r.Detail = "linux user namespaces unavailable; refusing silent unrestricted execution"
	return r
}

func namespacesAvailable() bool {
	f, err := os.Open("/proc/self/ns/user")
	if err != nil {
		return false
	}
	_ = f.Close()
	b, err := os.ReadFile("/proc/sys/user/max_user_namespaces")
	if err != nil {
		return true
	}
	return string(b) != "0\n" && string(b) != "0"
}
