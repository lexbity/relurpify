package core

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMergeAgentSpecsMergesCoordinationConfig(t *testing.T) {
	base := &AgentRuntimeSpec{
		Coordination: AgentCoordinationSpec{
			Enabled:            true,
			MaxDelegationDepth: 2,
			DelegationTargetSelectors: []CapabilitySelector{
				{Name: "planner", CoordinationRoles: []CoordinationRole{CoordinationRolePlanner}},
			},
			Projection: AgentProjectionPolicy{
				Strategy: "balanced",
				Hot: AgentProjectionTier{
					MaxTokens:      512,
					ResourceScopes: []string{"workflow.current"},
				},
			},
		},
	}
	overlay := AgentSpecOverlay{
		Coordination: &AgentCoordinationSpec{
			AllowBackgroundDelegation: true,
			DelegationTargetSelectors: []CapabilitySelector{
				{Name: "reviewer", CoordinationRoles: []CoordinationRole{CoordinationRoleReviewer}},
			},
			Projection: AgentProjectionPolicy{
				Cold: AgentProjectionTier{
					Persist:        true,
					ResourceScopes: []string{"workflow.archive"},
				},
			},
		},
	}

	merged := MergeAgentSpecs(base, overlay)

	require.True(t, merged.Coordination.Enabled)
	require.True(t, merged.Coordination.AllowBackgroundDelegation)
	require.Equal(t, 2, merged.Coordination.MaxDelegationDepth)
	require.Len(t, merged.Coordination.DelegationTargetSelectors, 2)
	require.Equal(t, []string{"workflow.current"}, merged.Coordination.Projection.Hot.ResourceScopes)
	require.True(t, merged.Coordination.Projection.Cold.Persist)
	require.Equal(t, []string{"workflow.archive"}, merged.Coordination.Projection.Cold.ResourceScopes)
}

func TestAgentSpecOverlayFromSpecClonesCoordinationConfig(t *testing.T) {
	spec := &AgentRuntimeSpec{
		Coordination: AgentCoordinationSpec{
			Enabled: true,
			DelegationTargetSelectors: []CapabilitySelector{
				{
					Name:                    "reviewer",
					CoordinationRoles:       []CoordinationRole{CoordinationRoleReviewer},
					CoordinationTaskTypes:   []string{"review"},
					CoordinationLongRunning: boolPtr(true),
				},
			},
			ScaleOut: AgentScaleOutPolicy{
				PreferredProviders: []string{"local-runtime"},
				Metadata:           map[string]string{"placement": "sticky"},
			},
		},
	}

	overlay := AgentSpecOverlayFromSpec(spec)
	require.NotNil(t, overlay.Coordination)

	overlay.Coordination.DelegationTargetSelectors[0].Name = "mutated"
	overlay.Coordination.DelegationTargetSelectors[0].CoordinationRoles[0] = CoordinationRolePlanner
	overlay.Coordination.ScaleOut.PreferredProviders[0] = "mutated-provider"
	overlay.Coordination.ScaleOut.Metadata["placement"] = "mutated"

	require.Equal(t, "reviewer", spec.Coordination.DelegationTargetSelectors[0].Name)
	require.Equal(t, CoordinationRoleReviewer, spec.Coordination.DelegationTargetSelectors[0].CoordinationRoles[0])
	require.Equal(t, "local-runtime", spec.Coordination.ScaleOut.PreferredProviders[0])
	require.Equal(t, "sticky", spec.Coordination.ScaleOut.Metadata["placement"])
}

func boolPtr(value bool) *bool {
	return &value
}
