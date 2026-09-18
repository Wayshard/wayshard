// Package harness discovers installed coding harnesses and maps them to adapters.
//
// Wayshard never installs, downloads, or bootstraps harnesses or ACP bridges.
// Capability negotiation is authoritative; optional ACP features are never
// inferred from an executable name.
package harness

import (
	"path/filepath"
	"strings"

	"github.com/Wayshard/wayshard/internal/acp"
	"github.com/Wayshard/wayshard/internal/domain"
)

const (
	AdapterGeneric  = "generic"
	AdapterOpenCode = "opencode"
	AdapterCodex    = "codex"
)

// HarnessAdapter maps a known (or generic) installation onto ACP launch and
// honestly reported isolation/resume capabilities.
type HarnessAdapter interface {
	ID() string
	DisplayName() string
	// LaunchSpec is the argv used to speak ACP on stdin/stdout.
	LaunchSpec(in Installation) acp.Spec
	// Isolation reports the effective tool-isolation mode given negotiated caps.
	Isolation(caps acp.AgentCapabilities) domain.IsolationMode
	IsolationDetail(caps acp.AgentCapabilities) string
	// SessionResume reports none/reconstruct/native_resume from advertised caps.
	SessionResume(caps acp.AgentCapabilities) domain.SessionResumeCapability
	// InterposeCommands is true when this adapter routes tool commands through
	// Wayshard (ACP terminal/fs callbacks), i.e. adapter_bridge.
	InterposeCommands() bool
}

// Installation is a discovered executable plus probe observations.
type Installation struct {
	ID              string
	DefinitionID    string
	DisplayName     string
	Executable      string
	ExtraArgs       []string
	Env             []string
	Dir             string
	Version         string
	Adapter         string
	Health          domain.HarnessHealth
	Compatibility   domain.CompatibilityClass
	Isolation       domain.IsolationMode
	IsolationDetail string
	Resume          domain.SessionResumeCapability
	AuthStatus      string
	Capabilities    acp.AgentCapabilities
	AgentInfo       acp.Implementation
	AuthMethods     []acp.AuthMethod
	Notes           []string
}

// AdapterFor selects a known adapter from definition ID or executable name.
// Optional ACP features are still taken only from negotiated capabilities.
func AdapterFor(definitionID, executable string) HarnessAdapter {
	id := strings.ToLower(strings.TrimSpace(definitionID))
	base := normalizeExecName(executable)
	switch id {
	case AdapterOpenCode:
		return OpenCodeAdapter{}
	case AdapterCodex:
		return CodexAdapter{}
	}
	switch base {
	case "opencode":
		return OpenCodeAdapter{}
	case "codex", "codex-acp":
		return CodexAdapter{}
	default:
		return GenericACPAdapter{}
	}
}

func normalizeExecName(path string) string {
	base := strings.ToLower(filepath.Base(path))
	base = strings.TrimSuffix(base, ".exe")
	base = strings.TrimSuffix(base, ".cmd")
	base = strings.TrimSuffix(base, ".bat")
	return base
}

func hasAcpArg(args []string) bool {
	for _, a := range args {
		if a == "acp" || a == "--acp" {
			return true
		}
	}
	return false
}

func resumeFromCaps(caps acp.AgentCapabilities) domain.SessionResumeCapability {
	if caps.HasNativeResume() {
		return domain.ResumeNative
	}
	return domain.ResumeReconstruct
}

// GenericACPAdapter is used for any ACP-compliant agent without a known profile.
type GenericACPAdapter struct{}

func (GenericACPAdapter) ID() string          { return AdapterGeneric }
func (GenericACPAdapter) DisplayName() string { return "Generic ACP" }

func (GenericACPAdapter) LaunchSpec(in Installation) acp.Spec {
	return acp.Spec{Command: in.Executable, Args: append([]string{}, in.ExtraArgs...), Env: in.Env, Dir: in.Dir}
}

func (GenericACPAdapter) Isolation(acp.AgentCapabilities) domain.IsolationMode {
	return domain.IsolationOuterOnly
}

func (GenericACPAdapter) IsolationDetail(acp.AgentCapabilities) string {
	return "generic ACP: no inner tool sandbox verified; outer harness process only"
}

func (GenericACPAdapter) SessionResume(caps acp.AgentCapabilities) domain.SessionResumeCapability {
	return resumeFromCaps(caps)
}

func (GenericACPAdapter) InterposeCommands() bool { return false }

// OpenCodeAdapter launches `opencode acp` and interposes command execution
// through ACP terminal/fs callbacks (adapter_bridge) when that is the
// advertised path. Native inner sandbox is reported only if negotiated.
type OpenCodeAdapter struct{}

func (OpenCodeAdapter) ID() string          { return AdapterOpenCode }
func (OpenCodeAdapter) DisplayName() string { return "OpenCode" }

func (OpenCodeAdapter) LaunchSpec(in Installation) acp.Spec {
	args := append([]string{}, in.ExtraArgs...)
	if !hasAcpArg(args) {
		args = append([]string{"acp"}, args...)
	}
	return acp.Spec{Command: in.Executable, Args: args, Env: in.Env, Dir: in.Dir}
}

func (a OpenCodeAdapter) Isolation(caps acp.AgentCapabilities) domain.IsolationMode {
	if caps.NativeSandboxAdvertised() {
		return domain.IsolationNative
	}
	if a.InterposeCommands() {
		return domain.IsolationAdapterBridge
	}
	return domain.IsolationOuterOnly
}

func (OpenCodeAdapter) IsolationDetail(caps acp.AgentCapabilities) string {
	if caps.NativeSandboxAdvertised() {
		return "harness advertised native tool sandbox"
	}
	return "adapter_bridge: Wayshard interposes ACP terminal/fs; inner OpenCode tools are not independently verified"
}

func (OpenCodeAdapter) SessionResume(caps acp.AgentCapabilities) domain.SessionResumeCapability {
	return resumeFromCaps(caps)
}

func (OpenCodeAdapter) InterposeCommands() bool { return true }

// CodexAdapter launches a user-installed Codex ACP entrypoint (never npx).
type CodexAdapter struct{}

func (CodexAdapter) ID() string          { return AdapterCodex }
func (CodexAdapter) DisplayName() string { return "Codex" }

func (CodexAdapter) LaunchSpec(in Installation) acp.Spec {
	args := append([]string{}, in.ExtraArgs...)
	base := normalizeExecName(in.Executable)
	if base == "codex" && !hasAcpArg(args) {
		args = append([]string{"acp"}, args...)
	}
	return acp.Spec{Command: in.Executable, Args: args, Env: in.Env, Dir: in.Dir}
}

func (a CodexAdapter) Isolation(caps acp.AgentCapabilities) domain.IsolationMode {
	if caps.NativeSandboxAdvertised() {
		return domain.IsolationNative
	}
	if a.InterposeCommands() {
		return domain.IsolationAdapterBridge
	}
	return domain.IsolationOuterOnly
}

func (CodexAdapter) IsolationDetail(caps acp.AgentCapabilities) string {
	if caps.NativeSandboxAdvertised() {
		return "harness advertised native tool sandbox"
	}
	return "adapter_bridge: Wayshard interposes ACP terminal/fs when the Codex ACP entrypoint uses them"
}

func (CodexAdapter) SessionResume(caps acp.AgentCapabilities) domain.SessionResumeCapability {
	return resumeFromCaps(caps)
}

func (CodexAdapter) InterposeCommands() bool { return true }
