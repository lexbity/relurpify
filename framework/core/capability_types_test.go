package core

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/platform/contracts"
	"github.com/stretchr/testify/require"
)

type descriptorTestTool struct{}

func (descriptorTestTool) Name() string        { return "demo_tool" }
func (descriptorTestTool) Description() string { return "demo" }
func (descriptorTestTool) Category() string    { return "testing" }
func (descriptorTestTool) Parameters() []ToolParameter {
	return []ToolParameter{
		{Name: "path", Type: "string", Required: true, Description: "path to inspect"},
		{Name: "limit", Type: "integer", Required: false, Default: 5},
	}
}
func (descriptorTestTool) Execute(ctx context.Context, state *contracts.Context, args map[string]interface{}) (*ToolResult, error) {
	return &ToolResult{Success: true}, nil
}
func (descriptorTestTool) IsAvailable(ctx context.Context, state *contracts.Context) bool {
	return true
}
func (descriptorTestTool) Permissions() ToolPermissions {
	return ToolPermissions{Permissions: &PermissionSet{
		FileSystem: []FileSystemPermission{
			{Action: FileSystemRead, Path: "."},
			{Action: FileSystemWrite, Path: "."},
		},
		Network: []NetworkPermission{
			{Direction: "egress", Protocol: "https", Host: "example.com"},
		},
	}}
}
func (descriptorTestTool) Tags() []string {
	return []string{TagReadOnly, TagNetwork, "lang:test", "verification"}
}

func TestToolDescriptorInfersCapabilityMetadata(t *testing.T) {
	desc := ToolDescriptor(context.Background(), nil, descriptorTestTool{})

	require.Equal(t, "tool:demo_tool", desc.ID)
	require.Equal(t, CapabilityKindTool, desc.Kind)
	require.Equal(t, CapabilityRuntimeFamilyLocalTool, desc.RuntimeFamily)
	require.Equal(t, "demo_tool", desc.Name)
	require.Equal(t, TrustClassBuiltinTrusted, desc.TrustClass)
	require.Contains(t, desc.RiskClasses, RiskClassDestructive)
	require.Contains(t, desc.RiskClasses, RiskClassNetwork)
	require.Contains(t, desc.EffectClasses, EffectClassFilesystemMutation)
	require.Contains(t, desc.EffectClasses, EffectClassNetworkEgress)
	require.Equal(t, []string{"lang:test", "verification"}, desc.Tags)
	require.NotNil(t, desc.InputSchema)
	require.Equal(t, "object", desc.InputSchema.Type)
	require.ElementsMatch(t, []string{"path"}, desc.InputSchema.Required)
	require.Equal(t, "integer", desc.InputSchema.Properties["limit"].Type)
	require.Equal(t, 5, desc.InputSchema.Properties["limit"].Default)
}

func TestNormalizeCapabilityDescriptorDefaultsRuntimeFamily(t *testing.T) {
	providerDesc := NormalizeCapabilityDescriptor(CapabilityDescriptor{
		ID:   "resource:remote",
		Kind: CapabilityKindResource,
		Source: CapabilitySource{
			ProviderID: "remote-mcp",
			Scope:      CapabilityScopeRemote,
		},
	})
	require.Equal(t, CapabilityRuntimeFamilyProvider, providerDesc.RuntimeFamily)

	relurpicDesc := NormalizeCapabilityDescriptor(CapabilityDescriptor{
		ID:   "prompt:planner",
		Kind: CapabilityKindPrompt,
		Name: "planner",
	})
	require.Equal(t, CapabilityRuntimeFamilyRelurpic, relurpicDesc.RuntimeFamily)
}

func TestNormalizeCapabilityDescriptorAppliesCoordinationMetadataDefaults(t *testing.T) {
	desc := NormalizeCapabilityDescriptor(CapabilityDescriptor{
		ID:   "prompt:planner",
		Kind: CapabilityKindPrompt,
		Name: "planner",
		InputSchema: &Schema{
			Type: "object",
			Properties: map[string]*Schema{
				"instruction": {Type: "string"},
			},
		},
		OutputSchema: &Schema{
			Type: "object",
			Properties: map[string]*Schema{
				"plan": {Type: "string"},
			},
		},
		Coordination: &CoordinationTargetMetadata{
			Target:         true,
			Role:           CoordinationRolePlanner,
			TaskTypes:      []string{"plan", "plan", " "},
			ExecutionModes: []CoordinationExecutionMode{CoordinationExecutionModeSync, CoordinationExecutionModeSync},
		},
	})

	require.NotNil(t, desc.Coordination)
	require.Equal(t, []string{"plan"}, desc.Coordination.TaskTypes)
	require.Equal(t, []CoordinationExecutionMode{CoordinationExecutionModeSync}, desc.Coordination.ExecutionModes)
	require.NotNil(t, desc.Coordination.ExpectedInput)
	require.NotNil(t, desc.Coordination.ExpectedOutput)

	desc.InputSchema.Properties["instruction"].Type = "integer"
	require.Equal(t, "string", desc.Coordination.ExpectedInput.Properties["instruction"].Type)
}

func TestValidateCoordinationTargetMetadataRejectsInvalidLongRunningSyncTarget(t *testing.T) {
	err := ValidateCoordinationTargetMetadata(&CoordinationTargetMetadata{
		Target:         true,
		Role:           CoordinationRoleReviewer,
		TaskTypes:      []string{"review"},
		ExecutionModes: []CoordinationExecutionMode{CoordinationExecutionModeSync},
		LongRunning:    true,
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "long-running")
}
