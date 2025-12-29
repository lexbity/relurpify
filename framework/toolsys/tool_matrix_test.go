package toolsys

import (
	"context"
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/stretchr/testify/require"
)

type stubTool struct {
	name     string
	category string
	perms    core.ToolPermissions
}

func (s stubTool) Name() string                     { return s.name }
func (s stubTool) Description() string              { return "stub" }
func (s stubTool) Category() string                 { return s.category }
func (s stubTool) Parameters() []core.ToolParameter { return nil }
func (s stubTool) Execute(ctx context.Context, state *core.Context, args map[string]interface{}) (*core.ToolResult, error) {
	return &core.ToolResult{Success: true}, nil
}
func (s stubTool) IsAvailable(ctx context.Context, state *core.Context) bool { return true }
func (s stubTool) Permissions() core.ToolPermissions                         { return s.perms }

func TestToolMatrixAppliedOnRegisterAfterSpec(t *testing.T) {
	registry := NewToolRegistry()
	spec := &AgentRuntimeSpec{
		Tools: AgentToolMatrix{
			FileRead:  true,
			FileWrite: false,
		},
	}
	registry.UseAgentSpec("agent", spec)

	readTool := stubTool{
		name:     "read_tool",
		category: "file",
		perms: core.ToolPermissions{Permissions: &core.PermissionSet{
			FileSystem: []core.FileSystemPermission{{Action: core.FileSystemRead, Path: "/tmp/**"}},
		}},
	}
	writeTool := stubTool{
		name:     "write_tool",
		category: "file",
		perms: core.ToolPermissions{Permissions: &core.PermissionSet{
			FileSystem: []core.FileSystemPermission{{Action: core.FileSystemWrite, Path: "/tmp/**"}},
		}},
	}

	require.NoError(t, registry.Register(readTool))
	require.NoError(t, registry.Register(writeTool))

	_, ok := registry.Get("read_tool")
	require.True(t, ok)
	_, ok = registry.Get("write_tool")
	require.False(t, ok)
}

func TestToolMatrixAppliedOnRegisterAfterRestrict(t *testing.T) {
	registry := NewToolRegistry()
	RestrictToolRegistryByMatrix(registry, AgentToolMatrix{FileRead: true, FileWrite: false})

	writeTool := stubTool{
		name:     "write_tool",
		category: "file",
		perms: core.ToolPermissions{Permissions: &core.PermissionSet{
			FileSystem: []core.FileSystemPermission{{Action: core.FileSystemWrite, Path: "/tmp/**"}},
		}},
	}

	require.NoError(t, registry.Register(writeTool))
	_, ok := registry.Get("write_tool")
	require.False(t, ok)
}

func TestToolMatrixDefaultDenyForUnclassifiedTool(t *testing.T) {
	registry := NewToolRegistry()
	RestrictToolRegistryByMatrix(registry, AgentToolMatrix{FileRead: true})

	miscTool := stubTool{
		name:     "misc_tool",
		category: "misc",
		perms:    core.ToolPermissions{},
	}

	require.NoError(t, registry.Register(miscTool))
	_, ok := registry.Get("misc_tool")
	require.False(t, ok)
}

func TestToolPolicyVisibleOverridesMatrix(t *testing.T) {
	registry := NewToolRegistry()
	visible := true
	spec := &AgentRuntimeSpec{
		Tools: AgentToolMatrix{
			FileRead:  false,
			FileWrite: false,
		},
		ToolPolicies: map[string]ToolPolicy{
			"special_tool": {Visible: &visible},
		},
	}
	registry.UseAgentSpec("agent", spec)

	tool := stubTool{
		name:     "special_tool",
		category: "misc",
		perms:    core.ToolPermissions{},
	}

	require.NoError(t, registry.Register(tool))
	_, ok := registry.Get("special_tool")
	require.True(t, ok)
}
