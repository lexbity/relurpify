package agents

import (
	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
)

// ToolScope defines the permission envelope used to filter a capability
// registry to only the tools appropriate for a given execution context.
type ToolScope struct {
	AllowRead      bool
	AllowWrite     bool
	AllowExecute   bool
	AllowNetwork   bool
	WritePathGlobs []string
}

// ScopeRegistry clones the registry, removing tools outside the given scope.
// If WritePathGlobs is non-empty a WritePathPrecheck is attached so path
// restrictions are enforced at invocation time.
func ScopeRegistry(registry *capability.Registry, scope ToolScope) *capability.Registry {
	if registry == nil {
		return capability.NewRegistry()
	}
	cloned := registry.CloneFiltered(func(tool core.Tool) bool {
		return toolAllowed(tool, scope)
	})
	if len(scope.WritePathGlobs) > 0 {
		cloned.AddPrecheck(capability.WritePathPrecheck{Globs: append([]string{}, scope.WritePathGlobs...)})
	}
	return cloned
}

// toolAllowed reports whether the tool's declared permissions fit within scope.
func toolAllowed(tool core.Tool, scope ToolScope) bool {
	perms := tool.Permissions()
	if perms.Permissions == nil {
		return true
	}
	for _, fs := range perms.Permissions.FileSystem {
		switch fs.Action {
		case core.FileSystemWrite:
			if !scope.AllowWrite {
				return false
			}
		case core.FileSystemExecute:
			if !scope.AllowExecute {
				return false
			}
		}
	}
	if len(perms.Permissions.Executables) > 0 && !scope.AllowExecute {
		return false
	}
	if len(perms.Permissions.Network) > 0 && !scope.AllowNetwork {
		return false
	}
	return true
}
