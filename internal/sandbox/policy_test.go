package sandbox

import (
	"context"
	"errors"
	"testing"
)

func TestRequiredIsolationNeverSilent(t *testing.T) {
	m := Manager{Backend: UnsupportedBackend{OS: "plan9"}}
	_, err := m.Start(context.Background(), Policy{Required: true, ReadWriteRoots: []string{"/tmp"}})
	if !errors.Is(err, ErrRequiredIsolation) {
		t.Fatalf("got %v", err)
	}
}

func TestLinuxRequiresWritableRoot(t *testing.T) {
	var b LinuxBackend
	_, err := b.Apply(context.Background(), Policy{Required: true})
	if !errors.Is(err, ErrRequiredIsolation) {
		t.Fatalf("got %v", err)
	}
}

func TestReducedSecurityIsExplicit(t *testing.T) {
	m := Manager{Backend: UnsupportedBackend{OS: "plan9"}}
	_, err := m.Start(context.Background(), Policy{Required: false, ReadWriteRoots: []string{"/tmp"}})
	if err == nil {
		t.Fatal("unsupported backend should still error; caller must choose reduced policy explicitly")
	}
}

func TestProbeNeverClaimsUnrestricted(t *testing.T) {
	r := Probe()
	if r.Mode == "unrestricted" {
		t.Fatal("must not report unrestricted")
	}
}
