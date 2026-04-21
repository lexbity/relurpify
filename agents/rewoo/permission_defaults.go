package rewoo

import (
	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/core"
)

// DefaultPermissionSet builds a minimal permission set that allows workspace read/write
// and grants all registered tool capabilities.
func DefaultPermissionSet(registry *capability.Registry, workspacePath string) *core.PermissionSet {
	perm := &core.PermissionSet{
		// Allow read/write on workspace
		FileSystem: []core.FileSystemPermission{
			{
				Action: core.FileSystemRead,
				Path:   workspacePath + "/**",
			},
			{
				Action: core.FileSystemWrite,
				Path:   workspacePath + "/**",
			},
		},
	}

	// Add all registered tools as capabilities
	if registry != nil {
		tools := registry.All()
		for _, tool := range tools {
			perm.Capabilities = append(perm.Capabilities, core.CapabilityPermission{
				Capability: tool.Name(),
			})
		}
	}

	return perm
}

// RestrictedPermissionSet builds a permission set that only allows specific tools.
// Useful for creating sandboxed execution contexts.
func RestrictedPermissionSet(workspacePath string, allowedTools []string) *core.PermissionSet {
	perm := &core.PermissionSet{
		FileSystem: []core.FileSystemPermission{
			{
				Action: core.FileSystemRead,
				Path:   workspacePath + "/**",
			},
			{
				Action: core.FileSystemWrite,
				Path:   workspacePath + "/**",
			},
		},
	}

	for _, tool := range allowedTools {
		perm.Capabilities = append(perm.Capabilities, core.CapabilityPermission{
			Capability: tool,
		})
	}

	return perm
}

// ReadOnlyPermissionSet builds a permission set that only allows file reads.
// Useful for analysis-only workflows.
func ReadOnlyPermissionSet(workspacePath string) *core.PermissionSet {
	return &core.PermissionSet{
		FileSystem: []core.FileSystemPermission{
			{
				Action: core.FileSystemRead,
				Path:   workspacePath + "/**",
			},
		},
	}
}
