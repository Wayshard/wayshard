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
	NetProvider     NetworkMode = "provider"
	NetLoopback     NetworkMode = "loopback"
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
	// AllowUnsafeHostNetwork must be explicitly set to permit NetUnrestricted.
	// It is never set by required-isolation production policies.
	AllowUnsafeHostNetwork bool
	AllowHosts             []string
	MemoryBytes            int64
	CPUPercent             int
	WallTimeSec            int
	MaxProcesses           int
	MaxOutputBytes         int64
	MaxDiskBytes           int64
	Required               bool
	// ProcIsolation runs the process in its own PID and mount namespace with a
	// private procfs, so /proc is scoped to the process and its descendants and
	// never exposes host processes. Used for harnesses/probes whose runtime
	// (for example Bun) requires /proc. Never set for tool/validation policies.
	ProcIsolation bool
	// ProcNamespaced is set by the backend when the clone flags for
	// ProcIsolation were actually applied, so the helper only mounts a procfs
	// inside its own namespaces.
	ProcNamespaced bool
	// LoopbackNamespaced is set by the backend when a private network namespace
	// (with only loopback) was created for NetLoopback.
	LoopbackNamespaced bool
}

type Backend interface {
	Name() string
	Available() bool
	Apply(ctx context.Context, p Policy) (Cleanup, error)
}

// IsolationReport is honest capability reporting for UI/routing.
type IsolationReport struct {
	Backend   string   `json:"backend"`
	Available bool     `json:"available"`
	Mode      string   `json:"mode"`
	Features  []string `json:"features,omitempty"`
	Missing   []string `json:"missing,omitempty"`
	Detail    string   `json:"detail"`
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
	_ = ctx
	return nil, fmt.Errorf("%w on %s", ErrRequiredIsolation, u.OS)
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

type LinuxBackend struct{}

func (LinuxBackend) Name() string    { return "linux" }
func (LinuxBackend) Available() bool { return runtime.GOOS == "linux" }

type DarwinBackend struct{}

func (DarwinBackend) Name() string    { return "darwin" }
func (DarwinBackend) Available() bool { return runtime.GOOS == "darwin" }

type WindowsBackend struct{}

func (WindowsBackend) Name() string    { return "windows" }
func (WindowsBackend) Available() bool { return runtime.GOOS == "windows" }

func ToolPolicy(runWorkspace, syntheticHome string, net NetworkMode) Policy {
	return Policy{
		ReadOnlyRoots:  systemReadOnlyRoots(),
		ReadWriteRoots: append([]string{runWorkspace, syntheticHome}, systemDeviceRoots()...),
		DeniedRoots:    nil,
		SyntheticHome:  syntheticHome,
		SyntheticTemp:  syntheticHome,
		Network:        net,
		MemoryBytes:    2 << 30,
		WallTimeSec:    30 * 60,
		MaxProcesses:   256,
		MaxOutputBytes: 32 << 20,
		Required:       true,
	}
}

func HarnessPolicy(runWorkspace, syntheticTemp string) Policy {
	return Policy{
		ReadOnlyRoots:  systemReadOnlyRoots(),
		ReadWriteRoots: append([]string{runWorkspace, syntheticTemp}, systemDeviceRoots()...),
		SyntheticTemp:  syntheticTemp,
		// Harnesses that require model/provider network are not launched under
		// raw host networking. Secure provider-only isolation is not yet
		// implemented, so required-isolation harnesses run with no network.
		Network:       NetNone,
		Required:      true,
		ProcIsolation: true,
	}
}

// ReadOnlyViewPolicy is used for read-only stages: the run/project view is
// readable but not writable, and non-system host paths stay denied.
func ReadOnlyViewPolicy(viewPath, syntheticTemp string) Policy {
	return Policy{
		ReadOnlyRoots:  append([]string{viewPath}, systemReadOnlyRoots()...),
		ReadWriteRoots: append([]string{syntheticTemp}, systemDeviceRoots()...),
		SyntheticTemp:  syntheticTemp,
		Network:        NetNone,
		Required:       true,
		ProcIsolation:  true,
	}
}
