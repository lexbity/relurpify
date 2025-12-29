package toolsys

import (
	"context"
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/stretchr/testify/require"
)

type allowlistStubTool struct {
	name string
}

func (s allowlistStubTool) Name() string                     { return s.name }
func (s allowlistStubTool) Description() string              { return "stub" }
func (s allowlistStubTool) Category() string                 { return "misc" }
func (s allowlistStubTool) Parameters() []core.ToolParameter { return nil }
func (s allowlistStubTool) Execute(ctx context.Context, state *core.Context, args map[string]interface{}) (*core.ToolResult, error) {
	return &core.ToolResult{Success: true}, nil
}
func (s allowlistStubTool) IsAvailable(ctx context.Context, state *core.Context) bool { return true }
func (s allowlistStubTool) Permissions() core.ToolPermissions {
	return core.ToolPermissions{Permissions: &core.PermissionSet{
		FileSystem: []core.FileSystemPermission{{Action: core.FileSystemRead, Path: "/tmp/**"}},
	}}
}

func TestAllowedToolsAppliedOnRegister(t *testing.T) {
	registry := NewToolRegistry()
	spec := &AgentRuntimeSpec{AllowedTools: []string{"keep_tool"}}
	registry.UseAgentSpec("agent", spec)

	require.NoError(t, registry.Register(allowlistStubTool{name: "keep_tool"}))
	require.NoError(t, registry.Register(allowlistStubTool{name: "drop_tool"}))

	_, ok := registry.Get("keep_tool")
	require.True(t, ok)
	_, ok = registry.Get("drop_tool")
	require.False(t, ok)
}

func TestAllowedToolsAppliedToExistingTools(t *testing.T) {
	registry := NewToolRegistry()
	require.NoError(t, registry.Register(allowlistStubTool{name: "keep_tool"}))
	require.NoError(t, registry.Register(allowlistStubTool{name: "drop_tool"}))

	spec := &AgentRuntimeSpec{AllowedTools: []string{"keep_tool"}}
	registry.UseAgentSpec("agent", spec)

	_, ok := registry.Get("keep_tool")
	require.True(t, ok)
	_, ok = registry.Get("drop_tool")
	require.False(t, ok)
}
