package workspace

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// CanonicalTreeHash computes a deterministic SHA-256 digest of a materialized
// workspace tree. It covers exactly the restorable state:
//
//	relative path, entry type, executable bit, file content, symlink target,
//	and tree membership (including empty directories).
//
// It intentionally excludes absolute paths, inode numbers, mtime/ctime and
// traversal order, so the same logical tree hashes identically wherever it is
// materialized.
func CanonicalTreeHash(root string) (string, error) {
	type entry struct {
		rel  string
		line string
	}
	var entries []entry
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == root {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		fi, err := os.Lstat(path)
		if err != nil {
			return err
		}
		switch {
		case fi.Mode()&os.ModeSymlink != 0:
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			entries = append(entries, entry{rel, "l " + rel + " " + hashString(target)})
		case fi.IsDir():
			entries = append(entries, entry{rel, "d " + rel})
		case fi.Mode().IsRegular():
			sum, err := hashFile(path)
			if err != nil {
				return err
			}
			mode := "-"
			if fi.Mode()&0o111 != 0 {
				mode = "x"
			}
			entries = append(entries, entry{rel, "f " + rel + " " + mode + " " + sum})
		default:
			return fmt.Errorf("unsupported entry type at %s", rel)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].rel < entries[j].rel })
	var b strings.Builder
	b.WriteString("wayshard-tree-v2\n")
	for _, e := range entries {
		b.WriteString(e.line)
		b.WriteString("\n")
	}
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:]), nil
}

func hashString(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
