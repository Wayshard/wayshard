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

// package runners that would download/install a harness if used as a launcher.
var forbiddenLaunchers = map[string]struct{}{
	"npx": {}, "npm": {}, "yarn": {}, "pnpm": {}, "bun": {}, "bunx": {},
	"deno": {}, "pipx": {}, "uvx": {},
}

// Definition is a known harness recipe. Optional ACP features are still probed.
type Definition struct {
	ID          string
	DisplayName string
	Names       []string
	Adapter     string
}

// KnownDefinitions are executable names Wayshard will look up. Discovery never
// scans every PATH entry and never installs a missing name.
func KnownDefinitions() []Definition {
	return []Definition{
		{ID: "opencode", DisplayName: "OpenCode", Names: []string{"opencode"}, Adapter: AdapterOpenCode},
		{ID: "codex", DisplayName: "Codex", Names: []string{"codex", "codex-acp"}, Adapter: AdapterCodex},
		{ID: "wayshard-fake-acp", DisplayName: "Wayshard Fake ACP", Names: []string{"wayshard-fake-acp"}, Adapter: AdapterGeneric},
	}
}

// DiscoverOptions controls where executables are resolved. Probe talks ACP.
type DiscoverOptions struct {
	PATH             string
	Home             string
	ExtraPaths       []string // explicit configured absolute (or home-relative) executables
	LookupNames      []string // override known names
	IncludeLoginPATH bool
	WellKnownDirs    bool
	Probe            bool
	ProbeTimeout     time.Duration
	LoginPATHTimeout time.Duration
	// Owners, when non-nil, gives every probe process tree a durable ownership
	// record so startup reconciliation can terminate surviving descendants
	// after a server crash.
	Owners ProbeOwnerSink
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

// Discover finds installed harness executables. It never downloads, npx-installs,
// or otherwise bootstraps a missing harness.
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

	var searchDirs []string
	searchDirs = append(searchDirs, splitPATH(searchPATH)...)
	if opts.WellKnownDirs {
		searchDirs = append(searchDirs, wellKnownBinDirs(home)...)
	}

	seen := map[string]struct{}{}
	var out []Installation

	lookups := opts.LookupNames
	defsByName := map[string]Definition{}
	for _, def := range KnownDefinitions() {
		for _, n := range def.Names {
			defsByName[n] = def
			if len(opts.LookupNames) == 0 {
				lookups = append(lookups, n)
			}
		}
	}

	for _, name := range lookups {
		if forbiddenLauncher(name) {
			continue
		}
		def := defsByName[name]
		if def.ID == "" {
			def = Definition{ID: name, DisplayName: name, Names: []string{name}, Adapter: AdapterGeneric}
		}
		resolved := lookInDirs(searchDirs, name)
		for _, exe := range resolved {
			key := canonicalPath(exe)
			if _, ok := seen[key]; ok {
				continue
			}
			if forbiddenLauncher(exe) {
				continue
			}
			seen[key] = struct{}{}
			inst := baseInstallation(def, exe)
			if opts.Probe {
				probeOne(ctx, &inst, opts.ProbeTimeout, opts.Owners)
			} else {
				inst.Health = domain.HarnessUnavailable
				inst.Notes = append(inst.Notes, "not probed")
			}
			out = append(out, inst)
		}
	}

	for _, p := range opts.ExtraPaths {
		p = expandHome(p, home)
		if p == "" {
			continue
		}
		if forbiddenLauncher(p) {
			out = append(out, Installation{
				ID:           id.New(),
				DefinitionID: "custom",
				DisplayName:  filepath.Base(p),
				Executable:   p,
				Adapter:      AdapterGeneric,
				Health:       domain.HarnessUnavailable,
				Notes:        []string{"refusing package-runner launcher (npx/npm/etc); Wayshard never installs harnesses"},
			})
			continue
		}
		st, err := os.Stat(p)
		if err != nil || st.IsDir() {
			out = append(out, Installation{
				ID:           id.New(),
				DefinitionID: "custom",
				DisplayName:  filepath.Base(p),
				Executable:   p,
				Adapter:      AdapterGeneric,
				Health:       domain.HarnessUnavailable,
				Notes:        []string{"configured path not found"},
			})
			continue
		}
		key := canonicalPath(p)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		def := Definition{ID: "custom", DisplayName: filepath.Base(p), Adapter: AdapterFor("", p).ID()}
		inst := baseInstallation(def, p)
		if opts.Probe {
			probeOne(ctx, &inst, opts.ProbeTimeout, opts.Owners)
		}
		out = append(out, inst)
	}
	return out, nil
}

func baseInstallation(def Definition, exe string) Installation {
	ad := AdapterFor(def.Adapter, exe)
	return Installation{
		ID:           id.New(),
		DefinitionID: def.ID,
		DisplayName:  def.DisplayName,
		Executable:   exe,
		Adapter:      ad.ID(),
		Health:       domain.HarnessUnavailable,
		Isolation:    ad.Isolation(acp.AgentCapabilities{}),
		Resume:       domain.ResumeReconstruct,
		AuthStatus:   "unknown",
	}
}

func probeOne(ctx context.Context, inst *Installation, timeout time.Duration, owners ProbeOwnerSink) {
	ad := AdapterFor(inst.Adapter, inst.Executable)
	spec := ad.LaunchSpec(*inst)

	// A dedicated ProbePolicy confines the version probe and the ACP
	// initialize: NetworkNone, synthetic HOME/TEMP, no project/SourceWorkspace,
	// no Wayshard runtime/DB/vault, no SSH agent or display sockets. If the
	// platform cannot enforce it, the probe is reported unavailable rather than
	// run unrestricted.
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
	pol := sandbox.ProbePolicy(inst.Executable, home, tmp)
	// A script/symlink harness (for example a Node ACP adapter) needs its
	// package tree and interpreter available to launch at all. These are
	// read-only and scoped to the harness's own package, never the home dir.
	closure := harnessClosureFor(inst.Executable)
	pol.ReadOnlyRoots = append(pol.ReadOnlyRoots, closure.Roots...)
	pol.ReadOnlyRoots = append(pol.ReadOnlyRoots, harnessProcRoots()...)

	// The deterministic fake harness scenario knob is forwarded so its behavior
	// is observable in tests; nothing else from the host environment survives.
	base := map[string]string{}
	for _, k := range []string{"WAYSHARD_FAKE_SCENARIO", "WAYSHARD_FAKE_INIT_CANARY", "WAYSHARD_FAKE_PROBE_DAEMON", "WAYSHARD_FAKE_PROBE_DAEMON_HANG"} {
		if v := os.Getenv(k); v != "" {
			base[k] = v
		}
	}
	if p := harnessEnvPATH(inst.Executable, os.Getenv("PATH")); p != os.Getenv("PATH") {
		base["PATH"] = p
	}

	// Every probe process tree gets durable ownership before launch so startup
	// reconciliation can terminate daemonized descendants after a server crash.
	versionEnv, versionLease, lerr := beginProbeEnv(ctx, owners, "version", home, tmp, base)
	if lerr != nil {
		classifyProbeError(inst, lerr)
		inst.Notes = append(inst.Notes, "probe ownership unavailable")
		return
	}
	spec.Env = versionEnv

	verTimeout := timeout
	if verTimeout > 3*time.Second {
		verTimeout = 3 * time.Second
	}
	out, verr := sandbox.RunConstrainedOutputWithStart(ctx, pol, verTimeout, 256<<10, inst.Executable, probeVersionArgs(ad), versionEnv, func(pgid int) { probeSetPGID(versionLease, pgid) })
	probeDone(versionLease)
	if errors.Is(verr, sandbox.ErrRequiredIsolation) {
		classifyProbeError(inst, verr)
		inst.Notes = append(inst.Notes, "probe isolation unavailable")
		return
	}
	inst.Version = firstLine(out)

	con := sandbox.AsConstrainer(sandbox.DefaultBackend())
	if _, err := con.Compile(pol); err != nil {
		classifyProbeError(inst, err)
		return
	}
	spec.SetupCmd = func(cmd *exec.Cmd) error { return con.Constrain(cmd, pol) }

	initEnv, initLease, lerr := beginProbeEnv(ctx, owners, "initialize", home, tmp, base)
	if lerr != nil {
		classifyProbeError(inst, lerr)
		inst.Notes = append(inst.Notes, "probe ownership unavailable")
		return
	}
	spec.Env = initEnv
	spec.AfterStart = func(cmd *exec.Cmd) (func(), error) {
		if cmd.Process != nil {
			probeSetPGID(initLease, cmd.Process.Pid)
		}
		return con.Attach(cmd, pol)
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
		if inst.Version != "" && inst.Health == domain.HarnessIncompatible {
			inst.Health = domain.HarnessIncompatible
		}
		return
	}
	if res == nil || res.Initialize == nil {
		inst.Health = domain.HarnessIncompatible
		inst.Compatibility = domain.CompatIncompatible
		inst.Notes = append(inst.Notes, "initialize returned no result")
		return
	}
	init := res.Initialize
	inst.Capabilities = init.AgentCapabilities
	inst.AgentInfo = init.AgentInfo
	inst.AuthMethods = init.AuthMethods
	inst.Isolation = ad.Isolation(init.AgentCapabilities)
	inst.IsolationDetail = ad.IsolationDetail(init.AgentCapabilities)
	inst.Resume = ad.SessionResume(init.AgentCapabilities)
	if inst.Version == "" && init.AgentInfo.Version != "" {
		inst.Version = init.AgentInfo.Version
	}
	if init.AgentInfo.Title != "" && inst.DefinitionID == "custom" {
		inst.DisplayName = init.AgentInfo.Title
	}

	if len(init.AuthMethods) > 0 {
		// Many agents advertise auth methods even when they are already
		// authenticated. The probe runs without real harness configuration, so it
		// cannot determine auth state; report it honestly as unknown rather than
		// assuming unauthenticated. Authentication stays harness-owned and is
		// resolved when the harness actually runs with its config roots.
		inst.AuthStatus = "unknown"
		inst.Health = domain.HarnessReady
		inst.Compatibility = domain.CompatRoutable
		if init.AgentCapabilities.HasNativeResume() || ad.InterposeCommands() {
			inst.Compatibility = domain.CompatEnhanced
		}
		inst.Notes = append(inst.Notes, "agent advertises auth methods; authentication is harness-owned and not verified by the probe")
		return
	}
	inst.AuthStatus = "none"
	inst.Health = domain.HarnessReady
	inst.Compatibility = domain.CompatRoutable
	if init.AgentCapabilities.HasNativeResume() || ad.InterposeCommands() {
		inst.Compatibility = domain.CompatEnhanced
	}
}

// probeVersionArgs returns the argv used to ask a known harness for its version.
// Unknown/generic harnesses keep the no-argument probe.
func probeVersionArgs(ad HarnessAdapter) []string {
	switch ad.ID() {
	case AdapterOpenCode, AdapterCodex:
		return []string{"--version"}
	default:
		return nil
	}
}

func classifyProbeError(inst *Installation, err error) {
	switch {
	case errors.Is(err, acp.ErrIncompatible), errors.Is(err, acp.ErrProtocol), errors.Is(err, acp.ErrStdoutPollution):
		inst.Health = domain.HarnessIncompatible
		inst.Compatibility = domain.CompatIncompatible
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, acp.ErrHandshakeTimeout):
		inst.Health = domain.HarnessDegraded
		inst.Notes = append(inst.Notes, "initialize timed out")
	default:
		var rpc *acp.Error
		if errors.As(err, &rpc) && rpc.AuthRequired() {
			inst.Health = domain.HarnessUnauth
			inst.AuthStatus = "unauthenticated"
			inst.Compatibility = domain.CompatCore
			return
		}
		inst.Health = domain.HarnessUnavailable
		inst.Compatibility = domain.CompatIncompatible
	}
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
			filepath.Join(home, ".opencode", "bin"),
		)
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
	_, ok := forbiddenLaunchers[normalizeExecName(path)]
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
