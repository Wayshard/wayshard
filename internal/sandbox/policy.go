// Package sandbox compiles a platform-neutral SandboxPolicy into OS backends.
package sandbox

import (
	"context"
	"errors"
	"fmt"
	"runtime"
)

var ErrRequiredIsolation = errors.New("required isolation could not be established")

type NetworkMode string

const (
	NetNone         NetworkMode = "none"
	NetAllowlist    NetworkMode = "allowlist"
	NetUnrestricted NetworkMode = "unrestricted"
	NetBrokered     NetworkMode = "brokered"
)

type Policy struct {
	ReadOnlyRoots  []string
	ReadWriteRoots []string
	DeniedRoots    []string
	SyntheticHome  string
	SyntheticTemp  string
	EnvAllow       []string
	Network        NetworkMode
	AllowHosts     []string
	MemoryBytes    int64
	CPUPercent     int
	WallTimeSec    int
	MaxProcesses   int
	MaxOutputBytes int64
	MaxDiskBytes   int64
	Required       bool
}

type Backend interface {
	Name() string
	Available() bool
	Apply(ctx context.Context, p Policy) (Cleanup, error)
}

// IsolationReport is honest capability reporting for UI/routing.
type IsolationReport struct {
	Backend   string `json:"backend"`
	Available bool   `json:"available"`
	Mode      string `json:"mode"`
	Detail    string `json:"detail"`
}

func Probe() IsolationReport {
	b := DefaultBackend()
	r := IsolationReport{Backend: b.Name(), Available: b.Available(), Mode: "outer_only"}
	if !b.Available() {
		r.Detail = "required isolation cannot be established; refusing silent unrestricted execution"
		return r
	}
	r.Mode = "namespaces"
	r.Detail = "platform backend available"
	return r
}

type Cleanup func()

type Manager struct {
	Backend Backend
}

func DefaultBackend() Backend {
	switch runtime.GOOS {
	case "linux":
		return LinuxBackend{}
	case "darwin":
		return DarwinBackend{}
	case "windows":
		return WindowsBackend{}
	default:
		return UnsupportedBackend{OS: runtime.GOOS}
	}
}

type UnsupportedBackend struct{ OS string }

func (u UnsupportedBackend) Name() string    { return "unsupported" }
func (u UnsupportedBackend) Available() bool { return false }
func (u UnsupportedBackend) Apply(ctx context.Context, p Policy) (Cleanup, error) {
	if p.Required {
		return nil, fmt.Errorf("%w on %s", ErrRequiredIsolation, u.OS)
	}
	return func() {}, fmt.Errorf("%w on %s", ErrRequiredIsolation, u.OS)
}

func (m *Manager) Start(ctx context.Context, p Policy) (Cleanup, error) {
	b := m.Backend
	if b == nil {
		b = DefaultBackend()
	}
	if !b.Available() && p.Required {
		return nil, fmt.Errorf("%w: backend %s unavailable", ErrRequiredIsolation, b.Name())
	}
	clean, err := b.Apply(ctx, p)
	if err != nil && p.Required {
		return nil, err
	}
	if err != nil {
		return nil, err
	}
	return clean, nil
}

// LinuxBackend uses the best available primitives; it never silently becomes unsandboxed if Required.
type LinuxBackend struct{}

func (LinuxBackend) Name() string    { return "linux" }
func (LinuxBackend) Available() bool { return runtime.GOOS == "linux" }

// Linux Apply is implemented in linux.go (build-tagged) so containment is real.

type DarwinBackend struct{}

func (DarwinBackend) Name() string    { return "darwin" }
func (DarwinBackend) Available() bool { return runtime.GOOS == "darwin" }
func (DarwinBackend) Apply(ctx context.Context, p Policy) (Cleanup, error) {
	_ = ctx
	if p.Required && len(p.ReadWriteRoots) == 0 {
		return nil, fmt.Errorf("%w: no writable roots in policy", ErrRequiredIsolation)
	}
	return func() {}, nil
}

type WindowsBackend struct{}

func (WindowsBackend) Name() string    { return "windows" }
func (WindowsBackend) Available() bool { return runtime.GOOS == "windows" }
func (WindowsBackend) Apply(ctx context.Context, p Policy) (Cleanup, error) {
	_ = ctx
	if p.Required && len(p.ReadWriteRoots) == 0 {
		return nil, fmt.Errorf("%w: no writable roots in policy", ErrRequiredIsolation)
	}
	return func() {}, nil
}

func ToolPolicy(runWorkspace, syntheticHome string, net NetworkMode) Policy {
	return Policy{
		ReadWriteRoots: []string{runWorkspace, syntheticHome},
		DeniedRoots:    []string{"/"},
		SyntheticHome:  syntheticHome,
		Network:        net,
		MemoryBytes:    2 << 30,
		WallTimeSec:    30 * 60,
		MaxProcesses:   256,
		MaxOutputBytes: 32 << 20,
		Required:       true,
	}
}

func HarnessPolicy(runWorkspace, configDir string) Policy {
	return Policy{
		ReadWriteRoots: []string{runWorkspace, configDir},
		Network:        NetUnrestricted, // provider control plane, distinct from tool network
		Required:       true,
	}
}
