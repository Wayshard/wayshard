package harness

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/Wayshard/wayshard/internal/domain"
)

//go:embed harnesses.toml
var shippedCatalogTOML string

// CatalogSchemaVersion is the supported harness catalog schema version.
const CatalogSchemaVersion = 1

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

	ACP                 string // native|bridge
	ACPArgs             []string
	BridgeArgs          []string
	ACPRequiresLoopback bool
	InterposeCommands   bool
	ModelSelection      string // none|config_option|set_model

	ConfigRoots []string

	RequiresProviderNetwork bool
	DeclaredTransport       string // none|http_proxy|unknown (declared requirement)

	Source DefinitionSource
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

// VerifiedTransport returns Wayshard's evidence-based provider transport for a
// definition. It is deliberately Wayshard-owned and not user-configurable: a
// user catalog entry cannot mark an unverified transport as trusted. Unknown
// definitions fail closed (TransportUnknown).
func VerifiedTransport(defID string) domain.ProviderTransport {
	if t, ok := verifiedProviderTransports[defID]; ok {
		return t
	}
	return domain.TransportUnknown
}

// verifiedProviderTransports lists harness definitions whose real provider
// traffic has been observed traversing the secure broker. It is a verification
// record, not a discovery catalog: adding a harness to the discovery catalog
// does not grant it provider transport trust.
var verifiedProviderTransports = map[string]domain.ProviderTransport{
	"opencode":          domain.TransportHTTPProxy,
	"codex":             domain.TransportHTTPProxy,
	"wayshard-fake-acp": domain.TransportHTTPProxy, // deterministic fixture
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
		if user, has := userByID[e.id]; has {
			fields = mergeFields(fields, user)
			src = SourceOverridden
		}
		def, diags := decodeDefinition(e.id, fields, src)
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
		def, diags := decodeDefinition(id, userByID[id], SourceUser)
		cat.Diagnostics = append(cat.Diagnostics, diags...)
		if diags == nil {
			cat.Definitions = append(cat.Definitions, def)
		}
	}
	return cat, nil
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
	validTrans  = map[string]struct{}{"none": {}, "http_proxy": {}, "unknown": {}}
	validPlatfs = map[string]struct{}{"linux": {}, "darwin": {}, "windows": {}}
)

func decodeDefinition(id string, m map[string]any, source DefinitionSource) (Definition, []CatalogDiagnostic) {
	d := Definition{ID: id, Enabled: true, Source: source, ModelSelection: "none", DeclaredTransport: "unknown"}
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
		case "acp_requires_loopback":
			d.ACPRequiresLoopback = asBool(v, k, fail)
		case "interpose_commands":
			d.InterposeCommands = asBool(v, k, fail)
		case "model_selection":
			d.ModelSelection = asString(v, k, fail)
		case "config_roots":
			d.ConfigRoots = asStringList(v, k, fail)
		case "requires_provider_network":
			d.RequiresProviderNetwork = asBool(v, k, fail)
		case "transport":
			d.DeclaredTransport = asString(v, k, fail)
		default:
			fail(k, "unknown field")
		}
	}
	if diags != nil {
		return Definition{}, diags
	}
	// Security and consistency validation.
	for _, name := range append(append([]string{}, d.Executables...), d.Bridges...) {
		if !execNameRe.MatchString(name) {
			fail("executables", fmt.Sprintf("invalid executable name %q", name))
		}
		if _, bad := forbidden[strings.ToLower(name)]; bad {
			fail("executables", fmt.Sprintf("refusing package-runner launcher %q (Wayshard never installs harnesses)", name))
		}
	}
	for _, root := range d.WellKnown {
		if err := validateHomeRelative(root); err != nil {
			fail("well_known", err.Error())
		}
	}
	for _, root := range d.ConfigRoots {
		if err := validateHomeRelative(root); err != nil {
			fail("config_roots", err.Error())
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
	if _, ok := validTrans[d.DeclaredTransport]; !ok {
		fail("transport", fmt.Sprintf("must be one of none|http_proxy|unknown (got %q)", d.DeclaredTransport))
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

// validateHomeRelative ensures a catalog path stays under the user's home and
// cannot point at an arbitrary host root.
func validateHomeRelative(p string) error {
	if p == "" {
		return fmt.Errorf("empty path")
	}
	if filepath.IsAbs(p) || strings.HasPrefix(p, "/") || strings.HasPrefix(p, "\\") {
		return fmt.Errorf("path %q must be home-relative, not absolute", p)
	}
	if strings.Contains(p, "..") {
		return fmt.Errorf("path %q must not contain '..'", p)
	}
	if len(p) >= 2 && p[1] == ':' {
		return fmt.Errorf("path %q must not contain a drive letter", p)
	}
	return nil
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
