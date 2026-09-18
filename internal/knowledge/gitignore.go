package knowledge

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var defaultSkipDirs = map[string]struct{}{
	".git": {}, "node_modules": {}, "vendor": {}, ".venv": {}, "venv": {},
	"__pycache__": {}, "target": {}, "dist": {}, "build": {}, ".next": {},
	".nuxt": {}, ".tox": {}, ".mypy_cache": {}, ".pytest_cache": {},
	".gradle": {}, ".idea": {},
}

type ignorePattern struct {
	negate  bool
	dirOnly bool
	re      *regexp.Regexp
	raw     string
}

type ignoreCache struct {
	byDir map[string][]ignorePattern
}

func newIgnoreCache() *ignoreCache {
	return &ignoreCache{byDir: make(map[string][]ignorePattern)}
}

func (c *ignoreCache) loadDir(absDir, relDir string) {
	data, err := os.ReadFile(filepath.Join(absDir, ".gitignore"))
	if err != nil {
		c.byDir[relDir] = nil
		return
	}
	c.byDir[relDir] = parseGitignore(string(data))
}

func parseGitignore(body string) []ignorePattern {
	var out []ignorePattern
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		p, ok := parseIgnoreLine(line)
		if ok {
			out = append(out, p)
		}
	}
	return out
}

func parseIgnoreLine(line string) (ignorePattern, bool) {
	p := ignorePattern{raw: line}
	if strings.HasPrefix(line, "!") {
		p.negate = true
		line = line[1:]
	}
	if line == "" {
		return ignorePattern{}, false
	}
	if strings.HasSuffix(line, "/") {
		p.dirOnly = true
		line = strings.TrimSuffix(line, "/")
	}
	anchored := strings.HasPrefix(line, "/") || strings.Contains(line, "/")
	if strings.HasPrefix(line, "/") {
		line = line[1:]
		anchored = true
	}
	if line == "" {
		return ignorePattern{}, false
	}
	re, err := regexp.Compile(gitGlobToRegex(line, !anchored))
	if err != nil {
		return ignorePattern{}, false
	}
	p.re = re
	return p, true
}

func gitGlobToRegex(glob string, unanchored bool) string {
	var b strings.Builder
	b.WriteString("^")
	if unanchored && !strings.HasPrefix(glob, "**") {
		b.WriteString("(?:.*/)?")
	}
	i := 0
	for i < len(glob) {
		switch {
		case strings.HasPrefix(glob[i:], "**/"):
			b.WriteString("(?:.*/)?")
			i += 3
		case strings.HasPrefix(glob[i:], "**"):
			b.WriteString(".*")
			i += 2
		case glob[i] == '*':
			b.WriteString("[^/]*")
			i++
		case glob[i] == '?':
			b.WriteString("[^/]")
			i++
		default:
			b.WriteString(regexp.QuoteMeta(string(glob[i])))
			i++
		}
	}
	b.WriteString("$")
	return b.String()
}

func (c *ignoreCache) ignored(rel string, isDir bool) bool {
	rel = NormalizeRel(rel)
	if rel == "" {
		return false
	}
	result := false
	result = applyPatterns(result, c.byDir[""], rel, isDir)
	parts := strings.Split(rel, "/")
	dir := ""
	for i := 0; i < len(parts)-1; i++ {
		if dir == "" {
			dir = parts[i]
		} else {
			dir += "/" + parts[i]
		}
		sub := strings.Join(parts[i+1:], "/")
		result = applyPatterns(result, c.byDir[dir], sub, isDir)
	}
	return result
}

func applyPatterns(current bool, pats []ignorePattern, path string, isDir bool) bool {
	for _, p := range pats {
		if p.dirOnly && !isDir {
			continue
		}
		if p.re != nil && p.re.MatchString(path) {
			current = !p.negate
		}
	}
	return current
}

func (p ignorePattern) matchGlob(name string) bool {
	if p.re == nil {
		return false
	}
	return p.re.MatchString(name)
}
