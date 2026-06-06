package contracts

import (
	"codeburg.org/lexbit/relurpify/governance/permissions"
)

type FileScopePolicy = permissions.FileScopePolicy
type FileScopeError = permissions.FileScopeError

var (
	ErrFileScopeOutsideWorkspace = permissions.ErrFileScopeOutsideWorkspace
	ErrFileScopeProtectedPath    = permissions.ErrFileScopeProtectedPath
)

func NewFileScopePolicy(workspace string, protectedPaths []string) *FileScopePolicy {
	return permissions.NewFileScopePolicy(workspace, protectedPaths)
}
