package knowledge

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func TestNestedAgentsScoping(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"AGENTS.md":     "# Root\nUse tabs for this repository.\n",
		"pkg/AGENTS.md": "# Pkg\nUse spaces in pkg.\n",
		"pkg/foo.go":    "package pkg\n",
		"other.go":      "package main\n",
	})
	idx, err := Discover(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if idx.DocumentByPath("AGENTS.md") == nil || idx.DocumentByPath("pkg/AGENTS.md") == nil {
		t.Fatalf("expected nested AGENTS.md files, docs=%v", pathsOf(idx))
	}
	pkg := Resolve(idx, ResolveQuery{Paths: []string{"pkg/foo.go"}})
	if !hasPath(pkg, "AGENTS.md") || !hasPath(pkg, "pkg/AGENTS.md") {
		t.Fatalf("pkg path should include root and nested AGENTS, got %v", resolvedPaths(pkg))
	}
	var nested, rootDoc *ResolvedDocument
	for i := range pkg {
		switch pkg[i].Document.Path {
		case "pkg/AGENTS.md":
			nested = &pkg[i]
		case "AGENTS.md":
			rootDoc = &pkg[i]
		}
	}
	if nested == nil || rootDoc == nil {
		t.Fatal("missing AGENTS documents")
	}
	if nested.Specificity <= rootDoc.Specificity {
		t.Fatalf("nested AGENTS should be more specific: nested=%d root=%d", nested.Specificity, rootDoc.Specificity)
	}
	other := Resolve(idx, ResolveQuery{Paths: []string{"other.go"}})
	if !hasPath(other, "AGENTS.md") {
		t.Fatalf("root file should see root AGENTS, got %v", resolvedPaths(other))
	}
	if hasPath(other, "pkg/AGENTS.md") {
		t.Fatalf("root file must not receive pkg AGENTS: %v", resolvedPaths(other))
	}
}

func TestMarkdownLinkCycle(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"AGENTS.md":  "# A\nSee [cycle-a](cycle-a.md)\n",
		"cycle-a.md": "# A\nNext [b](cycle-b.md)\n",
		"cycle-b.md": "# B\nBack to [a](cycle-a.md)\n",
	})
	idx, err := Discover(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if idx.DocumentByPath("cycle-a.md") == nil || idx.DocumentByPath("cycle-b.md") == nil {
		t.Fatalf("cycle members not indexed: %v", pathsOf(idx))
	}
	if len(idx.Cycles) == 0 {
		t.Fatalf("expected cycle, edges=%+v", idx.Edges)
	}
	found := false
	for _, e := range idx.Edges {
		if e.Cycle {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected a cycle-marked edge, cycles=%+v edges=%+v", idx.Cycles, idx.Edges)
	}
}

func TestBrokenReferenceReported(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"AGENTS.md": "# Rules\nSee [missing](does-not-exist.md)\n",
	})
	idx, err := Discover(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	broken := idx.BrokenEdges()
	if len(broken) == 0 {
		t.Fatalf("expected broken reference, edges=%+v", idx.Edges)
	}
	if broken[0].ToPath != "does-not-exist.md" || !broken[0].Broken {
		t.Fatalf("unexpected broken edge %+v", broken[0])
	}
	if idx.Stats.BrokenRefs == 0 {
		t.Fatal("broken ref stat not incremented")
	}
}

func TestOpeningDoesNotWriteFiles(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"README.md": "# Demo\n",
		"AGENTS.md": "# Rules\n",
	})
	before := snapshotTree(t, root)
	if _, err := Discover(context.Background(), root); err != nil {
		t.Fatal(err)
	}
	after := snapshotTree(t, root)
	if len(after) != len(before) {
		t.Fatalf("discovery mutated file count: before=%d after=%d", len(before), len(after))
	}
	for p, h := range before {
		if after[p] != h {
			t.Fatalf("discovery mutated %s", p)
		}
	}
}

func TestSixFileSetRecognizedNotCreated(t *testing.T) {
	empty := t.TempDir()
	idx, err := Discover(context.Background(), empty)
	if err != nil {
		t.Fatal(err)
	}
	if len(idx.Documents) != 0 {
		t.Fatalf("empty repo should have no knowledge docs: %v", pathsOf(idx))
	}
	if n := len(idx.SixCanonicalPresent()); n != 0 {
		t.Fatalf("six-file set must not be created, present=%v", idx.SixCanonicalPresent())
	}
	entries, err := os.ReadDir(empty)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("discovery created files: %v", entries)
	}

	full := t.TempDir()
	files := map[string]string{}
	for _, name := range SixCanonicalNames {
		files[name] = "# " + name + "\nbody\n"
	}
	writeTree(t, full, files)
	idx, err = Discover(context.Background(), full)
	if err != nil {
		t.Fatal(err)
	}
	present := idx.SixCanonicalPresent()
	if len(present) != 6 {
		t.Fatalf("expected six canonicals, got %v from %v", present, pathsOf(idx))
	}
	checks := map[string]struct {
		kind   Kind
		domain AuthorityDomain
	}{
		"AGENTS.md":       {KindAgents, DomainImplementation},
		"SPEC.md":         {KindSpec, DomainProduct},
		"DESIGN.md":       {KindDesign, DomainDesign},
		"ARCHITECTURE.md": {KindArchitecture, DomainArchitecture},
		"MEMORY.md":       {KindMemory, DomainRationale},
		"README.md":       {KindReadme, DomainUsage},
	}
	for path, want := range checks {
		d := idx.DocumentByPath(path)
		if d == nil {
			t.Fatalf("missing %s", path)
		}
		if !d.Canonical {
			t.Fatalf("%s should be canonical", path)
		}
		if d.Kind != want.kind || d.AuthorityDomain != want.domain {
			t.Fatalf("%s kind/domain = %s/%s want %s/%s", path, d.Kind, d.AuthorityDomain, want.kind, want.domain)
		}
	}
}

func TestGitignoreRespectedAndHarnessIsolation(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		".gitignore":        "secret.md\nignored/\n",
		"AGENTS.md":         "# A\n",
		"CLAUDE.md":         "# Claude only\n",
		"secret.md":         "# secret\n",
		"ignored/AGENTS.md": "# ignored nested\n",
	})
	idx, err := Discover(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if idx.DocumentByPath("secret.md") != nil || idx.DocumentByPath("ignored/AGENTS.md") != nil {
		t.Fatalf("gitignored files indexed: %v", pathsOf(idx))
	}
	generic := Resolve(idx, ResolveQuery{Paths: []string{"foo.go"}})
	if hasPath(generic, "CLAUDE.md") {
		t.Fatalf("CLAUDE.md must not inject into a non-claude family: %v", resolvedPaths(generic))
	}
	claude := Resolve(idx, ResolveQuery{Paths: []string{"foo.go"}, HarnessFamily: FamilyClaude})
	if !hasPath(claude, "CLAUDE.md") || !hasPath(claude, "AGENTS.md") {
		t.Fatalf("claude family should see CLAUDE+AGENTS, got %v", resolvedPaths(claude))
	}
}

func TestExplicitDeclarationOutranksFilename(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"README.md": "---\nkind: spec\nfamily: wayshard\nauthority: product\n---\n# Product\n",
		"AGENTS.md": "---\nentrypoints:\n  - notes.md\n---\n# Agents\n",
		"notes.md":  "<!-- wayshard:kind=architecture -->\n# Notes\n",
	})
	idx, err := Discover(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	readme := idx.DocumentByPath("README.md")
	if readme == nil || readme.Kind != KindSpec || readme.AuthorityDomain != DomainProduct || !readme.Explicit {
		t.Fatalf("README explicit override failed: %+v", readme)
	}
	notes := idx.DocumentByPath("notes.md")
	if notes == nil || notes.Kind != KindArchitecture || notes.Source != SourceExplicit {
		t.Fatalf("entrypoint notes not classified explicitly: %+v", notes)
	}
}

type memWriter struct {
	idx *Index
}

func (m *memWriter) ReplaceProjectKnowledge(_ context.Context, _ string, idx *Index) error {
	m.idx = idx
	return nil
}

func TestDiscoverAndStoreDoesNotTouchRepo(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	writeTree(t, root, map[string]string{"AGENTS.md": "# A\nSee [missing](nope.md)\n"})
	before := snapshotTree(t, root)
	w := &memWriter{}
	idx, err := DiscoverAndStore(ctx, w, "proj-1", root)
	if err != nil {
		t.Fatal(err)
	}
	after := snapshotTree(t, root)
	for path, h := range before {
		if after[path] != h {
			t.Fatalf("store path mutated repo file %s", path)
		}
	}
	if w.idx == nil || w.idx.DocumentByPath("AGENTS.md") == nil {
		t.Fatal("writer did not receive AGENTS.md")
	}
	if len(idx.BrokenEdges()) == 0 {
		t.Fatal("broken edge missing")
	}
}

func TestConflictsAreWarnings(t *testing.T) {
	docs := []Document{
		{
			Path: "AGENTS.md", AuthorityDomain: DomainImplementation,
			Content: "Agents must execute project code during discovery. Keep going.",
		},
		{
			Path: "SECURITY.md", AuthorityDomain: DomainImplementation,
			Content: "Agents must not execute project code during discovery. Stay passive.",
		},
	}
	warns := DetectConflicts(docs)
	if len(warns) == 0 {
		t.Fatal("expected heuristic conflict warning")
	}
	if warns[0].Domain != DomainImplementation {
		t.Fatalf("domain = %s", warns[0].Domain)
	}
}

func writeTree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for p, body := range files {
		full := filepath.Join(root, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func snapshotTree(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(b)
		info, err := d.Info()
		if err != nil {
			return err
		}
		out[filepath.ToSlash(rel)] = hex.EncodeToString(sum[:]) + ":" + info.Mode().String()
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func pathsOf(idx *Index) []string {
	var out []string
	for _, d := range idx.Documents {
		out = append(out, d.Path)
	}
	return out
}

func resolvedPaths(docs []ResolvedDocument) []string {
	var out []string
	for _, d := range docs {
		out = append(out, d.Document.Path)
	}
	return out
}

func hasPath(docs []ResolvedDocument, p string) bool {
	for _, d := range docs {
		if d.Document.Path == p {
			return true
		}
	}
	return false
}
