package authorization

import (
	"codeburg.org/lexbit/relurpify/capability/ports"
)

// ToolFromPorts wraps a ports.Tool into an authorization.Tool so the
// permission manager can perform its interface assertion.
func ToolFromPorts(t ports.Tool) Tool {
	return &portsToolAdapter{t: t}
}

type portsToolAdapter struct {
	t ports.Tool
}

func (a *portsToolAdapter) Name() string   { return a.t.Name() }
func (a *portsToolAdapter) Tags() []string { return a.t.Tags() }
func (a *portsToolAdapter) Permissions() ToolPermissions {
	return ToolPermissions{
		Permissions: a.t.Permissions().Permissions,
	}
}
