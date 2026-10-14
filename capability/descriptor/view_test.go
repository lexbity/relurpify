package descriptor

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/governance/classification"
	governanceports "codeburg.org/lexbit/relurpify/governance/ports"
)

func TestDescriptorViewAdapterSatisfiesGovernanceView(t *testing.T) {
	var view governanceports.DescriptorView = CapabilityDescriptorView(CapabilityDescriptor{})
	require.NotNil(t, view)
}

func TestDescriptorViewAdapterExposesDescriptorFields(t *testing.T) {
	desc := CapabilityDescriptor{
		ID:            "cap.id",
		Kind:          agentspec.CapabilityKindTool,
		Name:          "Search",
		RuntimeFamily: agentspec.CapabilityRuntimeFamilyProvider,
		Description:   "searches things",
		Version:       "1.4.2",
		Category:      "search",
		Tags:          []string{"fast"},
		TrustClass:    agentspec.TrustClassRemoteDeclared,
		EffectClasses: []classification.EffectClass{classification.EffectClassNetworkEgress},
		Source:        CapabilitySource{ProviderID: "prov-9", Scope: classification.CapabilityScopeRemote, SessionID: "sess-3"},
	}

	view := CapabilityDescriptorView(desc)

	assert.Equal(t, "cap.id", view.CapabilityID())
	assert.Equal(t, "Search", view.CapabilityName())
	assert.Equal(t, "tool", view.CapabilityKind())
	assert.Equal(t, "provider", view.RuntimeFamily())
	assert.Equal(t, "searches things", view.Description())
	assert.Equal(t, "1.4.2", view.Version())
	assert.Equal(t, "search", view.Category())
	assert.Equal(t, []string{"fast"}, view.Tags())
	assert.Equal(t, "remote-declared-untrusted", view.TrustClass())
	assert.Nil(t, view.RiskClasses(), "risk classes are owned by governance, the view reports none")
	assert.Equal(t, []classification.EffectClass{classification.EffectClassNetworkEgress}, view.EffectClasses())
	assert.Equal(t, "prov-9", view.SourceProviderID())
	assert.Equal(t, "remote", view.SourceScope())
	assert.Equal(t, "sess-3", view.SourceSessionID())
}

func TestDescriptorViewAdapterWithoutCoordination(t *testing.T) {
	view := CapabilityDescriptorView(CapabilityDescriptor{ID: "x"})

	assert.Empty(t, view.CoordinationRole())
	assert.False(t, view.CoordinationTarget())
	assert.Nil(t, view.CoordinationTaskTypes())
	assert.Nil(t, view.CoordinationExecutionModes())
	assert.Equal(t, int32(0), view.CoordinationLongRunning())
	assert.Equal(t, int32(0), view.CoordinationDirectInsertionAllowed())
	assert.Equal(t, 0, view.CoordinationMaxDepth())
	assert.Equal(t, 0, view.CoordinationMaxRuntimeSeconds())
}

func TestDescriptorViewAdapterWithCoordination(t *testing.T) {
	view := CapabilityDescriptorView(CapabilityDescriptor{
		Coordination: &CoordinationTargetMetadata{
			Target:                 true,
			Role:                   agentspec.CoordinationRoleReviewer,
			TaskTypes:              []string{"review"},
			ExecutionModes:         []agentspec.CoordinationExecutionMode{agentspec.CoordinationExecutionModeSessionBacked},
			LongRunning:            EnabledStateEnabled,
			DirectInsertionAllowed: EnabledStateDisabled,
			MaxDepth:               4,
			MaxRuntimeSeconds:      120,
		},
	})

	assert.True(t, view.CoordinationTarget())
	assert.Equal(t, "reviewer", view.CoordinationRole())
	assert.Equal(t, []string{"review"}, view.CoordinationTaskTypes())
	assert.Equal(t, []string{"session-backed"}, view.CoordinationExecutionModes())
	assert.Equal(t, int32(1), view.CoordinationLongRunning())
	assert.Equal(t, int32(2), view.CoordinationDirectInsertionAllowed())
	assert.Equal(t, 4, view.CoordinationMaxDepth())
	assert.Equal(t, 120, view.CoordinationMaxRuntimeSeconds())
}

func TestDescriptorViewAdapterClampsOversizedStates(t *testing.T) {
	view := CapabilityDescriptorView(CapabilityDescriptor{
		Coordination: &CoordinationTargetMetadata{
			LongRunning:            EnabledState(1 << 40),
			DirectInsertionAllowed: EnabledState(-(1 << 40)),
		},
	})
	assert.Equal(t, int32(2147483647), view.CoordinationLongRunning())
	assert.Equal(t, int32(-2147483648), view.CoordinationDirectInsertionAllowed())
}
