package harness

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/BurntSushi/toml"
)

//go:embed harnesses.toml
var shippedCatalogTOML string

// CatalogSchemaVersion is the supported harness catalog schema version.
const CatalogSchemaVersion = 1

// Resource bounds for a manually edited user catalog. They are generous for
// real users and only guard against an accidentally or deliberately unbounded
// catalog causing excessive startup work. They are not a quota framework.
const (
	maxUserCatalogBytes   = 1 << 20 // 1 MiB
	maxCatalogDefinitions = 512
	maxListEntries        = 64
	maxArgEntries         = 128
	maxGlobMatches        = 256
)

// executionIdentityVersion is part of the execution fingerprint so a future
// change to the fingerprinted field set is an explicit version bump.
const executionIdentityVersion = "wayshard-harness-exec-v1"

// DefinitionSource identifies where an effective definition came from.
type DefinitionSource string

const (
	SourceShipped    DefinitionSource = "shipped"
	SourceUser       DefinitionSource = "user"
	SourceOverridden DefinitionSource = "overridden"
)

// Definition is declarative knowledge about how Wayshard recognizes, probes and
// invokes a harness family. It never asserts that an installation exists.
type Definition struct {
	ID          string
	Enabled     bool
	DisplayName string
	Homepage    string
	Platforms   []string

	Executables []string
	Bridges     []string
	WellKnown   []string
	VersionArgs []string

	ACP               string // native|bridge
	ACPArgs           []string
	BridgeArgs        []string
	InterposeCommands bool
	ModelSelection    string // none|config_option|set_model

	Source DefinitionSource
}

// executionIdentity is the canonical, versioned serialization of the fields
// that materially change what a harness launches or how it behaves. Cosmetic
// metadata (display_name, homepage) and bookkeeping (enabled, source) are
// excluded.
type executionIdentity struct {
	Version           string   `json:"version"`
	Executables       []string `json:"executables"`
	Bridges           []string `json:"bridges"`
	WellKnown         []string `json:"well_known"`
	VersionArgs       []string `json:"version_args"`
	ACP               string   `json:"acp"`
	ACPArgs           []string `json:"acp_args"`
	BridgeArgs        []string `json:"bridge_args"`
	InterposeCommands bool     `json:"interpose_commands"`
	ModelSelection    string   `json:"model_selection"`
	Platforms         []string `json:"platforms"`
}

// ExecutionFingerprint returns a deterministic SHA-256 over the definition's
// execution-relevant fields. Two definitions with the same execution semantics
// produce the same fingerprint regardless of TOML key order; any material
// change produces a different one.
func (d Definition) ExecutionFingerprint() string {
	ident := executionIdentity{
		Version:           executionIdentityVersion,
		Executables:       d.Executables,
		Bridges:           d.Bridges,
		WellKnown:         d.WellKnown,
		VersionArgs:       d.VersionArgs,
		ACP:               d.ACP,
		ACPArgs:           d.ACPArgs,
		BridgeArgs:        d.BridgeArgs,
		InterposeCommands: d.InterposeCommands,
		ModelSelection:    d.ModelSelection,
		Platforms:         d.Platforms,
	}
	b, err := json.Marshal(ident)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// CatalogDiagnostic is a per-entry catalog problem. Non-fatal diagnostics leave
// the remaining definitions usable.
type CatalogDiagnostic struct {
	Source  string
	ID      string
	Field   string
	Message string
}

func (d CatalogDiagnostic) String() string {
	loc := d.Source
	if d.ID != "" {
		loc += ":" + d.ID
	}
	if d.Field != "" {
		loc += "." + d.Field
	}
	return loc + ": " + d.Message
}

// Catalog is the effective set of definitions (shipped merged with user).
type Catalog struct {
	SchemaVersion int
	Definitions   []Definition
	Diagnostics   []CatalogDiagnostic
}

// ByID returns the effective definition with the given id.
func (c *Catalog) ByID(id string) (Definition, bool) {
	for _, d := range c.Definitions {
		if d.ID == id {
			return d, true
		}
	}
	return Definition{}, false
}

// EnabledForPlatform returns enabled definitions that apply to goos.
func (c *Catalog) EnabledForPlatform(goos string) []Definition {
	var out []Definition
	for _, d := range c.Definitions {
		if !d.Enabled {
			continue
		}
		if !platformAllowed(d.Platforms, goos) {
			continue
		}
		out = append(out, d)
	}
	return out
}

// UserCatalogPath returns the platform-appropriate user catalog path.
func UserCatalogPath() string {
	return filepath.Join(configDir(), "wayshard", "harnesses.toml")
}

func configDir() string {
	home, _ := os.UserHomeDir()
	switch runtime.GOOS {
	case "windows":
		if app := os.Getenv("APPDATA"); app != "" {
			return app
		}
		return filepath.Join(home, "AppData", "Roaming")
	case "darwin":
		return filepath.Join(home, "Library", "Application Support")
	default:
		if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
			return xdg
		}
		return filepath.Join(home, ".config")
	}
}

// LoadCatalog builds the effective catalog from the shipped defaults and the
// user catalog at userPath. An empty userPath uses the platform default. A
// missing user file is not an error.
func LoadCatalog(userPath string) (*Catalog, error) {
	if userPath == "" {
		userPath = UserCatalogPath()
	}
	b, err := os.ReadFile(userPath)
	if err != nil {
		if os.IsNotExist(err) {
			return buildCatalog(shippedCatalogTOML, "")
		}
		return nil, fmt.Errorf("user harness catalog %s: %w", userPath, err)
	}
	if len(b) > maxUserCatalogBytes {
		return nil, fmt.Errorf("user harness catalog %s: %d bytes exceeds the %d-byte limit", userPath, len(b), maxUserCatalogBytes)
	}
	return buildCatalog(shippedCatalogTOML, string(b))
}

// ShippedCatalog returns the shipped defaults with no user overrides.
func ShippedCatalog() *Catalog {
	cat, err := buildCatalog(shippedCatalogTOML, "")
	if err != nil {
		return &Catalog{SchemaVersion: CatalogSchemaVersion}
	}
	return cat
}

func buildCatalog(shippedText, userText string) (*Catalog, error) {
	shipped, err := parseCatalogSource("shipped", shippedText)
	if err != nil {
		return nil, fmt.Errorf("shipped harness catalog: %w", err)
	}
	cat := &Catalog{SchemaVersion: CatalogSchemaVersion}
	shippedByID := map[string]map[string]any{}
	for _, e := range shipped {
		if _, dup := shippedByID[e.id]; dup {
			cat.Diagnostics = append(cat.Diagnostics, CatalogDiagnostic{Source: "shipped", ID: e.id, Message: "duplicate id in shipped catalog; keeping first"})
			continue
		}
		shippedByID[e.id] = e.fields
	}

	userByID := map[string]map[string]any{}
	userOrder := []string{}
	if userText != "" {
		user, perr := parseCatalogSource("user", userText)
		if perr != nil {
			return nil, fmt.Errorf("user harness catalog: %w", perr)
		}
		for _, e := range user {
			if _, dup := userByID[e.id]; dup {
				cat.Diagnostics = append(cat.Diagnostics, CatalogDiagnostic{Source: "user", ID: e.id, Message: "duplicate id in user catalog; keeping first"})
				continue
			}
			userByID[e.id] = e.fields
			userOrder = append(userOrder, e.id)
		}
	}

	seen := map[string]bool{}
	for _, e := range shipped {
		fields, ok := shippedByID[e.id]
		if !ok {
			continue
		}
		src := SourceShipped
		var userKeys map[string]bool
		if user, has := userByID[e.id]; has {
			fields = mergeFields(fields, user)
			src = SourceOverridden
			userKeys = keysOf(user)
		}
		def, diags := decodeDefinition(e.id, fields, src, userKeys)
		cat.Diagnostics = append(cat.Diagnostics, diags...)
		if diags == nil {
			cat.Definitions = append(cat.Definitions, def)
		}
		seen[e.id] = true
	}
	for _, id := range userOrder {
		if seen[id] {
			continue
		}
		def, diags := decodeDefinition(id, userByID[id], SourceUser, keysOf(userByID[id]))
		cat.Diagnostics = append(cat.Diagnostics, diags...)
		if diags == nil {
			cat.Definitions = append(cat.Definitions, def)
		}
	}
	return cat, nil
}

func keysOf(m map[string]any) map[string]bool {
	out := make(map[string]bool, len(m))
	for k := range m {
		out[k] = true
	}
	return out
}

type rawEntry struct {
	id     string
	fields map[string]any
}

type rawFile struct {
	SchemaVersion *int             `toml:"schema_version"`
	Harness       []map[string]any `toml:"harness"`
}

func parseCatalogSource(source, text string) ([]rawEntry, error) {
	var rf rawFile
	md, err := toml.Decode(text, &rf)
	if err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	if rf.SchemaVersion == nil {
		return nil, fmt.Errorf("missing schema_version (expected %d)", CatalogSchemaVersion)
	}
	if *rf.SchemaVersion != CatalogSchemaVersion {
		return nil, fmt.Errorf("unsupported schema_version %d (expected %d)", *rf.SchemaVersion, CatalogSchemaVersion)
	}
	if len(rf.Harness) > maxCatalogDefinitions {
		return nil, fmt.Errorf("catalog defines %d harnesses, exceeding the %d limit", len(rf.Harness), maxCatalogDefinitions)
	}
	_ = md
	var out []rawEntry
	for i, h := range rf.Harness {
		id, _ := h["id"].(string)
		if strings.TrimSpace(id) == "" {
			return nil, fmt.Errorf("harness[%d]: missing id", i)
		}
		out = append(out, rawEntry{id: id, fields: h})
	}
	return out, nil
}

// mergeFields overrides base keys with override keys (shallow; arrays replace).
func mergeFields(base, override map[string]any) map[string]any {
	out := make(map[string]any, len(base)+len(override))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range override {
		out[k] = v
	}
	return out
}

var (
	idRe        = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)
	execNameRe  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+-]*$`)
	forbidden   = map[string]struct{}{"npx": {}, "npm": {}, "yarn": {}, "pnpm": {}, "bun": {}, "bunx": {}, "deno": {}, "pipx": {}, "uvx": {}}
	validACP    = map[string]struct{}{"native": {}, "bridge": {}}
	validModel  = map[string]struct{}{"none": {}, "config_option": {}, "set_model": {}}
	validPlatfs = map[string]struct{}{"linux": {}, "darwin": {}, "windows": {}}
)

func decodeDefinition(id string, m map[string]any, source DefinitionSource, userKeys map[string]bool) (Definition, []CatalogDiagnostic) {
	d := Definition{ID: id, Enabled: true, Source: source, ModelSelection: "none"}
	var diags []CatalogDiagnostic
	fail := func(field, msg string) {
		diags = append(diags, CatalogDiagnostic{Source: string(source), ID: id, Field: field, Message: msg})
	}
	if !idRe.MatchString(id) {
		fail("id", "must match ^[a-z0-9][a-z0-9-]*$")
	}
	for k, v := range m {
		switch k {
		case "id":
			// already handled
		case "enabled":
			b, ok := v.(bool)
			if !ok {
				fail(k, "must be a boolean")
				continue
			}
			d.Enabled = b
		case "display_name":
			d.DisplayName = asString(v, k, fail)
		case "homepage":
			d.Homepage = asString(v, k, fail)
		case "platforms":
			d.Platforms = asStringList(v, k, fail)
		case "executables":
			d.Executables = asStringList(v, k, fail)
		case "bridges":
			d.Bridges = asStringList(v, k, fail)
		case "well_known":
			d.WellKnown = asStringList(v, k, fail)
		case "version_args":
			d.VersionArgs = asStringList(v, k, fail)
		case "acp":
			d.ACP = asString(v, k, fail)
		case "acp_args":
			d.ACPArgs = asStringList(v, k, fail)
		case "bridge_args":
			d.BridgeArgs = asStringList(v, k, fail)
		case "interpose_commands":
			d.InterposeCommands = asBool(v, k, fail)
		case "model_selection":
			d.ModelSelection = asString(v, k, fail)
		default:
			fail(k, "unknown field")
		}
	}
	if diags != nil {
		return Definition{}, diags
	}
	// Bounds.
	for _, l := range []struct {
		field string
		lim   int
		list  []string
	}{
		{"executables", maxListEntries, d.Executables},
		{"bridges", maxListEntries, d.Bridges},
		{"well_known", maxListEntries, d.WellKnown},
		{"version_args", maxListEntries, d.VersionArgs},
		{"acp_args", maxArgEntries, d.ACPArgs},
		{"bridge_args", maxArgEntries, d.BridgeArgs},
		{"platforms", maxListEntries, d.Platforms},
	} {
		if len(l.list) > l.lim {
			fail(l.field, fmt.Sprintf("has %d entries, exceeding the %d limit", len(l.list), l.lim))
		}
	}
	// Executable validation: bare names only, never package-runner launchers
	// (Wayshard never installs harnesses).
	for _, name := range append(append([]string{}, d.Executables...), d.Bridges...) {
		if !execNameRe.MatchString(name) {
			fail("executables", fmt.Sprintf("invalid executable name %q", name))
		}
		if _, bad := forbidden[strings.ToLower(name)]; bad {
			fail("executables", fmt.Sprintf("refusing package-runner launcher %q (Wayshard never installs harnesses)", name))
		}
	}
	// Well-known discovery dirs must be home-relative and contain no traversal.
	for _, root := range d.WellKnown {
		if err := validateRootSyntax(root); err != nil {
			fail("well_known", err.Error())
		}
	}
	if d.ACP == "" {
		if len(d.Bridges) > 0 {
			d.ACP = "bridge"
		} else {
			d.ACP = "native"
		}
	}
	if _, ok := validACP[d.ACP]; !ok {
		fail("acp", fmt.Sprintf("must be one of native|bridge (got %q)", d.ACP))
	}
	if _, ok := validModel[d.ModelSelection]; !ok {
		fail("model_selection", fmt.Sprintf("must be one of none|config_option|set_model (got %q)", d.ModelSelection))
	}
	for _, p := range d.Platforms {
		if _, ok := validPlatfs[p]; !ok {
			fail("platforms", fmt.Sprintf("unsupported platform %q", p))
		}
	}
	if d.ACP == "bridge" && len(d.Bridges) == 0 {
		fail("bridges", "acp = \"bridge\" requires at least one bridge executable")
	}
	if d.Enabled && len(d.Executables) == 0 && len(d.Bridges) == 0 {
		fail("executables", "enabled definition requires at least one executable or bridge")
	}
	if diags != nil {
		return Definition{}, diags
	}
	return d, nil
}

func asString(v any, field string, fail func(string, string)) string {
	s, ok := v.(string)
	if !ok {
		fail(field, "must be a string")
		return ""
	}
	return s
}

func asBool(v any, field string, fail func(string, string)) bool {
	b, ok := v.(bool)
	if !ok {
		fail(field, "must be a boolean")
		return false
	}
	return b
}

func asStringList(v any, field string, fail func(string, string)) []string {
	arr, ok := v.([]any)
	if !ok {
		fail(field, "must be a list of strings")
		return nil
	}
	var out []string
	for i, e := range arr {
		s, ok := e.(string)
		if !ok {
			fail(field, fmt.Sprintf("element %d must be a string", i))
			return nil
		}
		if strings.ContainsAny(s, "\x00\n\r") {
			fail(field, fmt.Sprintf("element %d contains control characters", i))
			return nil
		}
		out = append(out, s)
	}
	return out
}

// sensitiveRootComponents are home-relative top-level directories that must
// never be exposed to a harness through a user-supplied catalog root.
var sensitiveRootComponents = map[string]struct{}{
	".ssh": {}, ".aws": {}, ".gnupg": {}, ".gpg": {}, ".kube": {}, ".docker": {},
	".netrc": {}, ".password-store": {}, ".mozilla": {}, ".pki": {}, ".m2": {},
	".gradle": {}, ".git-credentials": {}, ".gitconfig": {}, ".npmrc": {},
	".bashrc": {}, ".bash_profile": {}, ".profile": {}, ".zshrc": {}, ".bash_history": {},
}

// platformConfigBases and the sensitive-root policy were part of the removed
// containment model. Only the home-relative syntax check remains, because
// discovery search directories must stay inside HOME.

func firstComponent(p string) string {
	return strings.Split(filepath.ToSlash(p), "/")[0]
}

func sensitiveRel(rel string) bool {
	_, bad := sensitiveRootComponents[firstComponent(rel)]
	return bad
}

// validateRootSyntax rejects empty, absolute, traversal and home-root paths.
func validateRootSyntax(p string) error {
	if p == "" {
		return fmt.Errorf("empty path")
	}
	if filepath.IsAbs(p) || strings.HasPrefix(p, "/") || strings.HasPrefix(p, "\\") {
		return fmt.Errorf("path %q must be home-relative, not absolute", p)
	}
	if len(p) >= 2 && p[1] == ':' {
		return fmt.Errorf("path %q must not contain a drive letter", p)
	}
	clean := filepath.ToSlash(filepath.Clean(p))
	if clean == "." || clean == ".." || clean == "/" || strings.HasPrefix(clean, "../") {
		return fmt.Errorf("path %q must name a location inside the home directory", p)
	}
	for _, comp := range strings.Split(filepath.ToSlash(p), "/") {
		if comp == ".." {
			return fmt.Errorf("path %q must not contain '..'", p)
		}
	}
	return nil
}

// resolveRootSafe canonicalizes a home-relative root without following a
// user-controlled symlink out of the home directory. It returns the resolved
// absolute path. Non-existent components are permitted (a harness may create
// them later).
func resolveRootSafe(home, r string) (string, error) {
	if home == "" {
		return "", fmt.Errorf("home is unknown")
	}
	// Canonicalize the home base once: platform temp/home paths may themselves
	// contain symlinks or short names (for example macOS /var -> /private/var).
	base := home
	if rp, err := filepath.EvalSymlinks(home); err == nil {
		base = rp
	}
	p := filepath.Join(base, r)
	rel, err := filepath.Rel(base, p)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("root %q escapes the home directory", r)
	}
	cur := base
	for _, comp := range strings.Split(filepath.ToSlash(r), "/") {
		if comp == "" || comp == "." {
			continue
		}
		next := filepath.Join(cur, comp)
		fi, lerr := os.Lstat(next)
		if lerr != nil {
			if os.IsNotExist(lerr) {
				return next, nil
			}
			return "", lerr
		}
		if fi.Mode()&os.ModeSymlink != 0 {
			target, terr := filepath.EvalSymlinks(next)
			if terr != nil {
				return "", terr
			}
			trel, rerr := filepath.Rel(base, target)
			if rerr != nil || trel == ".." || strings.HasPrefix(trel, ".."+string(os.PathSeparator)) {
				return "", fmt.Errorf("symlink %q escapes the home directory", next)
			}
			if sensitiveRel(trel) {
				return "", fmt.Errorf("symlink %q resolves to a sensitive location", next)
			}
			cur = target
			continue
		}
		cur = next
	}
	if finalRel, ferr := filepath.Rel(base, cur); ferr == nil && sensitiveRel(finalRel) {
		return "", fmt.Errorf("root %q resolves to a sensitive location", r)
	}
	return cur, nil
}

func platformAllowed(platforms []string, goos string) bool {
	if len(platforms) == 0 {
		return true
	}
	for _, p := range platforms {
		if p == goos {
			return true
		}
	}
	return false
}
