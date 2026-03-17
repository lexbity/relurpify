package euclo

import (
	"context"
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/stretchr/testify/require"
)

// stubCodingCapability is a test helper implementing EucloCodingCapability
// with configurable behavior.
type stubCodingCapability struct {
	id               string
	contract         ArtifactContract
	eligible         bool
	eligibleReason   string
	executeResult    ExecutionResult
	annotations      map[string]any
	executeCalled    bool
	eligibleArtState ArtifactState
}

func (s *stubCodingCapability) Descriptor() core.CapabilityDescriptor {
	return core.CapabilityDescriptor{
		ID:          s.id,
		Name:        s.id,
		Annotations: s.annotations,
	}
}

func (s *stubCodingCapability) Contract() ArtifactContract {
	return s.contract
}

func (s *stubCodingCapability) Eligible(artifacts ArtifactState, _ CapabilitySnapshot) EligibilityResult {
	s.eligibleArtState = artifacts
	return EligibilityResult{Eligible: s.eligible, Reason: s.eligibleReason}
}

func (s *stubCodingCapability) Execute(_ context.Context, _ ExecutionEnvelope) ExecutionResult {
	s.executeCalled = true
	return s.executeResult
}

func TestRegistryRegisterAndLookup(t *testing.T) {
	reg := NewEucloCapabilityRegistry()
	cap := &stubCodingCapability{id: "euclo:test.cap", eligible: true}

	err := reg.Register(cap)
	require.NoError(t, err)

	found, ok := reg.Lookup("euclo:test.cap")
	require.True(t, ok)
	require.Equal(t, "euclo:test.cap", found.Descriptor().ID)

	_, ok = reg.Lookup("euclo:nonexistent")
	require.False(t, ok)
}

func TestRegistryRegisterRejectsNil(t *testing.T) {
	reg := NewEucloCapabilityRegistry()
	err := reg.Register(nil)
	require.Error(t, err)
}

func TestRegistryRegisterRejectsEmptyID(t *testing.T) {
	reg := NewEucloCapabilityRegistry()
	cap := &stubCodingCapability{id: ""}
	err := reg.Register(cap)
	require.Error(t, err)
}

func TestRegistryRegisterOverwritesExisting(t *testing.T) {
	reg := NewEucloCapabilityRegistry()
	cap1 := &stubCodingCapability{id: "euclo:dup", eligibleReason: "v1"}
	cap2 := &stubCodingCapability{id: "euclo:dup", eligibleReason: "v2"}

	require.NoError(t, reg.Register(cap1))
	require.NoError(t, reg.Register(cap2))

	found, ok := reg.Lookup("euclo:dup")
	require.True(t, ok)
	result := found.Eligible(NewArtifactState(nil), CapabilitySnapshot{})
	require.Equal(t, "v2", result.Reason)
}

func TestRegistryList(t *testing.T) {
	reg := NewEucloCapabilityRegistry()
	require.NoError(t, reg.Register(&stubCodingCapability{id: "euclo:b"}))
	require.NoError(t, reg.Register(&stubCodingCapability{id: "euclo:a"}))
	require.NoError(t, reg.Register(&stubCodingCapability{id: "euclo:c"}))

	caps := reg.List()
	require.Len(t, caps, 3)
	require.Equal(t, "euclo:a", caps[0].Descriptor().ID)
	require.Equal(t, "euclo:b", caps[1].Descriptor().ID)
	require.Equal(t, "euclo:c", caps[2].Descriptor().ID)
}

func TestRegistryEligibleForFiltersIneligible(t *testing.T) {
	reg := NewEucloCapabilityRegistry()
	require.NoError(t, reg.Register(&stubCodingCapability{id: "euclo:yes", eligible: true}))
	require.NoError(t, reg.Register(&stubCodingCapability{id: "euclo:no", eligible: false}))
	require.NoError(t, reg.Register(&stubCodingCapability{id: "euclo:also_yes", eligible: true}))

	eligible := reg.EligibleFor(NewArtifactState(nil), CapabilitySnapshot{})
	require.Len(t, eligible, 2)
	require.Equal(t, "euclo:also_yes", eligible[0].Descriptor().ID)
	require.Equal(t, "euclo:yes", eligible[1].Descriptor().ID)
}

func TestRegistryEligibleForPassesArtifactState(t *testing.T) {
	stub := &stubCodingCapability{id: "euclo:check", eligible: true}
	reg := NewEucloCapabilityRegistry()
	require.NoError(t, reg.Register(stub))

	arts := NewArtifactState([]Artifact{{Kind: ArtifactKindPlan}})
	reg.EligibleFor(arts, CapabilitySnapshot{})
	require.True(t, stub.eligibleArtState.Has(ArtifactKindPlan))
}

func TestRegistryForProfileFiltersStringSlice(t *testing.T) {
	reg := NewEucloCapabilityRegistry()
	require.NoError(t, reg.Register(&stubCodingCapability{
		id:          "euclo:evr",
		annotations: map[string]any{"supported_profiles": []string{"edit_verify_repair", "reproduce_localize_patch"}},
	}))
	require.NoError(t, reg.Register(&stubCodingCapability{
		id:          "euclo:rlp",
		annotations: map[string]any{"supported_profiles": []string{"reproduce_localize_patch"}},
	}))
	require.NoError(t, reg.Register(&stubCodingCapability{
		id:          "euclo:other",
		annotations: map[string]any{"supported_profiles": []string{"plan_stage_execute"}},
	}))

	evr := reg.ForProfile("edit_verify_repair")
	require.Len(t, evr, 1)
	require.Equal(t, "euclo:evr", evr[0].Descriptor().ID)

	rlp := reg.ForProfile("reproduce_localize_patch")
	require.Len(t, rlp, 2)
}

func TestRegistryForProfileFiltersCaseInsensitive(t *testing.T) {
	reg := NewEucloCapabilityRegistry()
	require.NoError(t, reg.Register(&stubCodingCapability{
		id:          "euclo:cap",
		annotations: map[string]any{"supported_profiles": []string{"Edit_Verify_Repair"}},
	}))

	caps := reg.ForProfile("edit_verify_repair")
	require.Len(t, caps, 1)
}

func TestRegistryForProfileFiltersCommaString(t *testing.T) {
	reg := NewEucloCapabilityRegistry()
	require.NoError(t, reg.Register(&stubCodingCapability{
		id:          "euclo:cap",
		annotations: map[string]any{"supported_profiles": "edit_verify_repair,reproduce_localize_patch"},
	}))

	caps := reg.ForProfile("reproduce_localize_patch")
	require.Len(t, caps, 1)
}

func TestRegistryForProfileFiltersAnySlice(t *testing.T) {
	reg := NewEucloCapabilityRegistry()
	require.NoError(t, reg.Register(&stubCodingCapability{
		id:          "euclo:cap",
		annotations: map[string]any{"supported_profiles": []any{"edit_verify_repair"}},
	}))

	caps := reg.ForProfile("edit_verify_repair")
	require.Len(t, caps, 1)
}

func TestRegistryForProfileReturnsNilForEmptyProfileID(t *testing.T) {
	reg := NewEucloCapabilityRegistry()
	require.NoError(t, reg.Register(&stubCodingCapability{
		id:          "euclo:cap",
		annotations: map[string]any{"supported_profiles": []string{"edit_verify_repair"}},
	}))

	caps := reg.ForProfile("")
	require.Nil(t, caps)
}

func TestRegistryForProfileSkipsNoAnnotations(t *testing.T) {
	reg := NewEucloCapabilityRegistry()
	require.NoError(t, reg.Register(&stubCodingCapability{id: "euclo:bare"}))

	caps := reg.ForProfile("edit_verify_repair")
	require.Empty(t, caps)
}

func TestRegistryLookupTrimsWhitespace(t *testing.T) {
	reg := NewEucloCapabilityRegistry()
	require.NoError(t, reg.Register(&stubCodingCapability{id: "euclo:trimmed"}))

	found, ok := reg.Lookup("  euclo:trimmed  ")
	require.True(t, ok)
	require.Equal(t, "euclo:trimmed", found.Descriptor().ID)
}
