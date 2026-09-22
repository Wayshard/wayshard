package sandbox

import (
	"context"
	"fmt"
	"os"
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

func (c Compiled) hasFeature(f Feature) bool { return c.has(string(f)) }

// RequiredFeatures lists the containment properties a policy depends on. A
// backend that cannot enforce any of them must refuse a Required policy rather
// than silently run with weaker containment.
func RequiredFeatures(p Policy) []Feature {
	var out []Feature
	if p.Required {
		// Every required launch must be cancellable as a whole process tree.
		out = append(out, FeatureProcessTree)
	}
	if len(p.ReadOnlyRoots) > 0 {
		out = append(out, FeatureFSRead)
	}
	if len(p.ReadWriteRoots) > 0 {
		out = append(out, FeatureFSWrite)
	}
	switch p.Network {
	case NetNone, "":
		out = append(out, FeatureNetworkNone)
	case NetLoopback:
		out = append(out, FeatureNetworkLoopback)
	case NetProvider:
		out = append(out, FeatureNetworkProvider)
	}
	if p.SyntheticHome != "" || p.SyntheticTemp != "" {
		out = append(out, FeatureSyntheticEnv)
	}
	return out
}

// validateRequiredFeatures fails a Required policy when the backend does not
// declare every feature the policy depends on. Non-required policies are used
// only by tests and management paths and are not validated here.
func validateRequiredFeatures(c Compiled, p Policy) error {
	if !p.Required {
		return nil
	}
	for _, f := range RequiredFeatures(p) {
		if !c.hasFeature(f) {
			return fmt.Errorf("%w: backend %s cannot enforce required feature %q", ErrRequiredIsolation, c.Backend, f)
		}
	}
	return nil
}

func compileCommon(p Policy, allowProvider bool) error {
	if p.Required && len(p.ReadWriteRoots) == 0 {
		return fmt.Errorf("%w: no writable roots in policy", ErrRequiredIsolation)
	}
	switch p.Network {
	case NetProvider, NetLoopback:
		if !allowProvider {
			return fmt.Errorf("%w: network mode %q is unavailable", ErrRequiredIsolation, p.Network)
		}
	case NetAllowlist, NetBrokered:
		return fmt.Errorf("%w: network mode %q is not implemented", ErrRequiredIsolation, p.Network)
	case NetUnrestricted:
		if !p.AllowUnsafeHostNetwork {
			return fmt.Errorf("%w: unrestricted host network requires explicit unsafe opt-in", ErrRequiredIsolation)
		}
	}
	return nil
}

func filterEnv(p Policy, base []string) []string {
	if base == nil {
		base = os.Environ()
	}
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
