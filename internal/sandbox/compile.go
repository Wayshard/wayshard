package sandbox

import (
	"context"
	"fmt"
	"os/exec"
)

// Compiled is a platform-neutral record of which SandboxPolicy features the
// current backend can actually enforce. Unavailable required features fail.
type Compiled struct {
	Backend     string   `json:"backend"`
	Features    []string `json:"features"`
	Unavailable []string `json:"unavailable,omitempty"`
	Profile     string   `json:"profile,omitempty"`
}

func (c Compiled) has(feature string) bool {
	for _, f := range c.Features {
		if f == feature {
			return true
		}
	}
	return false
}

func compileCommon(p Policy) error {
	if p.Required && len(p.ReadWriteRoots) == 0 {
		return fmt.Errorf("%w: no writable roots in policy", ErrRequiredIsolation)
	}
	return nil
}

func filterEnv(p Policy, base []string) []string {
	deny := map[string]struct{}{"TYPESAFE_API_KEY": {}, "WAYSHARD_VAULT_KEY": {}, "WAYSHARD_VAULT_PASSPHRASE": {}}
	if len(p.EnvAllow) == 0 {
		out := make([]string, 0, len(base))
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

// Constrainer is the launch-time compilation of SandboxPolicy onto a child.
type Constrainer interface {
	Compile(p Policy) (Compiled, error)
	Constrain(cmd *exec.Cmd, p Policy) error
	Attach(cmd *exec.Cmd, p Policy) (Cleanup, error)
	KillTree(cmd *exec.Cmd) error
	Report() IsolationReport
}

func AsConstrainer(b Backend) Constrainer {
	if c, ok := b.(Constrainer); ok {
		return c
	}
	return unsupportedConstrainer{b}
}

type unsupportedConstrainer struct{ Backend }

func (u unsupportedConstrainer) Compile(p Policy) (Compiled, error) {
	c := Compiled{Backend: u.Name(), Unavailable: []string{"process_group", "filesystem", "network", "job_object", "namespaces", "seatbelt"}}
	if p.Required {
		return c, fmt.Errorf("%w: backend %s cannot compile policy", ErrRequiredIsolation, u.Name())
	}
	return c, fmt.Errorf("%w: backend %s cannot compile policy", ErrRequiredIsolation, u.Name())
}
func (u unsupportedConstrainer) Constrain(cmd *exec.Cmd, p Policy) error {
	if p.Required {
		return fmt.Errorf("%w: backend %s", ErrRequiredIsolation, u.Name())
	}
	return fmt.Errorf("%w: backend %s", ErrRequiredIsolation, u.Name())
}
func (u unsupportedConstrainer) Attach(*exec.Cmd, Policy) (Cleanup, error) {
	return func() {}, nil
}
func (u unsupportedConstrainer) KillTree(*exec.Cmd) error { return nil }
func (u unsupportedConstrainer) Report() IsolationReport {
	return IsolationReport{Backend: u.Name(), Available: false, Mode: "unavailable", Detail: "required isolation cannot be established; refusing silent unrestricted execution"}
}

func Probe() IsolationReport {
	return AsConstrainer(DefaultBackend()).Report()
}

// RunConstrained compiles policy, constrains the child, and returns an unstarted command.
func RunConstrained(ctx context.Context, p Policy, name string, args ...string) (*exec.Cmd, error) {
	_ = ctx
	b := DefaultBackend()
	c := AsConstrainer(b)
	if _, err := c.Compile(p); err != nil {
		return nil, err
	}
	cmd := exec.Command(name, args...)
	if err := c.Constrain(cmd, p); err != nil {
		return nil, err
	}
	return cmd, nil
}
