package rewoo

import (
	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/platform/contracts"
)

// DefaultPermissionSet builds a minimal permission set that allows workspace read/write
// and grants all registered tool capabilities.
func DefaultPermissionSet(registry *capability.Registry, workspacePath string) *contracts.PermissionSet {
	perm := &contracts.PermissionSet{
		// Allow read/write on workspace
		FileSystem: []contracts.FileSystemPermission{
			{
				Action: contracts.FileSystemRead,
				Path:   workspacePath + "/**",
			},
			{
				Action: contracts.FileSystemWrite,
				Path:   workspacePath + "/**",
			},
		},
	}

	// Add all registered tools as capabilities
	if registry != nil {
		tools := registry.All()
		for _, tool := range tools {
			perm.Capabilities = append(perm.Capabilities, contracts.CapabilityPermission{
				Capability: tool.Name(),
			})
		}
	}

	return perm
}

// RestrictedPermissionSet builds a permission set that only allows specific tools.
// Useful for creating sandboxed execution contexts.
func RestrictedPermissionSet(workspacePath string, allowedTools []string) *contracts.PermissionSet {
	perm := &contracts.PermissionSet{
		FileSystem: []contracts.FileSystemPermission{
			{
				Action: contracts.FileSystemRead,
				Path:   workspacePath + "/**",
			},
			{
				Action: contracts.FileSystemWrite,
				Path:   workspacePath + "/**",
			},
		},
	}

	for _, tool := range allowedTools {
		perm.Capabilities = append(perm.Capabilities, contracts.CapabilityPermission{
			Capability: tool,
		})
	}

	return perm
}

// ReadOnlyPermissionSet builds a permission set that only allows file reads.
// Useful for analysis-only workflows.
func ReadOnlyPermissionSet(workspacePath string) *contracts.PermissionSet {
	return &contracts.PermissionSet{
		FileSystem: []contracts.FileSystemPermission{
			{
				Action: contracts.FileSystemRead,
				Path:   workspacePath + "/**",
			},
		},
	}
}
