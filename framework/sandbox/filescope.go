package sandbox

import (
	"codeburg.org/lexbit/relurpify/platform/contracts"
)

// Re-export FileScope types from platform/contracts for backward compatibility

// FileScopePolicy captures the filesystem boundary enforced by sandbox-aware
// tools before host I/O occurs.
type FileScopePolicy = contracts.FileScopePolicy

// FileScopeError reports sandbox filesystem boundary violations.
type FileScopeError = contracts.FileScopeError

// ErrFileScopeOutsideWorkspace indicates a path escaped the sandbox workspace.
var ErrFileScopeOutsideWorkspace = contracts.ErrFileScopeOutsideWorkspace

// ErrFileScopeProtectedPath indicates a protected root was targeted.
var ErrFileScopeProtectedPath = contracts.ErrFileScopeProtectedPath

// NewFileScopePolicy canonicalizes a workspace root and protected roots for
// deterministic scope checks.
func NewFileScopePolicy(workspace string, protectedPaths []string) *FileScopePolicy {
	return contracts.NewFileScopePolicy(workspace, protectedPaths)
}
