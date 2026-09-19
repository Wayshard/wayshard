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
	if err := compileCommon(p); err != nil {
		return Compiled{Backend: "darwin"}, err
	}
	profile, err := SeatbeltProfile(p)
	if err != nil {
		return Compiled{Backend: "darwin"}, err
	}
	c := Compiled{Backend: "darwin", Features: []string{"process_group", "env_filter"}, Profile: profile}
	if sandboxExecPath() != "" {
		c.Features = append(c.Features, "seatbelt")
	} else {
		c.Unavailable = append(c.Unavailable, "seatbelt")
		if p.Required {
			return c, fmt.Errorf("%w: macOS sandbox-exec unavailable", ErrRequiredIsolation)
		}
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
		if lp, err := exec.LookPath(orig); err == nil {
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
	dir, err := os.MkdirTemp("", "wayshard-seatbelt-*")
	if err != nil {
		return err
	}
	profilePath := filepath.Join(dir, "profile.sb")
	if err := os.WriteFile(profilePath, []byte(compiled.Profile), 0o600); err != nil {
		return err
	}
	rest := []string{}
	if len(cmd.Args) > 1 {
		rest = cmd.Args[1:]
	}
	cmd.Path = exe
	cmd.Args = append([]string{"sandbox-exec", "-f", profilePath, orig}, rest...)
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
	if sandboxExecPath() == "" {
		return IsolationReport{Backend: "darwin", Available: false, Mode: "unavailable", Missing: []string{"seatbelt"}, Detail: "sandbox-exec not found; refusing silent unrestricted execution"}
	}
	return IsolationReport{Backend: "darwin", Available: true, Mode: "seatbelt", Features: []string{"seatbelt", "process_group", "env_filter"}, Detail: "macOS sandbox-exec seatbelt profile + process group"}
}
