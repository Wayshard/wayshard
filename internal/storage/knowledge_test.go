package storage

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Wayshard/wayshard/internal/domain"
	"github.com/Wayshard/wayshard/internal/knowledge"
)

func TestReplaceAndLoadKnowledgeIndex(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("# A\nSee [missing](nope.md)\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	st, err := Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	p := &domain.Project{Name: "p", Path: root, SourceKind: "filesystem"}
	if err := st.InsertProject(ctx, p); err != nil {
		t.Fatal(err)
	}
	idx, err := knowledge.DiscoverAndStore(ctx, st, p.ID, root)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := st.LoadKnowledgeIndex(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.DocumentByPath("AGENTS.md") == nil {
		t.Fatal("stored index missing AGENTS.md")
	}
	if len(loaded.BrokenEdges()) == 0 {
		t.Fatal("stored broken edge missing")
	}
	got, err := st.GetProject(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.KnowledgeRev != idx.Revision || got.KnowledgeRev == "" {
		t.Fatalf("knowledge_rev = %q want %q", got.KnowledgeRev, idx.Revision)
	}
}
