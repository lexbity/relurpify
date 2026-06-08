package sandbox

import "codeburg.org/lexbit/relurpify/governance/permissions"

var (
	ErrFileScopeOutsideWorkspace = permissions.ErrFileScopeOutsideWorkspace
	ErrFileScopeProtectedPath    = permissions.ErrFileScopeProtectedPath
)

func NewFileScopePolicy(workspace string, protectedPaths []string) *permissions.FileScopePolicy {
	return permissions.NewFileScopePolicy(workspace, protectedPaths)
}
