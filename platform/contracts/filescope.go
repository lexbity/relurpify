package contracts

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

var (
	// ErrFileScopeOutsideWorkspace indicates a path escaped the sandbox workspace.
	ErrFileScopeOutsideWorkspace = errors.New("outside workspace")
	// ErrFileScopeProtectedPath indicates a protected root was targeted.
	ErrFileScopeProtectedPath = errors.New("protected path")
)

// FileScopePolicy captures the filesystem boundary enforced by sandbox-aware
// tools before host I/O occurs.
type FileScopePolicy struct {
	Workspace      string
	ProtectedPaths []string
}

// NewFileScopePolicy canonicalizes a workspace root and protected roots for
// deterministic scope checks.
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

// FileScopeError reports sandbox filesystem boundary violations.
type FileScopeError struct {
	Action string
	Path   string
	Root   string
	Reason string
}

func (e *FileScopeError) Error() string {
	if e == nil {
		return ""
	}
	if e.Root != "" {
		return fmt.Sprintf("%s: %s (root: %s)", e.Reason, e.Path, e.Root)
	}
	return fmt.Sprintf("%s: %s", e.Reason, e.Path)
}

func (e *FileScopeError) Unwrap() error {
	if e == nil {
		return nil
	}
	switch e.Reason {
	case ErrFileScopeProtectedPath.Error():
		return ErrFileScopeProtectedPath
	default:
		return ErrFileScopeOutsideWorkspace
	}
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
	// Resolve symlinks for the longest existing prefix, then re-append
	// any non-existent tail. This closes the symlinked-parent write escape
	// (SEC-5): a path like /workspace/link/newfile where link -> /etc would
	// previously fall back to the lexical path because newfile doesn't exist.
	if resolved, err := filepath.EvalSymlinks(clean); err == nil {
		clean = resolved
	} else {
		clean = resolveExistingPrefix(clean)
	}
	return filepath.ToSlash(filepath.Clean(clean))
}

// resolveExistingPrefix walks up the path to find the longest existing ancestor
// that can be resolved through symlinks, then re-appends the non-existent tail.
// Iterative (stack-based) to avoid deep recursion on long paths.
func resolveExistingPrefix(p string) string {
	if p == "" || p == "/" {
		return p
	}
	// Walk up collecting path segments until we find a prefix that EvalSymlinks
	// can resolve. The resolved prefix replaces the equivalent lexical portion;
	// the remaining segments are re-appended as-is.
	var tail []string
	current := filepath.Clean(p)
	for {
		if current == "" || current == "/" {
			break
		}
		if resolved, err := filepath.EvalSymlinks(current); err == nil {
			// Re-append tail segments in reverse order.
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
	// No prefix could be resolved — return the original path unchanged.
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
