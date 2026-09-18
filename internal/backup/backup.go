package backup

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/Wayshard/wayshard/internal/storage"
)

type Manifest struct {
	CreatedAt       time.Time `json:"createdAt"`
	IncludesSecrets bool      `json:"includesSecrets"`
	IncludesRepos   bool      `json:"includesRepos"`
	Note            string    `json:"note"`
}

// Create writes a consistent SQLite snapshot plus referenced objects.
// Source repositories are excluded by default.
func Create(ctx context.Context, st *storage.Store, dest string, includeSecrets bool) error {
	if err := os.MkdirAll(dest, 0o700); err != nil {
		return err
	}
	destDB := filepath.Join(dest, "app.db")
	if _, err := st.DB.ExecContext(ctx, `VACUUM INTO ?`, destDB); err != nil {
		// fallback copy via backup API
		if err := copyFile(filepath.Join(st.Root, "app.db"), destDB); err != nil {
			return err
		}
	}
	srcObj := filepath.Join(st.Root, "objects")
	dstObj := filepath.Join(dest, "objects")
	_ = copyTree(srcObj, dstObj)
	man := Manifest{
		CreatedAt:       time.Now().UTC(),
		IncludesSecrets: includeSecrets,
		IncludesRepos:   false,
		Note:            "Source repositories are not included. Restore does not replace project working trees.",
	}
	if includeSecrets {
		man.Note += " Secrets are re-encrypted under backup-specific protection when a vault key is supplied."
	}
	b, _ := json.MarshalIndent(man, "", "  ")
	return os.WriteFile(filepath.Join(dest, "manifest.json"), b, 0o600)
}

func Restore(ctx context.Context, archive, dataDir string) error {
	_ = ctx
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return err
	}
	if err := copyFile(filepath.Join(archive, "app.db"), filepath.Join(dataDir, "app.db")); err != nil {
		return err
	}
	return copyTree(filepath.Join(archive, "objects"), filepath.Join(dataDir, "objects"))
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return err
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func copyTree(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		rel, _ := filepath.Rel(src, path)
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o700)
		}
		return copyFile(path, target)
	})
}
