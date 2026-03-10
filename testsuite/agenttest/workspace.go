package agenttest

import (
	"crypto/sha256"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/lexcodex/relurpify/framework/config"
	"github.com/lexcodex/relurpify/framework/templates"
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

func FilterChangedFiles(changed []string, ignore []string) []string {
	if len(changed) == 0 || len(ignore) == 0 {
		return changed
	}
	filtered := make([]string, 0, len(changed))
	for _, file := range changed {
		skip := false
		for _, pat := range ignore {
			if matchGlob(pat, file) {
				skip = true
				break
			}
		}
		if !skip {
			filtered = append(filtered, file)
		}
	}
	sort.Strings(filtered)
	return filtered
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

func MaterializeDerivedWorkspace(targetWorkspace, derivedWorkspace, templateProfile, manifestRef string, exclude []string, overlayFiles []SetupFileSpec) error {
	targetWorkspace = filepath.Clean(targetWorkspace)
	derivedWorkspace = filepath.Clean(derivedWorkspace)
	if err := os.RemoveAll(derivedWorkspace); err != nil {
		return err
	}
	if err := os.MkdirAll(derivedWorkspace, 0o755); err != nil {
		return err
	}

	copyExclude := append([]string{}, exclude...)
	copyExclude = append(copyExclude, config.DirName, filepath.ToSlash(filepath.Join(config.DirName, "**")))
	if err := CopyWorkspace(targetWorkspace, derivedWorkspace, uniqueStrings(copyExclude)); err != nil {
		return err
	}

	paths := config.New(derivedWorkspace)
	resolver := templates.NewResolver()
	profileRoot, err := resolver.ResolveTestsuiteTemplateProfile(templateProfile)
	if err != nil {
		return fmt.Errorf("resolve testsuite template profile %q: %w", firstNonEmpty(templateProfile, "default"), err)
	}
	if err := copyRenderedTree(profileRoot, paths.ConfigRoot(), derivedWorkspace, ""); err != nil {
		return err
	}
	if err := ensureDerivedManifest(resolver, targetWorkspace, derivedWorkspace, manifestRef); err != nil {
		return err
	}
	if err := applyWorkspaceFiles(derivedWorkspace, overlayFiles); err != nil {
		return err
	}

	for _, dir := range []string{
		paths.AgentsDir(),
		paths.SkillsDir(),
		paths.LogsDir(),
		paths.TelemetryDir(),
		paths.MemoryDir(),
		paths.SessionsDir(),
		paths.TestRunsDir(),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return nil
}

func ensureDerivedManifest(resolver templates.Resolver, targetWorkspace, derivedWorkspace, manifestRef string) error {
	manifestRef = filepath.ToSlash(strings.TrimSpace(manifestRef))
	if manifestRef == "" || filepath.IsAbs(manifestRef) || !strings.HasPrefix(manifestRef, config.DirName+"/") {
		return nil
	}
	dst := filepath.Join(derivedWorkspace, filepath.FromSlash(manifestRef))
	if _, err := os.Stat(dst); err == nil {
		return nil
	}

	var src string
	if strings.HasPrefix(manifestRef, filepath.ToSlash(filepath.Join(config.DirName, "agents"))+"/") {
		name := strings.TrimSuffix(filepath.Base(manifestRef), filepath.Ext(manifestRef))
		src, _ = resolver.ResolveStarterAgent(name)
	}
	if src == "" {
		candidate := filepath.Join(targetWorkspace, filepath.FromSlash(manifestRef))
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			src = candidate
		}
	}
	if src == "" {
		return nil
	}
	return copyRenderedFile(src, dst, derivedWorkspace, targetWorkspace)
}

func copyRenderedTree(srcRoot, dstRoot, workspace, sourceWorkspace string) error {
	return filepath.WalkDir(srcRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(srcRoot, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return os.MkdirAll(dstRoot, 0o755)
		}
		target := filepath.Join(dstRoot, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		return copyRenderedFile(path, target, workspace, sourceWorkspace)
	})
}

func copyRenderedFile(src, dst, workspace, sourceWorkspace string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	rendered := renderWorkspaceContent(data, workspace, sourceWorkspace)
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	info, err := os.Stat(src)
	mode := fs.FileMode(0o644)
	if err == nil {
		mode = info.Mode().Perm()
	}
	return os.WriteFile(dst, rendered, mode)
}

func renderWorkspaceContent(data []byte, workspace, sourceWorkspace string) []byte {
	rendered := strings.ReplaceAll(string(data), "${workspace}", filepath.ToSlash(workspace))
	if sourceWorkspace != "" {
		rendered = strings.ReplaceAll(rendered, filepath.ToSlash(sourceWorkspace), filepath.ToSlash(workspace))
	}
	return []byte(rendered)
}

func applyWorkspaceFiles(workspace string, files []SetupFileSpec) error {
	for _, f := range files {
		if f.Path == "" {
			continue
		}
		target := filepath.Join(workspace, filepath.FromSlash(f.Path))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(target, []byte(f.Content), 0o644); err != nil {
			return err
		}
	}
	return nil
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
