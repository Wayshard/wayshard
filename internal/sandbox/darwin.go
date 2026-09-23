//go:build darwin

package sandbox

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

func sandboxExecPath() string {
	for _, p := range []string{"/usr/bin/sandbox-exec", "/usr/bin/sandbox-exec5"} {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	if p, err := exec.LookPath("sandbox-exec"); err == nil {
		return p
	}
	return ""
}

func (DarwinBackend) Compile(p Policy) (Compiled, error) {
	if err := compileCommon(p, false); err != nil {
		return Compiled{Backend: "darwin"}, err
	}
	profile, err := SeatbeltProfile(p)
	if err != nil {
		return Compiled{Backend: "darwin"}, err
	}
	// FeatureProcessTree is NOT advertised on macOS. Seatbelt confines the
	// process, but there is no non-removable OS-backed ownership boundary: the
	// untrusted process controls its children's environment, so an environment
	// token can be stripped, and a process group is escaped by setsid. A required
	// policy therefore fails closed before untrusted code runs.
	c := Compiled{Backend: "darwin", Features: []string{
		"process_group", "env_filter",
	}, Profile: profile}
	c.Unavailable = append(c.Unavailable, string(FeatureProcessTree))
	if p.SyntheticHome != "" || p.SyntheticTemp != "" {
		c.Features = append(c.Features, string(FeatureSyntheticEnv))
	}
	if sandboxExecPath() != "" {
		c.Features = append(c.Features, "seatbelt", string(FeatureFSRead), string(FeatureFSWrite), string(FeatureNetworkNone))
	} else {
		c.Unavailable = append(c.Unavailable, "seatbelt", string(FeatureFSRead), string(FeatureFSWrite), string(FeatureNetworkNone))
		if p.Required {
			return c, fmt.Errorf("%w: macOS sandbox-exec unavailable", ErrRequiredIsolation)
		}
	}
	if err := validateRequiredFeatures(c, p); err != nil {
		return c, err
	}
	return c, nil
}

func (b DarwinBackend) Apply(ctx context.Context, p Policy) (Cleanup, error) {
	_ = ctx
	if _, err := b.Compile(p); err != nil {
		return nil, err
	}
	return func() {}, nil
}

func (DarwinBackend) Constrain(cmd *exec.Cmd, p Policy) error {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
	cmd.Env = filterEnv(p, cmd.Env)
	if p.SyntheticHome != "" {
		_ = os.MkdirAll(p.SyntheticHome, 0o700)
		cmd.Env = append(cmd.Env, "HOME="+p.SyntheticHome)
	}
	if p.SyntheticTemp != "" {
		_ = os.MkdirAll(p.SyntheticTemp, 0o700)
		cmd.Env = append(cmd.Env, "TMPDIR="+p.SyntheticTemp)
	}
	exe := sandboxExecPath()
	if exe == "" {
		if p.Required {
			return fmt.Errorf("%w: macOS sandbox-exec unavailable", ErrRequiredIsolation)
		}
		return nil
	}
	orig := cmd.Path
	if orig == "" {
		orig = cmd.Args[0]
	}
	if !filepath.IsAbs(orig) {
		// Resolve relative to the child's working directory when set; otherwise
		// fall back to PATH. Resolving against the server's own CWD would point
		// sandbox-exec at the wrong file when the caller sets cmd.Dir.
		if cmd.Dir != "" {
			orig = filepath.Join(cmd.Dir, orig)
		} else if lp, err := exec.LookPath(orig); err == nil {
			orig = lp
		}
	}
	if orig != "" {
		if abs, err := filepath.Abs(orig); err == nil {
			orig = abs
		}
		if rp, err := filepath.EvalSymlinks(orig); err == nil {
			orig = rp
		}
		p.ReadOnlyRoots = append(append([]string{}, p.ReadOnlyRoots...), filepath.Dir(orig))
	}
	compiled, err := (DarwinBackend{}).Compile(p)
	if err != nil {
		return err
	}
	rest := []string{}
	if len(cmd.Args) > 1 {
		rest = cmd.Args[1:]
	}
	// Pass the profile inline with -p so no on-disk profile exists for the
	// constrained process to read, modify, or leave behind. The profile is
	// established by sandbox-exec before it execs the target, so confinement is
	// in place before any untrusted code runs.
	cmd.Path = exe
	cmd.Args = append([]string{"sandbox-exec", "-p", compiled.Profile, orig}, rest...)
	return nil
}

func (DarwinBackend) Attach(cmd *exec.Cmd, p Policy) (Cleanup, error) {
	_ = cmd
	_ = p
	return func() {}, nil
}

func (DarwinBackend) KillTree(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
}

func (DarwinBackend) Report() IsolationReport {
	// macOS cannot establish a non-removable process-tree ownership boundary.
	// Seatbelt filesystem/network confinement works, but a process group is
	// escaped by setsid and an environment token can be stripped by the untrusted
	// process, so FeatureProcessTree is not advertised and required local
	// execution fails closed. macOS remains a full server/client platform.
	features := []string{
		"seatbelt", "process_group", "env_filter",
		string(FeatureFSRead), string(FeatureFSWrite), string(FeatureNetworkNone), string(FeatureSyntheticEnv),
	}
	missing := []string{string(FeatureProcessTree), string(FeatureResourceLimits)}
	if sandboxExecPath() == "" {
		return IsolationReport{
			Backend:   "darwin",
			Available: false,
			Mode:      "unavailable",
			Features:  features,
			Missing:   append(missing, "seatbelt"),
			Detail:    "sandbox-exec not found and no non-removable process-tree ownership; required local execution fails closed",
		}
	}
	return IsolationReport{
		Backend:   "darwin",
		Available: false,
		Mode:      "seatbelt_no_ownership",
		Features:  features,
		Missing:   missing,
		Detail:    "macOS Seatbelt confinement is available but there is no non-removable OS-backed process-tree ownership boundary (an untrusted process can strip the ownership marker and setsid), so required local harness/tool/probe/validation execution fails closed",
	}
}
