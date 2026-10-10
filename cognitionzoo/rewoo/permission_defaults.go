package rewoo

import (
	"context"

	capability "codeburg.org/lexbit/relurpify/capability/registry"
	ucperms "codeburg.org/lexbit/relurpify/userconfig/permissions"
)

// DefaultPermissionSet builds a minimal permission set that allows workspace read/write
// and grants all registered tool capabilities.
func DefaultPermissionSet(registry *capability.CapabilityRegistry, workspacePath string) *ucperms.PermissionSet {
	perm := &ucperms.PermissionSet{
		// Allow read/write on workspace
		FileSystem: []ucperms.FileSystemPermission{
			{
				Action: ucperms.FileSystemRead,
				Path:   workspacePath + "/**",
			},
			{
				Action: ucperms.FileSystemWrite,
				Path:   workspacePath + "/**",
			},
		},
	}

	// Add all registered tools as capabilities
	if registry != nil {
		tools := registry.All(context.Background())
		for _, tool := range tools {
			perm.Capabilities = append(perm.Capabilities, ucperms.CapabilityPermission{
				Capability: tool.Name(),
			})
		}
	}

	return perm
}

// RestrictedPermissionSet builds a permission set that only allows specific tools.
// Useful for creating sandboxed execution contexts.
func RestrictedPermissionSet(workspacePath string, allowedTools []string) *ucperms.PermissionSet {
	perm := &ucperms.PermissionSet{
		FileSystem: []ucperms.FileSystemPermission{
			{
				Action: ucperms.FileSystemRead,
				Path:   workspacePath + "/**",
			},
			{
				Action: ucperms.FileSystemWrite,
				Path:   workspacePath + "/**",
			},
		},
	}

	for _, tool := range allowedTools {
		perm.Capabilities = append(perm.Capabilities, ucperms.CapabilityPermission{
			Capability: tool,
		})
	}

	return perm
}

// ReadOnlyPermissionSet builds a permission set that only allows file reads.
// Useful for analysis-only workflows.
func ReadOnlyPermissionSet(workspacePath string) *ucperms.PermissionSet {
	return &ucperms.PermissionSet{
		FileSystem: []ucperms.FileSystemPermission{
			{
				Action: ucperms.FileSystemRead,
				Path:   workspacePath + "/**",
			},
		},
	}
}
