//go:build linux

package sandbox

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"syscall"

	"golang.org/x/sys/unix"
)

// setupProcIsolation mounts a procfs scoped to this process's PID namespace.
// It must run after the helper was created with CLONE_NEWPID|CLONE_NEWNS (and a
// user namespace when unprivileged), and before Landlock/seccomp.
func setupProcIsolation() error {
	if err := unix.Mount("", "/", "", unix.MS_REC|unix.MS_PRIVATE, ""); err != nil {
		return fmt.Errorf("make mount propagation private: %w", err)
	}
	if err := unix.Mount("proc", "/proc", "proc", unix.MS_NOSUID|unix.MS_NODEV|unix.MS_NOEXEC, ""); err != nil {
		return fmt.Errorf("mount scoped procfs: %w", err)
	}
	return nil
}

// procProbeExit mounts a scoped procfs in the current (already cloned) PID and
// mount namespace. Used as the capability probe body.
func procProbeExit() int {
	if err := setupProcIsolation(); err != nil {
		return 1
	}
	return 0
}

// loopbackProbeExit raises loopback in the current (already cloned) network
// namespace. Used as the capability probe body.
func loopbackProbeExit() int {
	if err := BringUpLoopback(); err != nil {
		return 1
	}
	return 0
}

var (
	loopOnce sync.Once
	loopOK   bool
)

// loopbackProbeSupported probes once whether this platform can create a private
// network namespace with loopback. When it cannot, the ACP discovery probe
// falls back to NetworkNone (a local-socket harness then reports incompatible
// honestly).
func loopbackProbeSupported() bool {
	loopOnce.Do(func() {
		exe, err := os.Executable()
		if err != nil {
			return
		}
		attr := &syscall.SysProcAttr{Cloneflags: unix.CLONE_NEWNET}
		if os.Geteuid() != 0 {
			attr.Cloneflags |= unix.CLONE_NEWUSER
			attr.UidMappings = []syscall.SysProcIDMap{{ContainerID: 0, HostID: os.Geteuid(), Size: 1}}
			attr.GidMappings = []syscall.SysProcIDMap{{ContainerID: 0, HostID: os.Getegid(), Size: 1}}
			attr.GidMappingsEnableSetgroups = false
		}
		cmd := exec.Command(exe, LoopbackProbeArg)
		cmd.SysProcAttr = attr
		loopOK = cmd.Run() == nil
	})
	return loopOK
}

// LoopbackProbeAvailable reports whether an isolated loopback network namespace
// can be created for the ACP discovery probe.
func LoopbackProbeAvailable() bool { return loopbackProbeSupported() }

var (
	procOnce sync.Once
	procOK   bool
)

// procIsolationSupported probes once whether this platform can mount a procfs
// scoped to a private PID namespace. When it cannot, ProcIsolation is skipped
// and /proc is not granted, rather than exposing the host procfs or failing
// every harness/probe launch.
func procIsolationSupported() bool {
	procOnce.Do(func() {
		exe, err := os.Executable()
		if err != nil {
			return
		}
		attr := &syscall.SysProcAttr{Cloneflags: unix.CLONE_NEWPID | unix.CLONE_NEWNS}
		if os.Geteuid() != 0 {
			attr.Cloneflags |= unix.CLONE_NEWUSER
			attr.UidMappings = []syscall.SysProcIDMap{{ContainerID: 0, HostID: os.Geteuid(), Size: 1}}
			attr.GidMappings = []syscall.SysProcIDMap{{ContainerID: 0, HostID: os.Getegid(), Size: 1}}
			attr.GidMappingsEnableSetgroups = false
		}
		cmd := exec.Command(exe, ProcProbeArg)
		cmd.SysProcAttr = attr
		procOK = cmd.Run() == nil
	})
	return procOK
}

func (LinuxBackend) Compile(p Policy) (Compiled, error) {
	if err := compileCommon(p, true); err != nil {
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
		// NetworkNone is enforced by seccomp-BPF at exec time (Landlock alone
		// cannot mediate UDP or AF_UNIX).
		if _, err := seccompAuditArch(); err != nil {
			c.Unavailable = append(c.Unavailable, "network_deny")
			if p.Required {
				return c, fmt.Errorf("%w: %v", ErrRequiredIsolation, err)
			}
		} else {
			c.Features = append(c.Features, "seccomp_network_deny")
		}
	}
	if p.Network == NetProvider {
		// Provider mode allows TCP to the in-namespace broker only. The address
		// isolation comes from the network namespace; seccomp enforces the
		// domain/type restriction (no UDP, no AF_UNIX/AF_NETLINK/AF_PACKET).
		if _, err := seccompAuditArch(); err != nil {
			c.Unavailable = append(c.Unavailable, "network_provider")
			if p.Required {
				return c, fmt.Errorf("%w: %v", ErrRequiredIsolation, err)
			}
		} else {
			c.Features = append(c.Features, "seccomp_provider_tcp")
		}
	}
	if p.Network == NetLoopback {
		// Loopback mode is a private network namespace with only `lo`. seccomp
		// permits TCP to that isolated loopback and denies everything else.
		if _, err := seccompAuditArch(); err != nil {
			c.Unavailable = append(c.Unavailable, "network_loopback")
			if p.Required {
				return c, fmt.Errorf("%w: %v", ErrRequiredIsolation, err)
			}
		} else {
			c.Features = append(c.Features, "seccomp_loopback_tcp")
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
	needUser := false
	if p.ProcIsolation && procIsolationSupported() {
		// A private PID + mount namespace lets the helper mount a procfs scoped
		// to this process and its descendants, so /proc never exposes host
		// processes.
		cmd.SysProcAttr.Cloneflags |= unix.CLONE_NEWPID | unix.CLONE_NEWNS
		p.ProcNamespaced = true
		needUser = true
	}
	if p.Network == NetLoopback {
		// A private network namespace with only loopback, so a harness that
		// needs local IPC has loopback but no host/LAN/public route.
		if !loopbackProbeSupported() {
			return fmt.Errorf("%w: loopback isolation unavailable", ErrRequiredIsolation)
		}
		cmd.SysProcAttr.Cloneflags |= unix.CLONE_NEWNET
		p.LoopbackNamespaced = true
		needUser = true
	}
	if needUser && os.Geteuid() != 0 {
		cmd.SysProcAttr.Cloneflags |= unix.CLONE_NEWUSER
		cmd.SysProcAttr.UidMappings = []syscall.SysProcIDMap{{ContainerID: 0, HostID: os.Geteuid(), Size: 1}}
		cmd.SysProcAttr.GidMappings = []syscall.SysProcIDMap{{ContainerID: 0, HostID: os.Getegid(), Size: 1}}
		cmd.SysProcAttr.GidMappingsEnableSetgroups = false
	}

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
	if _, err := seccompAuditArch(); err == nil {
		r.Features = append(r.Features, "seccomp_network_deny")
	}
	// Secure provider-only networking is not implemented; required-isolation
	// harnesses run with no network instead.
	r.Missing = append(r.Missing, "provider_network")
	r.Detail = "linux landlock filesystem confinement + seccomp network confinement (none) + process group + pdeathsig + env allowlist; secure provider network unavailable"
	return r
}
