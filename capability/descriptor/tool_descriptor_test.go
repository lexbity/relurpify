package descriptor

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/capability/ports"
	"codeburg.org/lexbit/relurpify/governance/classification"
)

func TestToolDescriptor_NilToolYieldsZeroDescriptor(t *testing.T) {
	assert.Equal(t, CapabilityDescriptor{}, ToolDescriptor(context.Background(), nil))
}

func TestToolDescriptor_BareToolDerivesEverything(t *testing.T) {
	tool := &bareTool{
		name:        "fs-read",
		description: "reads a file",
		category:    "filesystem",
		params: []ports.ToolParameter{
			{Name: "path", Type: "string", Required: true},
		},
		available: true,
		perms:     permSet(true, 0, 0),
		tags:      []string{"Files", "READ_ONLY"},
	}

	desc := ToolDescriptor(context.Background(), tool)

	assert.Equal(t, "tool:fs-read", desc.ID)
	assert.Equal(t, agentspec.CapabilityKindTool, desc.Kind)
	assert.Equal(t, agentspec.CapabilityRuntimeFamilyLocalTool, desc.RuntimeFamily)
	assert.Equal(t, "fs-read", desc.Name)
	assert.Equal(t, "reads a file", desc.Description)
	assert.Equal(t, "filesystem", desc.Category)
	assert.Equal(t, []string{"files"}, desc.Tags, "reserved security tag dropped, rest folded")
	assert.Equal(t, CapabilitySource{Scope: classification.CapabilityScopeBuiltin}, desc.Source)
	assert.Equal(t, agentspec.TrustClassBuiltinTrusted, desc.TrustClass)
	assert.Equal(t, []classification.EffectClass{classification.EffectClassFilesystemMutation}, desc.EffectClasses)
	require.NotNil(t, desc.InputSchema)
	assert.Equal(t, []string{"path"}, desc.InputSchema.Required)
	assert.True(t, desc.Availability.Available)
	assert.Nil(t, desc.Coordination)
	assert.Empty(t, desc.SessionAffinity)
	assert.Empty(t, desc.Version)
}

func TestToolDescriptor_UnavailableToolReportsUnavailable(t *testing.T) {
	desc := ToolDescriptor(context.Background(), &bareTool{name: "x", available: false})
	assert.False(t, desc.Availability.Available)
}

func TestToolDescriptor_OptionalProvidersFlowThrough(t *testing.T) {
	tool := &stubTool{
		bareTool:      bareTool{name: "wrapped", description: "ignored", category: "ignored"},
		capabilityID:  "remote.search",
		version:       "3.0.0",
		source:        CapabilitySource{ProviderID: "prov-1", Scope: classification.CapabilityScopeProvider},
		runtimeFamily: agentspec.CapabilityRuntimeFamilyProvider,
		trust:         agentspec.TrustClassRemoteDeclared,
		effects:       []classification.EffectClass{classification.EffectClassNetworkEgress},
		affinity:      "session-42",
		coordination:  &CoordinationTargetMetadata{Target: true, Role: agentspec.CoordinationRoleExecutor},
	}

	desc := ToolDescriptor(context.Background(), tool)

	assert.Equal(t, "remote.search", desc.ID)
	assert.Equal(t, "3.0.0", desc.Version)
	assert.Equal(t, "prov-1", desc.Source.ProviderID)
	assert.Equal(t, agentspec.CapabilityRuntimeFamilyProvider, desc.RuntimeFamily)
	assert.Equal(t, agentspec.TrustClassRemoteDeclared, desc.TrustClass)
	assert.Equal(t, []classification.EffectClass{classification.EffectClassNetworkEgress}, desc.EffectClasses)
	assert.Equal(t, "session-42", desc.SessionAffinity)
	require.NotNil(t, desc.Coordination)
	assert.True(t, desc.Coordination.Target)
	// The availability falls back to the tool: stubTool defaults available=false.
	assert.False(t, desc.Availability.Available)
}

func TestToolDescriptor_ProviderBranchFillsOnlyMissingFields(t *testing.T) {
	tool := &descriptorProviderTool{
		stubTool: stubTool{
			bareTool: bareTool{
				name:        "fs-write",
				description: "declared description",
				category:    "filesystem",
				available:   true,
			},
			capabilityID: "fs.write",
			affinity:     "sess-7",
		},
		desc: CapabilityDescriptor{
			ID:           "declared.id",
			Name:         "Declared Name",
			Availability: AvailabilitySpec{Reason: "disabled by config"},
			Coordination: &CoordinationTargetMetadata{Target: true, Role: agentspec.CoordinationRolePlanner},
		},
	}

	desc := ToolDescriptor(context.Background(), tool)

	// Declared fields win.
	assert.Equal(t, "declared.id", desc.ID)
	assert.Equal(t, "Declared Name", desc.Name)
	assert.False(t, desc.Availability.Available,
		"a declared unavailability reason must not be overridden by IsAvailable")
	assert.Equal(t, "disabled by config", desc.Availability.Reason)
	require.NotNil(t, desc.Coordination)
	assert.Equal(t, agentspec.CoordinationRolePlanner, desc.Coordination.Role)

	// Missing fields are derived from the tool.
	assert.Equal(t, agentspec.CapabilityKindTool, desc.Kind, "empty kind defaults to tool")
	assert.Equal(t, agentspec.CapabilityRuntimeFamilyLocalTool, desc.RuntimeFamily)
	assert.Equal(t, "declared description", desc.Description, "empty description falls back to the tool")
	assert.Equal(t, "filesystem", desc.Category)
	assert.Equal(t, agentspec.TrustClassBuiltinTrusted, desc.TrustClass)
	require.NotNil(t, desc.InputSchema, "empty input schema falls back to the tool parameters")

	// The provider branch never consults SessionAffinityProvider — that
	// derivation only happens on the plain-tool branch — so the declared
	// (empty) affinity is preserved as-is.
	assert.Empty(t, desc.SessionAffinity)
}

func TestToolDescriptor_ProviderBranchDerivesIDWhenEmpty(t *testing.T) {
	tool := &descriptorProviderTool{
		stubTool: stubTool{bareTool: bareTool{name: "derived-name", available: true}},
		desc:     CapabilityDescriptor{Name: "Has Name"},
	}
	desc := ToolDescriptor(context.Background(), tool)
	assert.Equal(t, "tool:derived-name", desc.ID, "empty declared ID falls back to the identity provider, then the name")
	assert.Equal(t, "Has Name", desc.Name)
	assert.True(t, desc.Availability.Available,
		"an undeclared availability is filled in from the tool when it is available")
}

func TestToolDescriptor_ProviderBranchDerivesTagsEffectsAndCoordination(t *testing.T) {
	tool := &descriptorProviderTool{
		stubTool: stubTool{
			bareTool: bareTool{
				name: "t",
				tags: []string{"Helper"},
			},
			coordination: &CoordinationTargetMetadata{Target: true, Role: agentspec.CoordinationRoleVerifier},
		},
		desc: CapabilityDescriptor{ID: "x"},
	}
	desc := ToolDescriptor(context.Background(), tool)
	assert.Equal(t, []string{"helper"}, desc.Tags, "empty declared tags fall back to tool tags")
	require.NotNil(t, desc.Coordination, "nil declared coordination falls back to the provider")
	assert.Equal(t, agentspec.CoordinationRoleVerifier, desc.Coordination.Role)
}
