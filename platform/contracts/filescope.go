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
	for _, path := range protectedPaths {
		if resolved := canonicalScopePath(path); resolved != "" {
			policy.ProtectedPaths = append(policy.ProtectedPaths, resolved)
		}
	}
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
	if resolved, err := filepath.EvalSymlinks(clean); err == nil {
		clean = resolved
	}
	return filepath.ToSlash(filepath.Clean(clean))
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
