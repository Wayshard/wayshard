package harness

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/Wayshard/wayshard/internal/acp"
	"github.com/Wayshard/wayshard/internal/domain"
	"github.com/Wayshard/wayshard/internal/id"
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
	searchPATH := opts.PATH
	if searchPATH == "" {
		searchPATH = os.Getenv("PATH")
	}
	if opts.IncludeLoginPATH {
		if lp, err := loginShellPATH(ctx, opts.LoginPATHTimeout); err == nil && lp != "" {
			searchPATH = mergePATH(searchPATH, lp)
		}
	}

	home := opts.Home
	if home == "" {
		home, _ = os.UserHomeDir()
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
				probeOne(ctx, &inst, opts.ProbeTimeout)
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
			probeOne(ctx, &inst, opts.ProbeTimeout)
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

func probeOne(ctx context.Context, inst *Installation, timeout time.Duration) {
	ad := AdapterFor(inst.Adapter, inst.Executable)
	spec := ad.LaunchSpec(*inst)
	ver := acp.ProbeVersion(ctx, inst.Executable, nil)
	inst.Version = ver

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
		inst.Health = domain.HarnessUnauth
		inst.AuthStatus = "unauthenticated"
		inst.Compatibility = domain.CompatCore
		inst.Notes = append(inst.Notes, "agent advertised auth methods; session creation may require authenticate")
		return
	}
	inst.AuthStatus = "none"
	inst.Health = domain.HarnessReady
	inst.Compatibility = domain.CompatRoutable
	if init.AgentCapabilities.HasNativeResume() || ad.InterposeCommands() {
		inst.Compatibility = domain.CompatEnhanced
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

// loginShellPATH asks the user's login shell for PATH. The output is parsed as
// a PATH string only; it is never executed. Bounded by timeout; stdin is unused.
func loginShellPATH(ctx context.Context, timeout time.Duration) (string, error) {
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
	cmd := exec.CommandContext(ctx, shell, "-lc", `printf '%s' "$PATH"`)
	cmd.Stdin = nil
	cmd.Env = os.Environ()
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	s := strings.TrimSpace(string(out))
	if s == "" || strings.ContainsAny(s, "\n\r") {
		return "", fmt.Errorf("login-shell PATH output rejected")
	}
	return s, nil
}
