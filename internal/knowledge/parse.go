package knowledge

import (
	"path"
	"regexp"
	"strings"
)

var (
	reATXHeading = regexp.MustCompile(`^#{1,6}[ \t]+(.+?)(?:[ \t]+#+)?[ \t]*$`)
	reInlineLink = regexp.MustCompile(`\[([^\]]*)\]\(([^)]+)\)`)
	reRefDef     = regexp.MustCompile(`^\[([^\]]+)\]:\s*(<[^>]+>|\S+)`)
	reRefUse     = regexp.MustCompile(`\[([^\]]+)\]\[([^\]]*)\]`)
	reEntryCmt   = regexp.MustCompile(`(?i)<!--\s*wayshard-entry:\s*([^>]+?)\s*-->`)
	reMetaCmt    = regexp.MustCompile(`(?i)<!--\s*wayshard:([a-z0-9_-]+)\s*=\s*([^>]+?)\s*-->`)
)

type parsedMeta struct {
	Kind        Kind
	Family      Family
	Domain      AuthorityDomain
	Scope       string
	AlwaysApply bool
	Globs       []string
	Entrypoints []string
	Explicit    bool
}

type parsedFile struct {
	Meta     parsedMeta
	Sections []rawSection
	Links    []string
}

type rawSection struct {
	Heading string
	Body    string
}

// HydrateRuntime restores glob/always-apply flags from stored document content.
func HydrateRuntime(d *Document) {
	if d == nil || d.Content == "" {
		return
	}
	parsed := parseKnowledgeFile(d.Path, d.Content)
	if len(d.Globs) == 0 {
		d.Globs = parsed.Meta.Globs
	}
	if parsed.Meta.AlwaysApply {
		d.AlwaysApply = true
	}
	if d.Kind == KindCopilot && NormalizeRel(d.Path) == ".github/copilot-instructions.md" {
		d.AlwaysApply = true
	}
	if d.Kind == KindCursorRules && strings.EqualFold(path.Base(d.Path), ".cursorrules") {
		d.AlwaysApply = true
	}
	if parsed.Meta.Explicit {
		d.Explicit = true
	}
}

func parseKnowledgeFile(_ string, content string) parsedFile {
	meta, rest := parseFrontmatter(content)
	applyComments(&meta, content)
	links := extractLinks(rest)
	links = append(links, meta.Entrypoints...)
	return parsedFile{
		Meta:     meta,
		Sections: parseSections(rest),
		Links:    uniqueStable(links),
	}
}

func parseFrontmatter(content string) (parsedMeta, string) {
	var meta parsedMeta
	rest := content
	if !strings.HasPrefix(content, "---\n") && !strings.HasPrefix(content, "---\r\n") {
		return meta, content
	}
	nl := "\n"
	body := content[4:]
	if strings.HasPrefix(content, "---\r\n") {
		body = content[5:]
		nl = "\r\n"
	}
	end := strings.Index(body, nl+"---")
	if end < 0 {
		return meta, content
	}
	fm := body[:end]
	rest = strings.TrimLeft(body[end+len(nl+"---"):], "\r\n")
	vals, lists := parseYAMLMap(fm)
	meta.Explicit = applyYAMLMeta(&meta, vals, lists)
	return meta, rest
}

func applyYAMLMeta(meta *parsedMeta, vals map[string]string, lists map[string][]string) bool {
	explicit := false
	get := func(keys ...string) string {
		for _, k := range keys {
			if v, ok := vals[k]; ok && v != "" {
				return v
			}
		}
		return ""
	}
	if v := get("kind", "wayshard.kind"); v != "" {
		if k := ParseKind(v); k != "" {
			meta.Kind = k
			explicit = true
		}
	}
	if v := get("family", "wayshard.family"); v != "" {
		if f := ParseFamily(v); f != "" {
			meta.Family = f
			explicit = true
		}
	}
	if v := get("authority", "authority_domain", "authorityDomain", "wayshard.authority"); v != "" {
		d := ParseDomain(v)
		if d != DomainUnknown || strings.EqualFold(v, "unknown") {
			meta.Domain = d
			explicit = true
		}
	}
	if v := get("scope", "wayshard.scope"); v != "" {
		meta.Scope = v
		explicit = true
	}
	if v := get("alwaysapply", "always_apply", "alwaysApply"); v != "" {
		meta.AlwaysApply = isTruthy(v)
		explicit = true
	}
	meta.Globs = append(meta.Globs, splitCSV(get("globs", "glob", "applyto", "apply_to", "applyTo"))...)
	for _, key := range []string{"globs", "glob", "applyto", "apply_to"} {
		meta.Globs = append(meta.Globs, lists[key]...)
	}
	for _, key := range []string{"entrypoints", "entry_points"} {
		meta.Entrypoints = append(meta.Entrypoints, lists[key]...)
	}
	meta.Entrypoints = append(meta.Entrypoints, splitCSV(get("entrypoints", "entry_points"))...)
	if len(meta.Globs) > 0 || len(meta.Entrypoints) > 0 {
		explicit = true
	}
	meta.Globs = uniqueStable(meta.Globs)
	meta.Entrypoints = uniqueStable(meta.Entrypoints)
	return explicit
}

func applyComments(meta *parsedMeta, content string) {
	for _, m := range reEntryCmt.FindAllStringSubmatch(content, -1) {
		p := strings.TrimSpace(m[1])
		if p != "" {
			meta.Entrypoints = append(meta.Entrypoints, p)
			meta.Explicit = true
		}
	}
	for _, m := range reMetaCmt.FindAllStringSubmatch(content, -1) {
		key := strings.ToLower(strings.TrimSpace(m[1]))
		val := strings.TrimSpace(m[2])
		switch key {
		case "kind":
			if k := ParseKind(val); k != "" {
				meta.Kind = k
				meta.Explicit = true
			}
		case "family":
			if f := ParseFamily(val); f != "" {
				meta.Family = f
				meta.Explicit = true
			}
		case "authority", "domain":
			meta.Domain = ParseDomain(val)
			meta.Explicit = true
		case "scope":
			meta.Scope = val
			meta.Explicit = true
		case "entry", "entrypoint":
			meta.Entrypoints = append(meta.Entrypoints, val)
			meta.Explicit = true
		}
	}
	meta.Entrypoints = uniqueStable(meta.Entrypoints)
}

func parseYAMLMap(fm string) (map[string]string, map[string][]string) {
	vals := map[string]string{}
	lists := map[string][]string{}
	prefix := ""
	currentList := ""
	for _, line := range strings.Split(fm, "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" || strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		trimmed := strings.TrimSpace(line)
		indent := len(line) - len(strings.TrimLeft(line, " \t"))
		if indent > 0 && strings.HasPrefix(trimmed, "- ") {
			item := strings.TrimSpace(strings.TrimPrefix(trimmed, "- "))
			item = strings.Trim(item, `"'`)
			key := currentList
			if prefix != "" && key == "" {
				key = prefix
			}
			if key != "" {
				lists[key] = append(lists[key], item)
			}
			continue
		}
		key, val, ok := splitYAMLKV(trimmed)
		if !ok {
			continue
		}
		if indent == 0 {
			prefix = ""
			currentList = ""
		}
		full := key
		if indent > 0 && prefix != "" {
			full = prefix + "." + key
		}
		if val == "" {
			if indent == 0 {
				prefix = key
				currentList = key
			} else {
				currentList = full
			}
			continue
		}
		if indent == 0 {
			prefix = ""
			currentList = ""
		}
		vals[strings.ToLower(full)] = val
		vals[strings.ToLower(key)] = val
	}
	return vals, lists
}

func splitYAMLKV(line string) (string, string, bool) {
	i := strings.IndexByte(line, ':')
	if i <= 0 {
		return "", "", false
	}
	key := strings.TrimSpace(line[:i])
	val := strings.TrimSpace(line[i+1:])
	val = strings.Trim(val, `"'`)
	if key == "" {
		return "", "", false
	}
	return key, val, true
}

func parseSections(content string) []rawSection {
	var sections []rawSection
	heading := ""
	var body []string
	inFence := false
	flush := func() {
		b := strings.TrimRight(strings.Join(body, "\n"), "\n")
		if heading == "" && strings.TrimSpace(b) == "" {
			body = nil
			return
		}
		sections = append(sections, rawSection{Heading: heading, Body: b})
		body = nil
	}
	for _, line := range strings.Split(content, "\n") {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "```") || strings.HasPrefix(trim, "~~~") {
			inFence = !inFence
			body = append(body, line)
			continue
		}
		if !inFence {
			if m := reATXHeading.FindStringSubmatch(line); m != nil {
				flush()
				heading = strings.TrimSpace(m[1])
				continue
			}
		}
		body = append(body, line)
	}
	flush()
	return sections
}

func extractLinks(content string) []string {
	var links []string
	defs := map[string]string{}
	inFence := false
	for _, line := range strings.Split(content, "\n") {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "```") || strings.HasPrefix(trim, "~~~") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		if m := reRefDef.FindStringSubmatch(line); m != nil {
			defs[strings.ToLower(m[1])] = cleanLink(m[2])
			continue
		}
		for _, m := range reInlineLink.FindAllStringSubmatchIndex(line, -1) {
			if m[0] > 0 && line[m[0]-1] == '!' {
				continue
			}
			raw := line[m[4]:m[5]]
			if l := cleanLink(raw); l != "" && !isExternalLink(l) {
				links = append(links, l)
			}
		}
		for _, m := range reRefUse.FindAllStringSubmatch(line, -1) {
			label := m[2]
			if label == "" {
				label = m[1]
			}
			if dest, ok := defs[strings.ToLower(label)]; ok && dest != "" && !isExternalLink(dest) {
				links = append(links, dest)
			}
		}
	}
	return uniqueStable(links)
}

func cleanLink(raw string) string {
	raw = strings.TrimSpace(raw)
	if i := strings.IndexAny(raw, " \t"); i >= 0 {
		raw = raw[:i]
	}
	raw = strings.Trim(raw, "<>")
	if i := strings.Index(raw, "#"); i >= 0 {
		raw = raw[:i]
	}
	return strings.TrimSpace(raw)
}

func isExternalLink(link string) bool {
	l := strings.ToLower(link)
	if l == "" {
		return true
	}
	if strings.Contains(l, "://") {
		return true
	}
	if strings.HasPrefix(l, "mailto:") || strings.HasPrefix(l, "tel:") || strings.HasPrefix(l, "data:") {
		return true
	}
	return false
}

func resolveLink(fromRel, link string) (string, bool) {
	link = cleanLink(link)
	if link == "" || isExternalLink(link) {
		return "", false
	}
	link = strings.ReplaceAll(link, "\\", "/")
	if strings.HasPrefix(link, "/") {
		return NormalizeRel(link), true
	}
	base := path.Dir(NormalizeRel(fromRel))
	joined := path.Clean(path.Join(base, link))
	joined = NormalizeRel(joined)
	if joined == "" {
		return "", false
	}
	if strings.HasPrefix(joined, "../") || joined == ".." {
		return "", false
	}
	return joined, true
}

func uniqueStable(in []string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	var out []string
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func isTruthy(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func matchAnyGlob(globs []string, filePath string) bool {
	filePath = NormalizeRel(filePath)
	for _, g := range globs {
		g = strings.TrimSpace(g)
		if g == "" {
			continue
		}
		p, ok := parseIgnoreLine(g)
		if ok && p.matchGlob(filePath) {
			return true
		}
		base := path.Base(filePath)
		if p, ok := parseIgnoreLine(g); ok && p.matchGlob(base) {
			return true
		}
	}
	return false
}

func isBinary(b []byte) bool {
	n := len(b)
	if n > 8000 {
		n = 8000
	}
	for i := 0; i < n; i++ {
		if b[i] == 0 {
			return true
		}
	}
	return false
}
