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
// Discovered executables are run normally as the server OS user for probing.
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
		if lp, err := loginShellPATH(ctx, opts.LoginPATHTimeout, home); err == nil && lp != "" {
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
						probeVersionOnly(ctx, &inst, def, opts.ProbeTimeout)
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
					probeOne(ctx, &inst, def, opts.ProbeTimeout)
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
				probeOne(ctx, &inst, def, opts.ProbeTimeout)
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
			probeOne(ctx, &inst, def, opts.ProbeTimeout)
		}
		emit(inst)
	}
	return out, nil
}

func baseInstallation(def Definition, acpExe, cliExe, bridgeExe string) Installation {
	inst := Installation{
		ID:                    id.New(),
		DefinitionID:          def.ID,
		DefinitionSource:      def.Source,
		Enabled:               def.Enabled,
		DisplayName:           def.DisplayName,
		Homepage:              def.Homepage,
		Executable:            acpExe,
		CLIExecutable:         cliExe,
		BridgeExecutable:      bridgeExe,
		BridgePresent:         bridgeExe != "",
		VersionArgs:           append([]string{}, def.VersionArgs...),
		Health:                domain.HarnessUnavailable,
		Compatibility:         domain.CompatIncompatible,
		Resume:                domain.ResumeReconstruct,
		AuthStatus:            "unknown",
		InterposeCommands:     def.InterposeCommands,
		ModelSelection:        def.ModelSelection,
		DefinitionFingerprint: def.ExecutionFingerprint(),
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
func probeVersionOnly(ctx context.Context, inst *Installation, def Definition, timeout time.Duration) {
	verTimeout := timeout
	if verTimeout > 3*time.Second {
		verTimeout = 3 * time.Second
	}
	out, err := runProbeCommand(ctx, verTimeout, inst.CLIExecutable, def.VersionArgs, probeEnv(inst.CLIExecutable))
	if err == nil {
		inst.Version = firstLine(out)
	} else {
		inst.VersionError = firstLine(out)
	}
}

func probeOne(ctx context.Context, inst *Installation, def Definition, timeout time.Duration) {
	// Version probe on the primary CLI when present, otherwise on the ACP
	// executable.
	versionExe := inst.CLIExecutable
	if versionExe == "" {
		versionExe = inst.Executable
	}
	if versionExe != "" {
		verTimeout := timeout
		if verTimeout > 3*time.Second {
			verTimeout = 3 * time.Second
		}
		out, err := runProbeCommand(ctx, verTimeout, versionExe, def.VersionArgs, probeEnv(versionExe))
		if err == nil {
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

	// ACP initialize probe. The harness runs normally as the server OS user.
	spec := acp.Spec{Command: inst.Executable, Args: definitionACPArgs(def), Env: probeEnv(inst.Executable), Dir: inst.Dir}
	res, err := acp.Probe(ctx, spec, timeout)
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
	inst.Resume = resumeFromCaps(init.AgentCapabilities)
	if inst.Version == "" && init.AgentInfo.Version != "" {
		inst.Version = init.AgentInfo.Version
	}
	if init.AgentInfo.Title != "" && inst.DefinitionID == "custom" {
		inst.DisplayName = init.AgentInfo.Title
	}
	inst.Health = domain.HarnessReady
	inst.Compatibility = domain.CompatRoutable
	if init.AgentCapabilities.HasNativeResume() || def.InterposeCommands {
		inst.Compatibility = domain.CompatEnhanced
	}
	if len(init.AuthMethods) > 0 {
		// Many agents advertise auth methods even when already authenticated. The
		// probe does not attempt to resolve auth state, so report it honestly.
		inst.AuthStatus = "unknown"
		inst.Notes = append(inst.Notes, "agent advertises auth methods; authentication is harness-owned and not verified by the probe")
		return
	}
	inst.AuthStatus = "none"
}

// probeEnv builds the environment for a probe: the server environment with the
// fake-harness knobs and any required interpreter PATH applied. Discovered
// harnesses run normally as the server OS user, so the environment is inherited.
func probeEnv(exePath string) []string {
	extra := map[string]string{}
	for _, k := range []string{"WAYSHARD_FAKE_SCENARIO", "WAYSHARD_FAKE_INIT_CANARY", "WAYSHARD_FAKE_PROBE_DAEMON", "WAYSHARD_FAKE_PROBE_DAEMON_HANG"} {
		if v := os.Getenv(k); v != "" {
			extra[k] = v
		}
	}
	if p := harnessEnvPATH(exePath, os.Getenv("PATH")); p != os.Getenv("PATH") {
		extra["PATH"] = p
	}
	return mergeEnv(os.Environ(), extra)
}

// runProbeCommand runs a probe command with a timeout and bounded combined
// output. The process tree is terminated best-effort on timeout/cancel.
func runProbeCommand(ctx context.Context, timeout time.Duration, exe string, args []string, env []string) ([]byte, error) {
	if exe == "" {
		return nil, errors.New("no executable")
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe, args...)
	cmd.Env = env
	process.Configure(cmd)
	buf := &limitedBuffer{limit: 256 << 10}
	cmd.Stdout = buf
	cmd.Stderr = buf
	cmd.Cancel = func() error { process.KillTree(cmd); return nil }
	cmd.WaitDelay = 2 * time.Second
	err := cmd.Run()
	return buf.Bytes(), err
}

type limitedBuffer struct {
	limit     int
	buf       []byte
	truncated bool
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	remain := b.limit - len(b.buf)
	if remain <= 0 {
		b.truncated = true
		return len(p), nil
	}
	if len(p) > remain {
		b.buf = append(b.buf, p[:remain]...)
		b.truncated = true
		return len(p), nil
	}
	b.buf = append(b.buf, p...)
	return len(p), nil
}

func (b *limitedBuffer) Bytes() []byte { return b.buf }

func mergeEnv(base []string, extra map[string]string) []string {
	if len(extra) == 0 {
		return base
	}
	out := make([]string, 0, len(base)+len(extra))
	seen := map[string]bool{}
	for _, kv := range base {
		eq := strings.IndexByte(kv, '=')
		if eq <= 0 {
			out = append(out, kv)
			continue
		}
		key := kv[:eq]
		if v, ok := extra[key]; ok {
			out = append(out, key+"="+v)
			seen[key] = true
			continue
		}
		out = append(out, kv)
	}
	for k, v := range extra {
		if !seen[k] {
			out = append(out, k+"="+v)
		}
	}
	return out
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
	var candidates []string
	p := filepath.Join(home, rel)
	if strings.ContainsAny(rel, "*?[") {
		m, err := filepath.Glob(p)
		if err != nil {
			return nil
		}
		if len(m) > maxGlobMatches {
			m = m[:maxGlobMatches]
		}
		candidates = m
	} else {
		candidates = []string{p}
	}
	var out []string
	for _, c := range candidates {
		cr, err := filepath.Rel(home, c)
		if err != nil {
			continue
		}
		rp, err := resolveRootSafe(home, cr)
		if err != nil {
			continue
		}
		out = append(out, rp)
	}
	return out
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
// validated as a PATH string only; it is never executed as a command.
func loginShellPATH(ctx context.Context, timeout time.Duration, home string) (string, error) {
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
	out, err := runProbeCommand(ctx, timeout, shell, []string{"-lc", `printf '%s' "$PATH"`}, os.Environ())
	if err != nil {
		return "", err
	}
	return sanitizeLoginPATH(strings.TrimSpace(string(out))), nil
}

// sanitizeLoginPATH validates the login shell's PATH as an absolute-only,
// deduplicated, bounded list of directories.
func sanitizeLoginPATH(raw string) string {
	seen := map[string]struct{}{}
	var parts []string
	for _, p := range filepath.SplitList(raw) {
		p = strings.TrimSpace(p)
		if p == "" || !filepath.IsAbs(p) {
			continue
		}
		if len(parts) >= maxListEntries*4 {
			break
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		parts = append(parts, p)
	}
	return strings.Join(parts, string(os.PathListSeparator))
}
