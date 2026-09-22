package sandbox

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
)

func requiredPolicies(t *testing.T) []Policy {
	t.Helper()
	ws := t.TempDir()
	home := t.TempDir()
	view := t.TempDir()
	exe, _ := os.Executable()
	if exe == "" {
		exe = filepath.Join(home, "probe")
	}
	return []Policy{
		ToolPolicy(ws, home, NetNone),
		HarnessPolicy(ws, home),
		ReadOnlyViewPolicy(view, home),
		ProbePolicy(exe, home, home),
	}
}

// TestRequiredPolicyCompileInvariant is the core cross-platform invariant: for
// every required production policy, the native backend either fails closed with
// ErrRequiredIsolation or declares every containment feature the policy depends
// on. A backend must never compile a required policy while silently omitting a
// required feature.
func TestRequiredPolicyCompileInvariant(t *testing.T) {
	c := AsConstrainer(DefaultBackend())
	for _, p := range requiredPolicies(t) {
		compiled, err := c.Compile(p)
		if err != nil {
			if !errors.Is(err, ErrRequiredIsolation) {
				t.Fatalf("compile error must be ErrRequiredIsolation, got %v", err)
			}
			continue
		}
		for _, f := range RequiredFeatures(p) {
			if !compiled.hasFeature(f) {
				t.Fatalf("backend %s compiled a required policy without declaring feature %q (features=%v)",
					compiled.Backend, f, compiled.Features)
			}
		}
	}
}

func TestRequiredFeaturesToolPolicy(t *testing.T) {
	p := ToolPolicy("/ws", "/home", NetNone)
	got := RequiredFeatures(p)
	want := []Feature{FeatureProcessTree, FeatureFSRead, FeatureFSWrite, FeatureNetworkNone, FeatureSyntheticEnv}
	for _, f := range want {
		if !slices.Contains(got, f) {
			t.Fatalf("ToolPolicy must require %q; got %v", f, got)
		}
	}
}

func TestValidateRequiredFeaturesRejectsPartialCapability(t *testing.T) {
	c := Compiled{Backend: "partial", Features: []string{string(FeatureProcessTree)}}
	p := ToolPolicy("/ws", "/home", NetNone)
	err := validateRequiredFeatures(c, p)
	if !errors.Is(err, ErrRequiredIsolation) {
		t.Fatalf("partial capability must fail closed, got %v", err)
	}
}

func TestValidateRequiredFeaturesIgnoresNonRequired(t *testing.T) {
	c := Compiled{Backend: "partial"}
	p := ToolPolicy("/ws", "/home", NetNone)
	p.Required = false
	if err := validateRequiredFeatures(c, p); err != nil {
		t.Fatalf("non-required policy must not fail validation: %v", err)
	}
}

// TestWindowsBackendFailsClosedForRequiredPolicy proves the Windows backend
// refuses a required policy on every build (native or stub) instead of running
// with weaker containment than the policy promises.
func TestWindowsBackendFailsClosedForRequiredPolicy(t *testing.T) {
	_, err := (WindowsBackend{}).Compile(ToolPolicy(t.TempDir(), t.TempDir(), NetNone))
	if !errors.Is(err, ErrRequiredIsolation) {
		t.Fatalf("windows must fail closed for a required tool policy, got %v", err)
	}
}

func TestWindowsReportDoesNotClaimFilesystemOrNetwork(t *testing.T) {
	rep := (WindowsBackend{}).Report()
	if rep.Available {
		t.Fatalf("windows must not report the required sandbox available: %+v", rep)
	}
	for _, f := range []Feature{FeatureFSRead, FeatureFSWrite, FeatureNetworkNone, FeatureProcessTree} {
		if !slices.Contains(rep.Missing, string(f)) {
			t.Fatalf("windows report must list %q as missing; got %v", f, rep.Missing)
		}
	}
}

// TestNativeReportMatchesCompiledFeatures proves a backend's capability report
// never claims more than its own Compile declares for a required policy.
func TestNativeReportMatchesCompiledFeatures(t *testing.T) {
	c := AsConstrainer(DefaultBackend())
	rep := c.Report()
	if rep.Backend != runtime.GOOS && rep.Backend != "unsupported" {
		t.Fatalf("report backend %q on %s", rep.Backend, runtime.GOOS)
	}
	if rep.Mode == "unrestricted" || rep.Mode == "" {
		t.Fatalf("report mode %q must never be unrestricted/empty", rep.Mode)
	}
	if !rep.Available {
		return
	}
	compiled, err := c.Compile(ToolPolicy(t.TempDir(), t.TempDir(), NetNone))
	if err != nil {
		t.Fatalf("report says available but a required tool policy does not compile: %v", err)
	}
	for _, f := range RequiredFeatures(ToolPolicy("/ws", "/home", NetNone)) {
		if !compiled.hasFeature(f) {
			t.Fatalf("report says available but compiled policy is missing %q", f)
		}
	}
}
