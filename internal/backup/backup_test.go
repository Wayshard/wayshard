package backup

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Wayshard/wayshard/internal/domain"
	"github.com/Wayshard/wayshard/internal/storage"
)

func TestBackupExcludesReposByDefault(t *testing.T) {
	ctx := context.Background()
	st, err := storage.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	p := &domain.Project{Name: "p", Path: "/tmp/src", SourceKind: "filesystem"}
	_ = st.InsertProject(ctx, p)
	dest := t.TempDir()
	if err := Create(ctx, st, dest, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dest, "manifest.json")); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(dest, "manifest.json"))
	if !contains(string(b), "not included") {
		t.Fatalf("manifest should say repos excluded: %s", b)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || (len(s) > 0 && (index(s, sub) >= 0)))
}
func index(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
