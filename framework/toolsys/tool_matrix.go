package toolsys

// RestrictToolRegistryByMatrix removes tools that are disabled by the manifest's
// coarse tool matrix. This is separate from PermissionManager enforcement: the
// matrix determines which tools an agent can *see*, while permissions determine
// whether a visible tool is allowed to execute.
func RestrictToolRegistryByMatrix(registry *ToolRegistry, matrix AgentToolMatrix) {
	if registry == nil {
		return
	}
	registry.setToolMatrix(matrix)
	allowed := make([]string, 0)
	for _, tool := range registry.All() {
		if toolVisibleByMatrixPolicy(tool, matrix, registry.toolPolicies) {
			allowed = append(allowed, tool.Name())
		}
	}
	registry.RestrictTo(allowed)
}

func toolVisibleByMatrixPolicy(tool Tool, matrix AgentToolMatrix, policies map[string]ToolPolicy) bool {
	if tool == nil {
		return false
	}
	if policies != nil {
		if policy, ok := policies[tool.Name()]; ok && policy.Visible != nil {
			return *policy.Visible
		}
	}
	return toolAllowedByMatrix(tool, matrix)
}

func toolAllowedByMatrix(tool Tool, matrix AgentToolMatrix) bool {
	if tool == nil {
		return false
	}
	perms := tool.Permissions().Permissions
	allowedByPerms := false
	if perms != nil {
		if permissionRequiresFileRead(perms) && !matrix.FileRead {
			return false
		}
		if permissionRequiresFileWrite(perms) && !matrix.FileWrite {
			return false
		}
		if permissionRequiresExecute(perms) && !matrix.BashExecute {
			return false
		}
		if permissionRequiresNetwork(perms) && !matrix.WebSearch {
			return false
		}
		allowedByPerms = true
	}

	switch tool.Category() {
	case "lsp":
		return matrix.LSPQuery
	case "search":
		return matrix.SearchCodebase
	case "git", "execution":
		return matrix.BashExecute
	default:
		return allowedByPerms
	}
}

func permissionRequiresFileRead(perms *PermissionSet) bool {
	for _, fs := range perms.FileSystem {
		if fs.Action == FileSystemRead || fs.Action == FileSystemList {
			return true
		}
	}
	return false
}

func permissionRequiresFileWrite(perms *PermissionSet) bool {
	for _, fs := range perms.FileSystem {
		if fs.Action == FileSystemWrite {
			return true
		}
	}
	return false
}

func permissionRequiresExecute(perms *PermissionSet) bool {
	for _, fs := range perms.FileSystem {
		if fs.Action == FileSystemExecute {
			return true
		}
	}
	return len(perms.Executables) > 0
}

func permissionRequiresNetwork(perms *PermissionSet) bool {
	return len(perms.Network) > 0
}
