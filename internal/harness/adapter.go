// Package harness discovers installed coding harnesses from the effective
// harness catalog and maps them onto ACP launch and honestly reported
// isolation/resume capabilities.
//
// Wayshard never installs, downloads, or bootstraps harnesses or ACP bridges.
// Capability negotiation is authoritative; optional ACP features are never
// inferred from an executable name. Harness behavior is described declaratively
// by the catalog (internal/harness/harnesses.toml and the user catalog), not by
// a compile-time list of supported harness names.
package harness

import (
	"path/filepath"
	"strings"

	"github.com/Wayshard/wayshard/internal/acp"
	"github.com/Wayshard/wayshard/internal/domain"
)

// Installation is an actual discovered installation of a catalog definition:
// resolved executable paths plus probe observations. It never asserts a
// capability that was not negotiated.
type Installation struct {
	ID               string
	DefinitionID     string
	DefinitionSource DefinitionSource
	Enabled          bool
	DisplayName      string
	Homepage         string

	// Executable is the executable Wayshard launches for ACP (the bridge when
	// the definition is bridge-based, otherwise the primary CLI).
	Executable string
	// CLIExecutable is the primary harness CLI when one was resolved.
	CLIExecutable string
	// BridgeExecutable is the resolved ACP bridge when the definition needs one.
	BridgeExecutable string
	BridgePresent    bool

	Version      string
	VersionArgs  []string
	VersionError string

	// ACPStatus is a structured discovery outcome:
	// ok | incompatible | bridge_missing | probe_unavailable | loopback_unavailable | disabled.
	ACPStatus      string
	ACPError       string
	BlockingReason string

	Dir             string
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

	// Catalog-derived behavior carried to the executor.
	InterposeCommands       bool
	ModelSelection          string
	RequiresProviderNetwork bool
	DeclaredTransport       string
	ConfigRoots             []string
	ACPRequiresLoopback     bool
}

// ACPArgs returns the argv used to speak ACP for this installation.
func definitionACPArgs(def Definition) []string {
	if def.ACP == "bridge" {
		return append([]string{}, def.BridgeArgs...)
	}
	return append([]string{}, def.ACPArgs...)
}

func definitionIsolation(def Definition, caps acp.AgentCapabilities) domain.IsolationMode {
	if caps.NativeSandboxAdvertised() {
		return domain.IsolationNative
	}
	if def.InterposeCommands {
		return domain.IsolationAdapterBridge
	}
	return domain.IsolationOuterOnly
}

func definitionIsolationDetail(def Definition, caps acp.AgentCapabilities) string {
	if caps.NativeSandboxAdvertised() {
		return "harness advertised native tool sandbox"
	}
	if def.InterposeCommands {
		return "adapter_bridge: Wayshard interposes ACP terminal/fs; inner harness tools are not independently verified"
	}
	return "outer_only: no inner tool sandbox verified; outer harness process only"
}

func normalizeExecName(path string) string {
	base := strings.ToLower(filepath.Base(path))
	base = strings.TrimSuffix(base, ".exe")
	base = strings.TrimSuffix(base, ".cmd")
	base = strings.TrimSuffix(base, ".bat")
	return base
}

func resumeFromCaps(caps acp.AgentCapabilities) domain.SessionResumeCapability {
	if caps.HasNativeResume() {
		return domain.ResumeNative
	}
	return domain.ResumeReconstruct
}
