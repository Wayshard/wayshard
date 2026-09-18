package workspace

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const (
	TypeFile    = "file"
	TypeSymlink = "symlink"

	ModeFile = "100644"
	ModeExec = "100755"
	ModeLink = "120000"
)

var (
	ErrUnsafePath = errors.New("unsafe path")
	ErrNestedDest = errors.New("destination must not be inside source")
	ErrNotDir     = errors.New("source is not a directory")
	ErrSkipGitDir = errors.New("refusing to publish into .git")
)

// FileMeta is a content-addressed description of one path.
type FileMeta struct {
	Path    string `json:"path"`
	Type    string `json:"type"`
	Mode    string `json:"mode"`
	Hash    string `json:"hash"`
	Size    int64  `json:"size,omitempty"`
	Target  string `json:"target,omitempty"`
	Missing bool   `json:"missing,omitempty"`
}

func (m FileMeta) Equal(o FileMeta) bool {
	if m.Missing && o.Missing {
		return true
	}
	if m.Missing || o.Missing {
		return false
	}
	return m.Type == o.Type && m.Mode == o.Mode && m.Hash == o.Hash && m.Target == o.Target
}

func missingMeta(rel string) FileMeta {
	return FileMeta{Path: rel, Missing: true}
}

func HashBytes(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func HashString(s string) string { return HashBytes([]byte(s)) }

func SafeRel(path string) (string, error) {
	path = filepath.ToSlash(path)
	if path == "" {
		return "", ErrUnsafePath
	}
	if filepath.IsAbs(path) {
		return "", ErrUnsafePath
	}
	clean := filepath.ToSlash(filepath.Clean(path))
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return "", ErrUnsafePath
	}
	if clean == "." {
		return "", ErrUnsafePath
	}
	if clean == ".git" || strings.HasPrefix(clean, ".git/") {
		return "", fmt.Errorf("%w: %s", ErrSkipGitDir, clean)
	}
	return clean, nil
}

func nested(parent, child string) bool {
	p, err := filepath.Abs(parent)
	if err != nil {
		return false
	}
	c, err := filepath.Abs(child)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(p, c)
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
}

func gitMode(fi os.FileInfo) string {
	if fi.Mode()&os.ModeSymlink != 0 {
		return ModeLink
	}
	if fi.Mode()&0o111 != 0 {
		return ModeExec
	}
	return ModeFile
}

func permFor(mode string) os.FileMode {
	if mode == ModeExec {
		return 0o755
	}
	return 0o644
}

func lstatMeta(root, rel string) (FileMeta, error) {
	rel, err := SafeRel(rel)
	if err != nil {
		return FileMeta{}, err
	}
	p := filepath.Join(root, filepath.FromSlash(rel))
	fi, err := os.Lstat(p)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return missingMeta(rel), nil
		}
		return FileMeta{}, err
	}
	return metaFromInfo(p, rel, fi)
}

func metaFromInfo(abs, rel string, fi os.FileInfo) (FileMeta, error) {
	m := FileMeta{Path: rel, Mode: gitMode(fi)}
	if fi.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(abs)
		if err != nil {
			return FileMeta{}, err
		}
		m.Type = TypeSymlink
		m.Target = target
		m.Hash = HashString(target)
		return m, nil
	}
	if !fi.Mode().IsRegular() {
		return FileMeta{}, fmt.Errorf("unsupported file type %s: %s", fi.Mode(), rel)
	}
	h, size, err := hashRegular(abs)
	if err != nil {
		return FileMeta{}, err
	}
	m.Type = TypeFile
	m.Hash = h
	m.Size = size
	return m, nil
}

func hashRegular(path string) (string, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}

// ListFiles hashes regular files and symlinks under root, skipping .git.
func ListFiles(root string) (map[string]FileMeta, error) {
	return walkMeaningful(root)
}

func walkMeaningful(root string) (map[string]FileMeta, error) {
	out := make(map[string]FileMeta)
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
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
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if _, err := SafeRel(rel); err != nil {
			return nil
		}
		fi, err := d.Info()
		if err != nil {
			return err
		}
		if fi.Mode()&os.ModeSymlink == 0 && !fi.Mode().IsRegular() {
			return nil
		}
		m, err := metaFromInfo(path, rel, fi)
		if err != nil {
			return err
		}
		out[rel] = m
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func copyTree(src, dst string) error {
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	files, err := walkMeaningful(src)
	if err != nil {
		return err
	}
	for rel := range files {
		if err := copyRel(src, dst, rel); err != nil {
			return err
		}
	}
	return nil
}

func copyRel(srcRoot, dstRoot, rel string) error {
	rel, err := SafeRel(rel)
	if err != nil {
		return err
	}
	src := filepath.Join(srcRoot, filepath.FromSlash(rel))
	dst := filepath.Join(dstRoot, filepath.FromSlash(rel))
	return copyPath(src, dst)
}

func copyPath(src, dst string) error {
	fi, err := os.Lstat(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(src)
		if err != nil {
			return err
		}
		_ = os.Remove(dst)
		return os.Symlink(target, dst)
	}
	if !fi.Mode().IsRegular() {
		return fmt.Errorf("unsupported file type at %s", src)
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp := dst + ".wayshard-tmp"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, fi.Mode().Perm())
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		os.Remove(tmp)
		return copyErr
	}
	if closeErr != nil {
		os.Remove(tmp)
		return closeErr
	}
	if err := os.Chmod(tmp, fi.Mode().Perm()); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, dst); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

func writeBytes(dst string, data []byte, mode string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	tmp := dst + ".wayshard-tmp"
	if err := os.WriteFile(tmp, data, permFor(mode)); err != nil {
		return err
	}
	if err := os.Chmod(tmp, permFor(mode)); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, dst); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

func writeSymlink(dst, target string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	_ = os.Remove(dst)
	return os.Symlink(target, dst)
}

func applyMeta(dstRoot string, m FileMeta, content []byte) error {
	rel, err := SafeRel(m.Path)
	if err != nil {
		return err
	}
	dst := filepath.Join(dstRoot, filepath.FromSlash(rel))
	if m.Missing {
		if err := os.Remove(dst); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	if m.Type == TypeSymlink {
		return writeSymlink(dst, m.Target)
	}
	return writeBytes(dst, content, m.Mode)
}

func pruneEmpty(root, rel string) {
	dir := filepath.Dir(filepath.Join(root, filepath.FromSlash(rel)))
	for {
		if dir == root || !strings.HasPrefix(dir, root) {
			return
		}
		if err := os.Remove(dir); err != nil {
			return
		}
		dir = filepath.Dir(dir)
	}
}

func absClean(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.Clean(abs), nil
}

func ensureOutside(source, dest string) error {
	if nested(source, dest) {
		return fmt.Errorf("%w: %s in %s", ErrNestedDest, dest, source)
	}
	return nil
}

func unionPaths(maps ...map[string]FileMeta) []string {
	seen := make(map[string]struct{})
	var out []string
	for _, m := range maps {
		for p := range m {
			if _, ok := seen[p]; ok {
				continue
			}
			seen[p] = struct{}{}
			out = append(out, p)
		}
	}
	return out
}
