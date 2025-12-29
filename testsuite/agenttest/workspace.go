package agenttest

import (
	"crypto/sha256"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
)

type WorkspaceSnapshot struct {
	Files map[string]string // rel path -> sha256 hex
}

func SnapshotWorkspace(root string, exclude []string) (*WorkspaceSnapshot, error) {
	root = filepath.Clean(root)
	files := make(map[string]string)
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path == root {
				return nil
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			rel = filepath.ToSlash(rel)
			for _, pat := range exclude {
				if matchGlob(pat, rel) || matchGlob(pat, rel+"/") || matchGlob(pat, rel+"/**") {
					return filepath.SkipDir
				}
			}
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		for _, pat := range exclude {
			if matchGlob(pat, rel) {
				return nil
			}
		}
		sum, err := hashFile(path)
		if err != nil {
			return err
		}
		files[rel] = sum
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &WorkspaceSnapshot{Files: files}, nil
}

func DiffSnapshots(before, after *WorkspaceSnapshot) (changed []string) {
	if before == nil || after == nil {
		return nil
	}
	seen := make(map[string]struct{}, len(before.Files)+len(after.Files))
	for k := range before.Files {
		seen[k] = struct{}{}
	}
	for k := range after.Files {
		seen[k] = struct{}{}
	}
	for file := range seen {
		if before.Files[file] != after.Files[file] {
			changed = append(changed, file)
		}
	}
	sort.Strings(changed)
	return changed
}

func CopyWorkspace(src, dst string, exclude []string) error {
	src = filepath.Clean(src)
	dst = filepath.Clean(dst)
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		relSlash := filepath.ToSlash(rel)
		for _, pat := range exclude {
			if d.IsDir() {
				if matchGlob(pat, relSlash) || matchGlob(pat, relSlash+"/") || matchGlob(pat, relSlash+"/**") {
					return filepath.SkipDir
				}
			} else if matchGlob(pat, relSlash) {
				return nil
			}
		}

		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, info.Mode().Perm())
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, in); err != nil {
			out.Close()
			return err
		}
		return out.Close()
	})
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
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}
