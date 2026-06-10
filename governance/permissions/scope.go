package permissions

import (
	"fmt"
	"path/filepath"
	"strings"
)

var (
	ErrFileScopeOutsideWorkspace = fmt.Errorf("outside workspace")
	ErrFileScopeProtectedPath    = fmt.Errorf("protected path")
)

// FileScopePolicy captures the filesystem boundary enforced by sandbox-aware
// tools before host I/O occurs.
type FileScopePolicy struct {
	Workspace      string
	ProtectedPaths []string
}

// FileScopeError reports sandbox filesystem boundary violations.
type FileScopeError struct {
	Action string
	Path   string
	Root   string
	Reason string
}

func (e *FileScopeError) Unwrap() error {
	if e == nil {
		return nil
	}
	if e.Reason == ErrFileScopeOutsideWorkspace.Error() {
		return ErrFileScopeOutsideWorkspace
	}
	if e.Reason == ErrFileScopeProtectedPath.Error() {
		return ErrFileScopeProtectedPath
	}
	return nil
}

func (e *FileScopeError) Error() string {
	if e == nil {
		return ""
	}
	if e.Root != "" {
		return e.Reason + ": " + e.Path + " (root: " + e.Root + ")"
	}
	return e.Reason + ": " + e.Path
}

// NewFileScopePolicy canonicalizes a workspace root and protected roots.
func NewFileScopePolicy(workspace string, protectedPaths []string) *FileScopePolicy {
	policy := &FileScopePolicy{Workspace: canonicalScopePath(workspace)}
	for _, root := range defaultFileScopeProtectedRoots(policy.Workspace) {
		if root != "" {
			policy.ProtectedPaths = append(policy.ProtectedPaths, root)
		}
	}
	for _, path := range protectedPaths {
		if resolved := canonicalScopePath(path); resolved != "" {
			policy.ProtectedPaths = append(policy.ProtectedPaths, resolved)
		}
	}
	policy.ProtectedPaths = dedupeScopePaths(policy.ProtectedPaths)
	return policy
}

// Check validates a target path before sandbox-backed host I/O proceeds.
func (p *FileScopePolicy) Check(action FileSystemAction, target string) error {
	if p == nil {
		return nil
	}
	clean := canonicalScopePath(target)
	if clean == "" {
		return fmt.Errorf("%w: %s", ErrFileScopeOutsideWorkspace, target)
	}
	if p.Workspace != "" && !pathWithinOrEqual(clean, p.Workspace) {
		return &FileScopeError{Action: string(action), Path: clean, Reason: ErrFileScopeOutsideWorkspace.Error()}
	}
	for _, root := range p.ProtectedPaths {
		if root == "" {
			continue
		}
		if pathWithinOrEqual(clean, root) {
			return &FileScopeError{Action: string(action), Path: clean, Root: root, Reason: ErrFileScopeProtectedPath.Error()}
		}
	}
	return nil
}

// Contains checks whether path is within the workspace.
func (p *FileScopePolicy) Contains(basePath string) error {
	if !strings.HasPrefix(filepath.Clean(basePath), p.Workspace) {
		return ErrFileScopeOutsideWorkspace
	}
	for _, root := range p.ProtectedPaths {
		if strings.HasPrefix(filepath.Clean(basePath), root) {
			return fmt.Errorf("%w: %s", ErrFileScopeProtectedPath, root)
		}
	}
	return nil
}

func canonicalScopePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	clean := filepath.Clean(path)
	if !filepath.IsAbs(clean) {
		if abs, err := filepath.Abs(clean); err == nil {
			clean = abs
		}
	}
	if resolved, err := filepath.EvalSymlinks(clean); err == nil {
		clean = resolved
	} else {
		clean = resolveExistingPrefix(clean)
	}
	return filepath.ToSlash(filepath.Clean(clean))
}

func resolveExistingPrefix(p string) string {
	if p == "" || p == "/" {
		return p
	}
	var tail []string
	current := filepath.Clean(p)
	for current != "" && current != "/" {

		if resolved, err := filepath.EvalSymlinks(current); err == nil {
			for i := len(tail) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, tail[i])
			}
			return filepath.Clean(resolved)
		}
		tail = append(tail, filepath.Base(current))
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	return p
}

func pathWithinOrEqual(target, root string) bool {
	target = filepath.ToSlash(filepath.Clean(target))
	root = filepath.ToSlash(filepath.Clean(root))
	if root == "" {
		return false
	}
	if root == "/" {
		return true
	}
	if target == root {
		return true
	}
	return strings.HasPrefix(target, root+"/")
}

func defaultFileScopeProtectedRoots(workspace string) []string {
	if workspace == "" {
		return nil
	}
	return []string{
		filepath.ToSlash(filepath.Clean(filepath.Join(workspace, "relurpify_cfg"))),
		filepath.ToSlash(filepath.Clean(filepath.Join(workspace, ".git"))),
	}
}

func cleanScopeRoots(roots []string) []string {
	out := make([]string, 0, len(roots))
	for _, root := range roots {
		cleaned := filepath.Clean(root)
		if cleaned != "" && cleaned != "." && cleaned != "/" {
			out = append(out, cleaned+string(filepath.Separator))
		}
	}
	return out
}

func dedupeScopePaths(paths []string) []string {
	if len(paths) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(paths))
	out := make([]string, 0, len(paths))
	for _, path := range paths {
		path = filepath.ToSlash(filepath.Clean(path))
		if path == "." || path == "" {
			continue
		}
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		out = append(out, path)
	}
	return out
}
