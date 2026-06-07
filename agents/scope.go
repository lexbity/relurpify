package agents

import (
	"codeburg.org/lexbit/relurpify/capability"
	"codeburg.org/lexbit/relurpify/capability/ports"
	"codeburg.org/lexbit/relurpify/governance/permissions"
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
	cloned := registry.CloneFiltered(func(tool ports.Tool) bool {
		return toolAllowed(tool, scope)
	})
	if len(scope.WritePathGlobs) > 0 {
		cloned.AddPrecheck(capability.WritePathPrecheck{Globs: append([]string{}, scope.WritePathGlobs...)})
	}
	// Phase 9: default doom-loop detector — blocks after 3 identical failing calls.
	loopCheck := capability.NewDoomLoopPrecheck()
	cloned.AddPrecheck(loopCheck)
	cloned.AddPostcheck(loopCheck)
	return cloned
}

// toolAllowed reports whether the tool's declared permissions fit within scope.
func toolAllowed(tool ports.Tool, scope ToolScope) bool {
	perms := tool.Permissions()
	ps := permissionSet(perms.Permissions)
	if ps == nil {
		return true
	}
	for _, fs := range ps.FileSystem {
		switch fs.Action {
		case permissions.FileSystemWrite:
			if !scope.AllowWrite {
				return false
			}
		case permissions.FileSystemExecute:
			if !scope.AllowExecute {
				return false
			}
		}
	}
	if len(ps.Executables) > 0 && !scope.AllowExecute {
		return false
	}
	if len(ps.Network) > 0 && !scope.AllowNetwork {
		return false
	}
	return true
}

func permissionSet(v interface{}) *permissions.PermissionSet {
	if v == nil {
		return nil
	}
	ps, _ := v.(*permissions.PermissionSet)
	return ps
}
