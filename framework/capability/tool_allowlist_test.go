package capability

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/agentspec"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/platform/contracts"
	"github.com/stretchr/testify/require"
)

type allowlistStubTool struct {
	name string
}

func (s allowlistStubTool) Name() string                     { return s.name }
func (s allowlistStubTool) Description() string              { return "stub" }
func (s allowlistStubTool) Category() string                 { return "misc" }
func (s allowlistStubTool) Parameters() []core.ToolParameter { return nil }
func (s allowlistStubTool) Execute(ctx context.Context, state *contracts.Context, args map[string]interface{}) (*core.ToolResult, error) {
	return &core.ToolResult{Success: true}, nil
}
func (s allowlistStubTool) IsAvailable(ctx context.Context, state *contracts.Context) bool {
	return true
}
func (s allowlistStubTool) Permissions() core.ToolPermissions {
	return core.ToolPermissions{Permissions: &core.PermissionSet{
		FileSystem: []core.FileSystemPermission{{Action: core.FileSystemRead, Path: "/tmp/**"}},
	}}
}
func (s allowlistStubTool) Tags() []string { return nil }

func TestAllowedCapabilitiesAppliedOnRegister(t *testing.T) {
	registry := NewCapabilityRegistry()
	spec := &AgentRuntimeSpec{AllowedCapabilities: []agentspec.CapabilitySelector{{Name: "keep_tool", Kind: core.CapabilityKindTool}}}
	registry.UseAgentSpec("agent", spec)

	require.NoError(t, registry.Register(allowlistStubTool{name: "keep_tool"}))
	require.NoError(t, registry.Register(allowlistStubTool{name: "drop_tool"}))

	_, ok := registry.Get("keep_tool")
	require.True(t, ok)
	_, ok = registry.Get("drop_tool")
	require.False(t, ok)
}

func TestAllowedCapabilitiesAppliedToExistingTools(t *testing.T) {
	registry := NewCapabilityRegistry()
	require.NoError(t, registry.Register(allowlistStubTool{name: "keep_tool"}))
	require.NoError(t, registry.Register(allowlistStubTool{name: "drop_tool"}))

	spec := &AgentRuntimeSpec{AllowedCapabilities: []agentspec.CapabilitySelector{{Name: "keep_tool", Kind: core.CapabilityKindTool}}}
	registry.UseAgentSpec("agent", spec)

	_, ok := registry.Get("keep_tool")
	require.True(t, ok)
	_, ok = registry.Get("drop_tool")
	require.False(t, ok)
}

func TestAllowedCapabilitiesMatchDescriptorTags(t *testing.T) {
	registry := NewCapabilityRegistry()
	require.NoError(t, registry.Register(allowlistStubTool{name: "go_test"}))
	require.NoError(t, registry.RegisterCapability(core.CapabilityDescriptor{
		ID:   "prompt:go",
		Kind: core.CapabilityKindPrompt,
		Name: "go_prompt",
		Tags: []string{"lang:go"},
	}))

	registry.RestrictToCapabilities([]core.CapabilitySelector{core.CapabilitySelectorFromAgentSpec(agentspec.CapabilitySelector{Tags: []string{"lang:go"}})})

	_, ok := registry.Get("go_test")
	require.False(t, ok)
	_, ok = registry.GetCapability("prompt:go")
	require.True(t, ok)
}
