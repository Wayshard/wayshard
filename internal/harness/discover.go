package harness

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/Wayshard/wayshard/internal/acp"
	"github.com/Wayshard/wayshard/internal/domain"
	"github.com/Wayshard/wayshard/internal/id"
	"github.com/Wayshard/wayshard/internal/process"
	"github.com/Wayshard/wayshard/internal/sandbox"
)

// DiscoverOptions controls where executables are resolved. Probe talks ACP.
type DiscoverOptions struct {
	PATH             string
	Home             string
	ExtraPaths       []string // explicit configured executables (absolute or ~-relative)
	LookupNames      []string // restrict to definitions whose id/executables/bridges match
	IncludeLoginPATH bool
	WellKnownDirs    bool
	Probe            bool
	ProbeTimeout     time.Duration
	LoginPATHTimeout time.Duration
	// Owners, when non-nil, gives every probe process tree a durable ownership
	// record so startup reconciliation can terminate surviving descendants
	// after a server crash.
	Owners ProbeOwnerSink
	// Definitions overrides the effective catalog. When nil, the effective
	// catalog (shipped + user) is loaded.
	Definitions []Definition
	// CatalogPath overrides the user catalog path (used by tests).
	CatalogPath string
}

func DefaultDiscoverOptions() DiscoverOptions {
	return DiscoverOptions{
		IncludeLoginPATH: true,
		WellKnownDirs:    true,
		Probe:            true,
		ProbeTimeout:     8 * time.Second,
		LoginPATHTimeout: 3 * time.Second,
	}
}

func (o DiscoverOptions) withDefaults() DiscoverOptions {
	d := DefaultDiscoverOptions()
	if o.ProbeTimeout <= 0 {
		o.ProbeTimeout = d.ProbeTimeout
	}
	if o.LoginPATHTimeout <= 0 {
		o.LoginPATHTimeout = d.LoginPATHTimeout
	}
	return o
}

// Discover finds installed members of the effective harness catalog. It never
// downloads, npx-installs, or otherwise bootstraps a missing harness or bridge.
func Discover(ctx context.Context, opts DiscoverOptions) ([]Installation, error) {
	opts = opts.withDefaults()
	home := opts.Home
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	searchPATH := opts.PATH
	if searchPATH == "" {
		searchPATH = os.Getenv("PATH")
	}
	if opts.IncludeLoginPATH {
		if lp, err := loginShellPATH(ctx, opts.LoginPATHTimeout, home, opts.Owners); err == nil && lp != "" {
			searchPATH = mergePATH(searchPATH, lp)
		}
	}
	baseDirs := splitPATH(searchPATH)
	if opts.WellKnownDirs {
		baseDirs = append(baseDirs, wellKnownBinDirs(home)...)
	}

	defs := opts.Definitions
	if defs == nil {
		cat, err := LoadCatalog(opts.CatalogPath)
		if err != nil {
			return nil, err
		}
		defs = cat.EnabledForPlatform(runtime.GOOS)
	}

	seen := map[string]struct{}{}
	var out []Installation
	emit := func(inst Installation) {
		key := canonicalPath(inst.Executable)
		if key != "" {
			if _, ok := seen[key]; ok {
				return
			}
			seen[key] = struct{}{}
		}
		out = append(out, inst)
	}

	for _, def := range defs {
		if !definitionMatchesLookup(def, opts.LookupNames) {
			continue
		}
		dirs := append([]string{}, baseDirs...)
		if opts.WellKnownDirs {
			for _, wk := range def.WellKnown {
				dirs = append(dirs, expandHomePattern(home, wk)...)
			}
		}
		cliPaths := resolveExecutableNames(dirs, def.Executables)
		bridgePaths := resolveExecutableNames(dirs, def.Bridges)

		if def.ACP == "bridge" {
			if len(bridgePaths) == 0 {
				// Report a present CLI without its required bridge. The CLI path is
				// used as the row identity; the entry is unavailable and never
				// routed because the ACP executable is absent.
				for _, cli := range cliPaths {
					inst := baseInstallation(def, cli, cli, "")
					inst.ACPStatus = "bridge_missing"
					inst.BlockingReason = "ACP bridge " + strings.Join(quoteAll(def.Bridges), " or ") + " is not installed"
					inst.Health = domain.HarnessUnavailable
					inst.Compatibility = domain.CompatIncompatible
					inst.Notes = append(inst.Notes, inst.BlockingReason)
					if opts.Probe {
						probeVersionOnly(ctx, &inst, def, opts.ProbeTimeout, opts.Owners)
					}
					emit(inst)
				}
				continue
			}
			for _, br := range bridgePaths {
				cli := ""
				if len(cliPaths) > 0 {
					cli = cliPaths[0]
				}
				inst := baseInstallation(def, br, cli, br)
				if opts.Probe {
					probeOne(ctx, &inst, def, opts.ProbeTimeout, opts.Owners)
				} else {
					inst.Health = domain.HarnessUnavailable
					inst.Notes = append(inst.Notes, "not probed")
				}
				emit(inst)
			}
			continue
		}

		for _, exe := range cliPaths {
			inst := baseInstallation(def, exe, exe, "")
			if opts.Probe {
				probeOne(ctx, &inst, def, opts.ProbeTimeout, opts.Owners)
			} else {
				inst.Health = domain.HarnessUnavailable
				inst.Notes = append(inst.Notes, "not probed")
			}
			emit(inst)
		}
	}

	// Explicit configured executables: match a catalog definition by executable
	// name; otherwise report an unavailable installation rather than inventing
	// behavior for an unknown harness.
	for _, p := range opts.ExtraPaths {
		p = expandHome(p, home)
		if p == "" {
			continue
		}
		base := normalizeExecName(p)
		var def Definition
		found := false
		for _, d := range defs {
			if containsName(d.Executables, base) || containsName(d.Bridges, base) {
				def = d
				found = true
				break
			}
		}
		if !found {
			emit(Installation{
				ID: id.New(), DefinitionID: "custom", DisplayName: filepath.Base(p),
				Executable: p, ACPStatus: "no_definition",
				BlockingReason: "no catalog definition matches " + filepath.Base(p),
				Health:         domain.HarnessUnavailable, Compatibility: domain.CompatIncompatible,
				AuthStatus: "unknown",
			})
			continue
		}
		inst := baseInstallation(def, p, p, "")
		if opts.Probe {
			probeOne(ctx, &inst, def, opts.ProbeTimeout, opts.Owners)
		}
		emit(inst)
	}
	return out, nil
}

func baseInstallation(def Definition, acpExe, cliExe, bridgeExe string) Installation {
	inst := Installation{
		ID:                      id.New(),
		DefinitionID:            def.ID,
		DefinitionSource:        def.Source,
		Enabled:                 def.Enabled,
		DisplayName:             def.DisplayName,
		Homepage:                def.Homepage,
		Executable:              acpExe,
		CLIExecutable:           cliExe,
		BridgeExecutable:        bridgeExe,
		BridgePresent:           bridgeExe != "",
		VersionArgs:             append([]string{}, def.VersionArgs...),
		Health:                  domain.HarnessUnavailable,
		Compatibility:           domain.CompatIncompatible,
		Resume:                  domain.ResumeReconstruct,
		AuthStatus:              "unknown",
		InterposeCommands:       def.InterposeCommands,
		ModelSelection:          def.ModelSelection,
		RequiresProviderNetwork: def.RequiresProviderNetwork,
		DeclaredTransport:       def.DeclaredTransport,
		ConfigRoots:             append([]string{}, def.ConfigRoots...),
		ACPRequiresLoopback:     def.ACPRequiresLoopback,
	}
	if inst.DisplayName == "" {
		inst.DisplayName = def.ID
	}
	if acpExe != "" {
		inst.Dir = filepath.Dir(acpExe)
	}
	return inst
}

func definitionMatchesLookup(def Definition, lookups []string) bool {
	if len(lookups) == 0 {
		return true
	}
	for _, l := range lookups {
		if l == def.ID || containsName(def.Executables, l) || containsName(def.Bridges, l) {
			return true
		}
	}
	return false
}

func containsName(list []string, name string) bool {
	for _, x := range list {
		if x == name {
			return true
		}
	}
	return false
}

func quoteAll(list []string) []string {
	out := make([]string, 0, len(list))
	for _, x := range list {
		out = append(out, "'"+x+"'")
	}
	return out
}

// resolveExecutableNames resolves bare executable names across search dirs and
// returns deduplicated physical paths.
func resolveExecutableNames(dirs, names []string) []string {
	var out []string
	seen := map[string]struct{}{}
	for _, name := range names {
		if name == "" || forbiddenLauncher(name) {
			continue
		}
		for _, p := range lookInDirs(dirs, name) {
			key := canonicalPath(p)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, p)
		}
	}
	return out
}

// probeVersionOnly runs just the version probe (used for a CLI whose bridge is
// missing, so the CLI presence is reported accurately).
func probeVersionOnly(ctx context.Context, inst *Installation, def Definition, timeout time.Duration, owners ProbeOwnerSink) {
	probeDir, err := os.MkdirTemp("", "wayshard-probe-")
	if err != nil {
		return
	}
	defer os.RemoveAll(probeDir)
	home := filepath.Join(probeDir, "home")
	tmp := filepath.Join(probeDir, "tmp")
	_ = os.MkdirAll(home, 0o700)
	_ = os.MkdirAll(tmp, 0o700)
	pol := sandbox.ProbePolicy(inst.CLIExecutable, home, tmp)
	pol.ReadOnlyRoots = append(pol.ReadOnlyRoots, harnessClosureFor(inst.CLIExecutable).Roots...)
	pol.ReadOnlyRoots = append(pol.ReadOnlyRoots, harnessProcRoots()...)
	base := probeBaseEnv(inst.CLIExecutable)
	env, lease, lerr := beginProbeEnv(ctx, owners, "version", home, tmp, base)
	if lerr != nil {
		return
	}
	verTimeout := timeout
	if verTimeout > 3*time.Second {
		verTimeout = 3 * time.Second
	}
	out, verr := sandbox.RunConstrainedOutputWithStart(ctx, pol, verTimeout, 256<<10, inst.CLIExecutable, def.VersionArgs, env, func(pgid int) { probeSetPGID(lease, pgid) })
	probeDone(lease)
	if verr == nil {
		inst.Version = firstLine(out)
	} else {
		inst.VersionError = firstLine(out)
	}
}

func probeOne(ctx context.Context, inst *Installation, def Definition, timeout time.Duration, owners ProbeOwnerSink) {
	probeDir, err := os.MkdirTemp("", "wayshard-probe-")
	if err != nil {
		classifyProbeError(inst, err)
		return
	}
	defer os.RemoveAll(probeDir)
	home := filepath.Join(probeDir, "home")
	tmp := filepath.Join(probeDir, "tmp")
	_ = os.MkdirAll(home, 0o700)
	_ = os.MkdirAll(tmp, 0o700)

	// Version probe on the primary CLI when present, otherwise on the ACP
	// executable. Version probing is always NetworkNone.
	versionExe := inst.CLIExecutable
	if versionExe == "" {
		versionExe = inst.Executable
	}
	if versionExe != "" {
		pol := sandbox.ProbePolicy(versionExe, home, tmp)
		pol.ReadOnlyRoots = append(pol.ReadOnlyRoots, harnessClosureFor(versionExe).Roots...)
		pol.ReadOnlyRoots = append(pol.ReadOnlyRoots, harnessProcRoots()...)
		base := probeBaseEnv(versionExe)
		versionEnv, versionLease, lerr := beginProbeEnv(ctx, owners, "version", home, tmp, base)
		if lerr != nil {
			classifyProbeError(inst, lerr)
			inst.Notes = append(inst.Notes, "probe ownership unavailable")
			return
		}
		verTimeout := timeout
		if verTimeout > 3*time.Second {
			verTimeout = 3 * time.Second
		}
		out, verr := sandbox.RunConstrainedOutputWithStart(ctx, pol, verTimeout, 256<<10, versionExe, def.VersionArgs, versionEnv, func(pgid int) { probeSetPGID(versionLease, pgid) })
		probeDone(versionLease)
		if errors.Is(verr, sandbox.ErrRequiredIsolation) {
			classifyProbeError(inst, verr)
			inst.Notes = append(inst.Notes, "probe isolation unavailable")
			return
		}
		if verr == nil {
			inst.Version = firstLine(out)
		} else {
			inst.VersionError = firstLine(out)
		}
	}

	if inst.Executable == "" {
		inst.ACPStatus = "bridge_missing"
		inst.Health = domain.HarnessUnavailable
		inst.Compatibility = domain.CompatIncompatible
		return
	}

	// ACP initialize probe. Harnesses whose ACP server needs private local IPC
	// use the isolated loopback capability when available; otherwise NetworkNone.
	acpPol := sandbox.ProbePolicy(inst.Executable, home, tmp)
	acpPol.ReadOnlyRoots = append(acpPol.ReadOnlyRoots, harnessClosureFor(inst.Executable).Roots...)
	acpPol.ReadOnlyRoots = append(acpPol.ReadOnlyRoots, harnessProcRoots()...)
	if def.ACPRequiresLoopback && sandbox.LoopbackProbeAvailable() {
		acpPol.Network = sandbox.NetLoopback
	}
	con := sandbox.AsConstrainer(sandbox.DefaultBackend())
	if _, err := con.Compile(acpPol); err != nil {
		classifyProbeError(inst, err)
		return
	}
	acpArgs := definitionACPArgs(def)
	base := probeBaseEnv(inst.Executable)
	initEnv, initLease, lerr := beginProbeEnv(ctx, owners, "initialize", home, tmp, base)
	if lerr != nil {
		classifyProbeError(inst, lerr)
		inst.Notes = append(inst.Notes, "probe ownership unavailable")
		return
	}
	spec := acp.Spec{Command: inst.Executable, Args: acpArgs, Env: initEnv, Dir: inst.Dir}
	spec.SetupCmd = func(cmd *exec.Cmd) error { return con.Constrain(cmd, acpPol) }
	spec.AfterStart = func(cmd *exec.Cmd) (func(), error) {
		if cmd.Process != nil {
			probeSetPGID(initLease, cmd.Process.Pid)
		}
		return con.Attach(cmd, acpPol)
	}
	res, err := acp.Probe(ctx, spec, timeout)
	probeDone(initLease)
	if res != nil {
		for _, d := range res.Stderr {
			if d.Source == "stderr" && d.Text != "" {
				inst.Notes = append(inst.Notes, d.Text)
			}
		}
	}
	if err != nil {
		classifyProbeError(inst, err)
		if inst.ACPStatus == "" {
			inst.ACPStatus = "incompatible"
		}
		return
	}
	if res == nil || res.Initialize == nil {
		inst.Health = domain.HarnessIncompatible
		inst.Compatibility = domain.CompatIncompatible
		inst.ACPStatus = "incompatible"
		inst.Notes = append(inst.Notes, "initialize returned no result")
		return
	}
	init := res.Initialize
	inst.ACPStatus = "ok"
	inst.Capabilities = init.AgentCapabilities
	inst.AgentInfo = init.AgentInfo
	inst.AuthMethods = init.AuthMethods
	inst.Isolation = definitionIsolation(def, init.AgentCapabilities)
	inst.IsolationDetail = definitionIsolationDetail(def, init.AgentCapabilities)
	inst.Resume = resumeFromCaps(init.AgentCapabilities)
	if inst.Version == "" && init.AgentInfo.Version != "" {
		inst.Version = init.AgentInfo.Version
	}
	if init.AgentInfo.Title != "" && inst.DefinitionID == "custom" {
		inst.DisplayName = init.AgentInfo.Title
	}
	if len(init.AuthMethods) > 0 {
		// Many agents advertise auth methods even when already authenticated. The
		// probe runs without real harness configuration, so auth state is unknown.
		inst.AuthStatus = "unknown"
		inst.Health = domain.HarnessReady
		inst.Compatibility = domain.CompatRoutable
		if init.AgentCapabilities.HasNativeResume() || def.InterposeCommands {
			inst.Compatibility = domain.CompatEnhanced
		}
		inst.Notes = append(inst.Notes, "agent advertises auth methods; authentication is harness-owned and not verified by the probe")
		return
	}
	inst.AuthStatus = "none"
	inst.Health = domain.HarnessReady
	inst.Compatibility = domain.CompatRoutable
	if init.AgentCapabilities.HasNativeResume() || def.InterposeCommands {
		inst.Compatibility = domain.CompatEnhanced
	}
}

func probeBaseEnv(exePath string) map[string]string {
	base := map[string]string{}
	for _, k := range []string{"WAYSHARD_FAKE_SCENARIO", "WAYSHARD_FAKE_INIT_CANARY", "WAYSHARD_FAKE_PROBE_DAEMON", "WAYSHARD_FAKE_PROBE_DAEMON_HANG"} {
		if v := os.Getenv(k); v != "" {
			base[k] = v
		}
	}
	if p := harnessEnvPATH(exePath, os.Getenv("PATH")); p != os.Getenv("PATH") {
		base["PATH"] = p
	}
	return base
}

func classifyProbeError(inst *Installation, err error) {
	switch {
	case errors.Is(err, acp.ErrIncompatible), errors.Is(err, acp.ErrProtocol), errors.Is(err, acp.ErrStdoutPollution):
		inst.Health = domain.HarnessIncompatible
		inst.Compatibility = domain.CompatIncompatible
		inst.ACPStatus = "incompatible"
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, acp.ErrHandshakeTimeout):
		inst.Health = domain.HarnessDegraded
		inst.ACPStatus = "incompatible"
		inst.Notes = append(inst.Notes, "initialize timed out")
	default:
		var rpc *acp.Error
		if errors.As(err, &rpc) && rpc.AuthRequired() {
			inst.Health = domain.HarnessUnauth
			inst.AuthStatus = "unauthenticated"
			inst.Compatibility = domain.CompatCore
			inst.ACPStatus = "incompatible"
			return
		}
		inst.Health = domain.HarnessUnavailable
		inst.Compatibility = domain.CompatIncompatible
		inst.ACPStatus = "incompatible"
	}
	inst.ACPError = err.Error()
	inst.Notes = append(inst.Notes, err.Error())
}

func firstLine(b []byte) string {
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return line
		}
	}
	return ""
}

func lookInDirs(dirs []string, name string) []string {
	var out []string
	names := []string{name}
	if runtime.GOOS == "windows" {
		names = []string{name, name + ".exe", name + ".cmd", name + ".bat"}
	}
	seen := map[string]struct{}{}
	for _, dir := range dirs {
		if dir == "" {
			continue
		}
		for _, n := range names {
			p := filepath.Join(dir, n)
			st, err := os.Stat(p)
			if err != nil || st.IsDir() {
				continue
			}
			if runtime.GOOS != "windows" && st.Mode()&0o111 == 0 {
				continue
			}
			key := canonicalPath(p)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, p)
		}
	}
	return out
}

func wellKnownBinDirs(home string) []string {
	var dirs []string
	if home != "" {
		dirs = append(dirs,
			filepath.Join(home, ".local", "bin"),
			filepath.Join(home, "bin"),
			filepath.Join(home, ".cargo", "bin"),
			filepath.Join(home, "go", "bin"),
			filepath.Join(home, ".volta", "bin"),
		)
		// Node version-manager layouts are common homes for npm-distributed
		// harness CLIs and ACP bridges. These are home-relative and never
		// project-controlled.
		if m, _ := filepath.Glob(filepath.Join(home, ".nvm", "versions", "node", "*", "bin")); len(m) > 0 {
			dirs = append(dirs, m...)
		}
	}
	switch runtime.GOOS {
	case "darwin":
		dirs = append(dirs, "/opt/homebrew/bin", "/usr/local/bin")
	case "windows":
		if local := os.Getenv("LOCALAPPDATA"); local != "" {
			dirs = append(dirs, filepath.Join(local, "Programs"))
		}
	default:
		dirs = append(dirs, "/usr/local/bin", "/usr/bin")
	}
	return dirs
}

// expandHomePattern resolves a catalog-declared home-relative well-known path.
// A glob pattern (for example `.nvm/versions/node/*/bin`) expands within HOME
// only; the catalog validator rejects absolute paths and `..`.
func expandHomePattern(home, rel string) []string {
	p := filepath.Join(home, rel)
	if strings.ContainsAny(rel, "*?[") {
		if m, err := filepath.Glob(p); err == nil {
			return m
		}
		return nil
	}
	return []string{p}
}

func splitPATH(p string) []string {
	return filepath.SplitList(p)
}

func mergePATH(a, b string) string {
	seen := map[string]struct{}{}
	var parts []string
	for _, p := range append(splitPATH(a), splitPATH(b)...) {
		if p == "" {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		parts = append(parts, p)
	}
	return strings.Join(parts, string(os.PathListSeparator))
}

func canonicalPath(p string) string {
	if abs, err := filepath.Abs(p); err == nil {
		p = abs
	}
	if rp, err := filepath.EvalSymlinks(p); err == nil {
		p = rp
	}
	return p
}

func homeDir() string {
	h, _ := os.UserHomeDir()
	return h
}

func expandHome(p, home string) string {
	if p == "" {
		return p
	}
	if strings.HasPrefix(p, "~"+string(os.PathSeparator)) || p == "~" {
		if home == "" {
			return p
		}
		return filepath.Join(home, strings.TrimPrefix(p, "~"))
	}
	return p
}

func forbiddenLauncher(path string) bool {
	_, ok := forbidden[normalizeExecName(path)]
	return ok
}

// loginShellPATH asks the user's login shell for PATH. The output is parsed and
// validated as a PATH string only; it is never executed. The probe runs under
// LoginShellPolicy (NetworkNone, synthetic TMP, secrets dropped) with read-only
// access only to the shell executable directory and the specific per-shell
// startup files required to compute PATH, never the home directory itself.
func loginShellPATH(ctx context.Context, timeout time.Duration, home string, owners ProbeOwnerSink) (string, error) {
	if runtime.GOOS == "windows" {
		return "", errors.New("login-shell PATH not used on windows")
	}
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	probeDir, derr := os.MkdirTemp("", "wayshard-loginpath-")
	if derr != nil {
		return "", derr
	}
	defer os.RemoveAll(probeDir)
	pol := sandbox.LoginShellPolicy(shell, home, "/etc", probeDir, probeDir)

	env, lease, lerr := beginProbeEnv(ctx, owners, "login_path", home, probeDir, nil)
	if lerr != nil {
		return "", lerr
	}
	out, err := sandbox.RunConstrainedOutputWithStart(ctx, pol, timeout, 64<<10, shell, []string{"-lc", `printf '%s' "$PATH"`}, env, func(pgid int) { probeSetPGID(lease, pgid) })
	probeDone(lease)
	if err != nil {
		return "", err
	}
	return sandbox.SanitizeLoginPATH(strings.TrimSpace(string(out)))
}

// beginProbeEnv resolves a durable ownership lease (when owners is non-nil),
// injects the raw token into the probe environment and returns the built
// environment. A nil sink yields no lease and an unmodified environment.
func beginProbeEnv(ctx context.Context, owners ProbeOwnerSink, kind, home, tmp string, base map[string]string) ([]string, ProbeLease, error) {
	add := map[string]string{}
	for k, v := range base {
		add[k] = v
	}
	if owners == nil {
		return sandbox.HarnessEnv(home, tmp, add), nil, nil
	}
	lease, err := owners.BeginProbe(ctx, kind)
	if err != nil {
		return nil, nil, err
	}
	add[process.TokenEnv] = lease.Token()
	return sandbox.HarnessEnv(home, tmp, add), lease, nil
}

func probeSetPGID(l ProbeLease, pgid int) {
	if l != nil {
		l.SetPGID(pgid)
	}
}

func probeDone(l ProbeLease) {
	if l != nil {
		l.Done()
	}
}
