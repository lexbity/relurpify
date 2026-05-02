package core

import (
	"path/filepath"
	"strings"

	"codeburg.org/lexbit/relurpify/platform/contracts"
)

// NewFileSystemPermissionSet builds a permission set for the provided actions scoped to base.
func NewFileSystemPermissionSet(base string, actions ...contracts.FileSystemAction) *contracts.PermissionSet {
	scope := computeWorkspaceScope(base)
	perms := make([]contracts.FileSystemPermission, 0, len(actions))
	for _, action := range actions {
		perms = append(perms, contracts.FileSystemPermission{
			Action: action,
			Path:   scope,
		})
	}
	return &contracts.PermissionSet{
		FileSystem: perms,
	}
}

// NewExecutionPermissionSet extends filesystem permissions with execution metadata.
func NewExecutionPermissionSet(base string, binary string, args []string) *contracts.PermissionSet {
	perms := NewFileSystemPermissionSet(base, contracts.FileSystemRead, contracts.FileSystemWrite, contracts.FileSystemExecute, contracts.FileSystemList)
	perms.Executables = append(perms.Executables, contracts.ExecutablePermission{
		Binary: binary,
		Args:   normalizeArgs(args),
	})
	return perms
}

// computeWorkspaceScope normalizes the workspace path into a glob that grants
// access to every file inside the directory tree without accidentally escaping
// to parent directories.
func computeWorkspaceScope(base string) string {
	if base == "" || base == "." {
		return "**"
	}
	clean := filepath.ToSlash(filepath.Clean(base))
	if clean == "." || clean == "" {
		return "**"
	}
	clean = strings.TrimSuffix(clean, "/")
	return clean + "/**"
}

// normalizeArgs replaces empty arguments with wildcards so permission entries
// match invocations even when optional flags are omitted.
func normalizeArgs(args []string) []string {
	if len(args) == 0 {
		return nil
	}
	normalized := make([]string, len(args))
	for i, arg := range args {
		if arg == "" {
			normalized[i] = "*"
			continue
		}
		normalized[i] = arg
	}
	return normalized
}
