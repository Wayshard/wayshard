//go:build linux

package sandbox

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

// Constrain applies Linux isolation to cmd. If Required isolation cannot be
// established, it returns ErrRequiredIsolation and does not launch unrestricted.
func (LinuxBackend) Constrain(cmd *exec.Cmd, p Policy) error {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
	cmd.SysProcAttr.Pdeathsig = syscall.SIGKILL

	applied := false
	if namespacesAvailable() {
		cmd.SysProcAttr.Cloneflags = syscall.CLONE_NEWUSER | syscall.CLONE_NEWNET | syscall.CLONE_NEWNS | syscall.CLONE_NEWPID | syscall.CLONE_NEWUTS
		applied = true
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
	if p.Required && !applied {
		return fmt.Errorf("%w: linux user namespaces unavailable", ErrRequiredIsolation)
	}
	return nil
}

func (b LinuxBackend) Apply(ctx context.Context, p Policy) (Cleanup, error) {
	_ = ctx
	if p.Required && len(p.ReadWriteRoots) == 0 {
		return nil, fmt.Errorf("%w: no writable roots in policy", ErrRequiredIsolation)
	}
	if p.Required && !namespacesAvailable() {
		return nil, fmt.Errorf("%w: linux user namespaces unavailable", ErrRequiredIsolation)
	}
	return func() {}, nil
}

func namespacesAvailable() bool {
	f, err := os.Open("/proc/self/ns/user")
	if err != nil {
		return false
	}
	_ = f.Close()
	// Probe unprivileged user ns by checking max_user_namespaces
	b, err := os.ReadFile("/proc/sys/user/max_user_namespaces")
	if err != nil {
		return true // assume available if the knob is absent
	}
	return string(b) != "0\n" && string(b) != "0"
}

func filterEnv(p Policy, base []string) []string {
	if len(p.EnvAllow) == 0 {
		out := make([]string, 0, len(base))
		deny := map[string]struct{}{"TYPESAFE_API_KEY": {}, "WAYSHARD_VAULT_KEY": {}, "WAYSHARD_VAULT_PASSPHRASE": {}}
		for _, e := range base {
			k := e
			if i := indexByte(e, '='); i >= 0 {
				k = e[:i]
			}
			if _, bad := deny[k]; bad {
				continue
			}
			out = append(out, e)
		}
		return out
	}
	allow := map[string]struct{}{}
	for _, k := range p.EnvAllow {
		allow[k] = struct{}{}
	}
	var out []string
	for _, e := range base {
		k := e
		if i := indexByte(e, '='); i >= 0 {
			k = e[:i]
		}
		if _, ok := allow[k]; ok {
			out = append(out, e)
		}
	}
	return out
}

func indexByte(s string, c byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			return i
		}
	}
	return -1
}

// RunConstrained starts cmd under policy. Used by tool sandbox / tests.
func RunConstrained(ctx context.Context, p Policy, name string, args ...string) (*exec.Cmd, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	b := DefaultBackend()
	if lb, ok := b.(LinuxBackend); ok {
		if err := lb.Constrain(cmd, p); err != nil {
			return nil, err
		}
	} else if p.Required && !b.Available() {
		return nil, fmt.Errorf("%w: backend %s unavailable", ErrRequiredIsolation, b.Name())
	}
	return cmd, nil
}
