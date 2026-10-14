package descriptor

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/governance/classification"
)

func TestDescriptorAccessorMethodsWithoutCoordination(t *testing.T) {
	d := CapabilityDescriptor{
		ID:            "cap.id",
		Name:          "Capability",
		TrustClass:    agentspec.TrustClassWorkspaceTrusted,
		RuntimeFamily: agentspec.CapabilityRuntimeFamilyRelurpic,
		Source:        CapabilitySource{ProviderID: "prov", Scope: classification.CapabilityScopeWorkspace, SessionID: "sess-1"},
	}

	assert.Equal(t, "cap.id", d.CapabilityID())
	assert.Equal(t, "Capability", d.CapabilityName())
	assert.Equal(t, "workspace-trusted", d.CapabilityTrustClass())
	assert.Equal(t, "relurpic", d.CapabilityRuntimeFamily())
	assert.Equal(t, classification.CapabilityScopeWorkspace, d.SourceScope())
	assert.Equal(t, "prov", d.SourceProviderID())
	assert.Equal(t, "sess-1", d.SourceSessionID())

	// Without coordination metadata every coordination accessor reports zero.
	assert.Empty(t, d.CoordinationRole())
	assert.False(t, d.CoordinationTarget())
	assert.Equal(t, int32(0), d.LongRunning())
	assert.Equal(t, int32(0), d.DirectInsertionAllowed())
	assert.Nil(t, d.CoordinationTaskTypes())
	assert.Nil(t, d.CoordinationExecutionModes())
}

func TestDescriptorAccessorMethodsWithCoordination(t *testing.T) {
	d := CapabilityDescriptor{
		Coordination: &CoordinationTargetMetadata{
			Target:                 true,
			Role:                   agentspec.CoordinationRolePlanner,
			TaskTypes:              []string{"plan", "design"},
			ExecutionModes:         []agentspec.CoordinationExecutionMode{agentspec.CoordinationExecutionModeSync},
			LongRunning:            EnabledStateEnabled,
			DirectInsertionAllowed: EnabledStateDisabled,
		},
	}

	assert.True(t, d.CoordinationTarget())
	assert.Equal(t, "planner", d.CoordinationRole())
	assert.Equal(t, int32(1), d.LongRunning())
	assert.Equal(t, int32(2), d.DirectInsertionAllowed())
	assert.Equal(t, []string{"plan", "design"}, d.CoordinationTaskTypes())
	assert.Equal(t, []string{"sync"}, d.CoordinationExecutionModes(),
		"execution modes are surfaced as plain strings")
}

func TestDescriptorAccessorMethodsClampOversizedStates(t *testing.T) {
	// EnabledState is int-backed, so out-of-int32-range values are
	// constructible; the accessors must clamp rather than wrap.
	huge := EnabledState(1 << 40)
	tiny := EnabledState(-(1 << 40))

	high := CapabilityDescriptor{Coordination: &CoordinationTargetMetadata{
		Target:                 true,
		LongRunning:            huge,
		DirectInsertionAllowed: huge,
	}}
	assert.Equal(t, int32(2147483647), high.LongRunning())
	assert.Equal(t, int32(2147483647), high.DirectInsertionAllowed())

	low := CapabilityDescriptor{Coordination: &CoordinationTargetMetadata{
		LongRunning:            tiny,
		DirectInsertionAllowed: tiny,
	}}
	assert.Equal(t, int32(-2147483648), low.LongRunning())
	assert.Equal(t, int32(-2147483648), low.DirectInsertionAllowed())
}
