package knowledge

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Wayshard/wayshard/internal/crypto"
	"github.com/Wayshard/wayshard/internal/id"
)

const (
	defaultMaxFileBytes  = 512 << 10
	defaultMaxFiles      = 2000
	defaultMaxWalk       = 100000
	defaultMaxDepth      = 24
	defaultMaxLinkDepth  = 8
	defaultMaxReferences = 250
	defaultMaxDocsSeeds  = 200
)

// Writer persists a discovered knowledge index. Implemented by storage.Store.
type Writer interface {
	ReplaceProjectKnowledge(ctx context.Context, projectID string, idx *Index) error
}

// Discoverer walks a repository without executing project code or writing files.
type Discoverer struct {
	MaxFileBytes  int
	MaxFiles      int
	MaxWalk       int
	MaxDepth      int
	MaxLinkDepth  int
	MaxReferences int
	MaxDocsSeeds  int
}

func NewDiscoverer() *Discoverer {
	return &Discoverer{
		MaxFileBytes:  defaultMaxFileBytes,
		MaxFiles:      defaultMaxFiles,
		MaxWalk:       defaultMaxWalk,
		MaxDepth:      defaultMaxDepth,
		MaxLinkDepth:  defaultMaxLinkDepth,
		MaxReferences: defaultMaxReferences,
		MaxDocsSeeds:  defaultMaxDocsSeeds,
	}
}

// Index is the discovered knowledge graph for one repository snapshot.
type Index struct {
	Root      string
	Documents []Document
	Sections  []Section
	Edges     []Edge
	Cycles    []Cycle
	Conflicts []ConflictWarning
	Revision  string
	Stats     Stats
}

type Stats struct {
	FilesVisited int
	DocsIndexed  int
	BrokenRefs   int
	Cycles       int
	Truncated    int
	Ignored      int
}

func (idx *Index) DocumentByPath(rel string) *Document {
	rel = NormalizeRel(rel)
	for i := range idx.Documents {
		if idx.Documents[i].Path == rel {
			return &idx.Documents[i]
		}
	}
	return nil
}

func (idx *Index) BrokenEdges() []Edge {
	var out []Edge
	for _, e := range idx.Edges {
		if e.Broken {
			out = append(out, e)
		}
	}
	return out
}

func (idx *Index) SixCanonicalPresent() []string {
	var out []string
	have := map[string]struct{}{}
	for _, d := range idx.Documents {
		if d.Canonical {
			base := pathBase(d.Path)
			have[strings.ToLower(base)] = struct{}{}
		}
	}
	for _, name := range SixCanonicalNames {
		if _, ok := have[strings.ToLower(name)]; ok {
			out = append(out, name)
		}
	}
	return out
}

func pathBase(rel string) string {
	i := strings.LastIndex(rel, "/")
	if i < 0 {
		return rel
	}
	return rel[i+1:]
}

// Discover indexes repository-owned knowledge. It never mutates the repository.
func Discover(ctx context.Context, root string) (*Index, error) {
	return NewDiscoverer().Discover(ctx, root)
}

// DiscoverAndStore indexes a project and writes the index to control-plane storage.
func DiscoverAndStore(ctx context.Context, w Writer, projectID, root string) (*Index, error) {
	idx, err := Discover(ctx, root)
	if err != nil {
		return nil, err
	}
	if w != nil {
		if err := w.ReplaceProjectKnowledge(ctx, projectID, idx); err != nil {
			return idx, err
		}
	}
	return idx, nil
}

func (d *Discoverer) Discover(ctx context.Context, root string) (*Index, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fs.ErrInvalid
	}
	d.applyDefaults()

	idx := &Index{Root: abs}
	ignore := newIgnoreCache()
	ignore.loadDir(abs, "")

	type queued struct {
		rel    string
		source string
		depth  int
	}
	seen := map[string]struct{}{}
	var queue []queued
	docsSeeds := 0

	err = filepath.WalkDir(abs, func(full string, de fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if de != nil && de.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		idx.Stats.FilesVisited++
		if idx.Stats.FilesVisited > d.MaxWalk {
			if de.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(abs, full)
		if err != nil {
			return nil
		}
		rel = NormalizeRel(filepath.ToSlash(rel))
		if rel == "" {
			return nil
		}
		depth := strings.Count(rel, "/") + 1
		if depth > d.MaxDepth {
			if de.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		name := de.Name()
		isDir := de.IsDir()
		if isDir && name == ".git" {
			return filepath.SkipDir
		}
		if ignore.ignored(rel, isDir) {
			idx.Stats.Ignored++
			if isDir {
				return filepath.SkipDir
			}
			return nil
		}
		if isDir {
			if _, skip := defaultSkipDirs[name]; skip {
				return filepath.SkipDir
			}
			if de.Type()&os.ModeSymlink != 0 {
				return filepath.SkipDir
			}
			ignore.loadDir(full, rel)
			return nil
		}
		if de.Type()&os.ModeSymlink != 0 && !symlinkInside(abs, full) {
			return nil
		}
		if !IsSeedPath(rel) {
			return nil
		}
		if strings.HasPrefix(rel, "docs/") {
			docsSeeds++
			if docsSeeds > d.MaxDocsSeeds {
				return nil
			}
		}
		if _, ok := seen[rel]; ok {
			return nil
		}
		seen[rel] = struct{}{}
		queue = append(queue, queued{rel: rel, source: SourceDiscovered, depth: 0})
		return nil
	})
	if err != nil {
		return nil, err
	}

	docs := map[string]*Document{}
	var edges []Edge
	linkFollowed := 0

	for i := 0; i < len(queue); i++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		item := queue[i]
		if len(docs) >= d.MaxFiles {
			break
		}
		doc, parsed, ok := d.readDocument(abs, item.rel, item.source)
		if !ok {
			continue
		}
		docs[doc.Path] = doc
		if doc.Status == StatusTruncated {
			idx.Stats.Truncated++
		}

		for _, link := range parsed.Links {
			target, ok := resolveLink(item.rel, link)
			if !ok {
				continue
			}
			kind := EdgeMarkdownLink
			if inList(parsed.Meta.Entrypoints, link) || inList(parsed.Meta.Entrypoints, target) {
				kind = EdgeEntryPoint
			}
			e := Edge{
				ID:             id.New(),
				FromDocumentID: doc.ID,
				FromPath:       doc.Path,
				ToPath:         target,
				Kind:           kind,
			}
			full := filepath.Join(abs, filepath.FromSlash(target))
			st, err := os.Lstat(full)
			if err != nil {
				e.Broken = true
				idx.Stats.BrokenRefs++
				edges = append(edges, e)
				continue
			}
			if ignore.ignored(target, st.IsDir()) {
				edges = append(edges, e)
				continue
			}
			if st.IsDir() {
				edges = append(edges, e)
				continue
			}
			edges = append(edges, e)
			if _, have := seen[target]; have {
				continue
			}
			if item.depth >= d.MaxLinkDepth || linkFollowed >= d.MaxReferences {
				continue
			}
			if !IsMarkdown(target) {
				continue
			}
			seen[target] = struct{}{}
			linkFollowed++
			src := SourceReferenced
			if kind == EdgeEntryPoint {
				src = SourceExplicit
			}
			queue = append(queue, queued{rel: target, source: src, depth: item.depth + 1})
		}
	}

	for _, doc := range docs {
		idx.Documents = append(idx.Documents, *doc)
	}
	sort.Slice(idx.Documents, func(i, j int) bool { return idx.Documents[i].Path < idx.Documents[j].Path })
	idByPath := map[string]string{}
	for i := range idx.Documents {
		idByPath[idx.Documents[i].Path] = idx.Documents[i].ID
		for _, sec := range d.sectionsFor(&idx.Documents[i]) {
			idx.Sections = append(idx.Sections, sec)
		}
	}

	cycles := detectCycles(edges)
	cycleSet := map[string]struct{}{}
	for _, c := range cycles {
		for i := 0; i < len(c.Paths); i++ {
			a := c.Paths[i]
			b := c.Paths[(i+1)%len(c.Paths)]
			cycleSet[a+"\x00"+b] = struct{}{}
		}
	}
	for i := range edges {
		if _, ok := cycleSet[edges[i].FromPath+"\x00"+edges[i].ToPath]; ok {
			edges[i].Cycle = true
		}
		if id := idByPath[edges[i].FromPath]; id != "" {
			edges[i].FromDocumentID = id
		}
	}
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].FromPath == edges[j].FromPath {
			return edges[i].ToPath < edges[j].ToPath
		}
		return edges[i].FromPath < edges[j].FromPath
	})
	idx.Edges = edges
	idx.Cycles = cycles
	idx.Stats.Cycles = len(cycles)
	idx.Stats.DocsIndexed = len(idx.Documents)
	idx.Revision = computeRevision(idx.Documents)
	idx.Conflicts = DetectConflicts(idx.Documents)
	return idx, nil
}

func (d *Discoverer) applyDefaults() {
	if d.MaxFileBytes <= 0 {
		d.MaxFileBytes = defaultMaxFileBytes
	}
	if d.MaxFiles <= 0 {
		d.MaxFiles = defaultMaxFiles
	}
	if d.MaxWalk <= 0 {
		d.MaxWalk = defaultMaxWalk
	}
	if d.MaxDepth <= 0 {
		d.MaxDepth = defaultMaxDepth
	}
	if d.MaxLinkDepth <= 0 {
		d.MaxLinkDepth = defaultMaxLinkDepth
	}
	if d.MaxReferences <= 0 {
		d.MaxReferences = defaultMaxReferences
	}
	if d.MaxDocsSeeds <= 0 {
		d.MaxDocsSeeds = defaultMaxDocsSeeds
	}
}

func (d *Discoverer) readDocument(root, rel, source string) (*Document, parsedFile, bool) {
	full := filepath.Join(root, filepath.FromSlash(rel))
	data, err := os.ReadFile(full)
	if err != nil || isBinary(data) {
		return nil, parsedFile{}, false
	}
	status := StatusOK
	if len(data) > d.MaxFileBytes {
		data = data[:d.MaxFileBytes]
		status = StatusTruncated
	}
	content := string(data)
	parsed := parseKnowledgeFile(rel, content)
	kind, family, domain, ok := ClassifyFilename(rel)
	if !ok {
		kind, family, domain = KindMarkdown, FamilyReferenced, DomainUnknown
	}
	scope := scopeFor(rel)
	always := false
	var globs []string
	explicit := parsed.Meta.Explicit
	if parsed.Meta.Kind != "" {
		kind = parsed.Meta.Kind
		explicit = true
	}
	if parsed.Meta.Family != "" {
		family = parsed.Meta.Family
		explicit = true
	}
	if parsed.Meta.Explicit && parsed.Meta.Domain != "" {
		domain = parsed.Meta.Domain
	}
	if parsed.Meta.Scope != "" {
		scope = NormalizeRel(parsed.Meta.Scope)
		if scope == "" {
			scope = "/"
		}
		explicit = true
	}
	if len(parsed.Meta.Globs) > 0 {
		globs = parsed.Meta.Globs
	}
	if parsed.Meta.AlwaysApply {
		always = true
	}
	if kind == KindCopilot && NormalizeRel(rel) == ".github/copilot-instructions.md" {
		always = true
		scope = "/"
	}
	if kind == KindCursorRules && strings.EqualFold(pathBase(rel), ".cursorrules") {
		always = true
		scope = "/"
	}
	if explicit {
		source = SourceExplicit
	}
	canonical := scope == "/" && IsCanonicalRootName(pathBase(rel)) && !strings.Contains(rel, "/")
	doc := &Document{
		ID:              id.New(),
		Path:            NormalizeRel(rel),
		Kind:            kind,
		Family:          family,
		Scope:           scope,
		AuthorityDomain: domain,
		Source:          source,
		Hash:            crypto.HashSHA256(data),
		Status:          status,
		Content:         content,
		Canonical:       canonical,
		AlwaysApply:     always,
		Globs:           globs,
		Explicit:        explicit,
	}
	return doc, parsed, true
}

func (d *Discoverer) sectionsFor(doc *Document) []Section {
	parsed := parseKnowledgeFile(doc.Path, doc.Content)
	out := make([]Section, 0, len(parsed.Sections))
	for i, s := range parsed.Sections {
		body := s.Body
		out = append(out, Section{
			ID:         id.New(),
			DocumentID: doc.ID,
			Heading:    s.Heading,
			Ordinal:    i + 1,
			Hash:       crypto.HashSHA256([]byte(s.Heading + "\n" + body)),
			Body:       body,
		})
	}
	return out
}

func computeRevision(docs []Document) string {
	var b strings.Builder
	for _, d := range docs {
		b.WriteString(d.Path)
		b.WriteByte('\t')
		b.WriteString(d.Hash)
		b.WriteByte('\n')
	}
	return crypto.HashSHA256([]byte(b.String()))
}

func symlinkInside(root, full string) bool {
	resolved, err := filepath.EvalSymlinks(full)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(root, resolved)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
}

func inList(list []string, v string) bool {
	v = NormalizeRel(v)
	for _, x := range list {
		if NormalizeRel(x) == v || x == v {
			return true
		}
	}
	return false
}

func detectCycles(edges []Edge) []Cycle {
	graph := map[string][]string{}
	for _, e := range edges {
		if e.Broken {
			continue
		}
		graph[e.FromPath] = append(graph[e.FromPath], e.ToPath)
	}
	const (
		white = 0
		gray  = 1
		black = 2
	)
	color := map[string]int{}
	var stack []string
	var cycles []Cycle
	seenCycle := map[string]struct{}{}

	var dfs func(string)
	dfs = func(n string) {
		color[n] = gray
		stack = append(stack, n)
		for _, to := range graph[n] {
			switch color[to] {
			case white:
				dfs(to)
			case gray:
				cyc := cycleFromStack(stack, to)
				key := cycleKey(cyc.Paths)
				if _, ok := seenCycle[key]; !ok {
					seenCycle[key] = struct{}{}
					cycles = append(cycles, cyc)
				}
			}
		}
		stack = stack[:len(stack)-1]
		color[n] = black
	}
	nodes := make([]string, 0, len(graph))
	for n := range graph {
		nodes = append(nodes, n)
	}
	sort.Strings(nodes)
	for _, n := range nodes {
		if color[n] == white {
			dfs(n)
		}
	}
	return cycles
}

func cycleFromStack(stack []string, to string) Cycle {
	var paths []string
	for i, p := range stack {
		if p == to {
			paths = append([]string{}, stack[i:]...)
			break
		}
	}
	if len(paths) == 0 {
		paths = append([]string{}, stack...)
	}
	paths = append(paths, to)
	return Cycle{Paths: paths}
}

func cycleKey(paths []string) string {
	if len(paths) < 2 {
		return strings.Join(paths, ">")
	}
	core := paths[:len(paths)-1]
	minI := 0
	for i := 1; i < len(core); i++ {
		if core[i] < core[minI] {
			minI = i
		}
	}
	rot := append(append([]string{}, core[minI:]...), core[:minI]...)
	return strings.Join(rot, ">")
}
