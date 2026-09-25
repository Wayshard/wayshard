// Package harness discovers installed coding harnesses from the effective
// harness catalog and maps them onto ACP launch and negotiated resume
// capabilities.
//
// Wayshard never installs, downloads, or bootstraps harnesses or ACP bridges.
// Discovered harnesses run as the Wayshard server OS user with their normal
// configuration, authentication, environment, filesystem and network access.
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
	// ok | incompatible | bridge_missing | disabled.
	ACPStatus      string
	ACPError       string
	BlockingReason string

	Dir           string
	Health        domain.HarnessHealth
	Compatibility domain.CompatibilityClass
	Resume        domain.SessionResumeCapability
	AuthStatus    string
	Capabilities  acp.AgentCapabilities
	AgentInfo     acp.Implementation
	AuthMethods   []acp.AuthMethod
	Notes         []string

	// Catalog-derived behavior carried to the executor.
	InterposeCommands bool
	ModelSelection    string
	// DefinitionFingerprint is the execution fingerprint of the effective
	// definition that produced this installation. A persisted installation is
	// only current while the fingerprint still matches the effective catalog.
	DefinitionFingerprint string
}

// definitionACPArgs returns the argv used to speak ACP for this installation.
func definitionACPArgs(def Definition) []string {
	if def.ACP == "bridge" {
		return append([]string{}, def.BridgeArgs...)
	}
	return append([]string{}, def.ACPArgs...)
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
