package memory

import (
	"testing"

	relurpctx "codeburg.org/lexbit/relurpify/context"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	"github.com/stretchr/testify/require"
)

func sampleResults() []MemoryRecordEnvelope {
	return []MemoryRecordEnvelope{
		{TaskID: "task-a", Key: "alpha", Entry: MemoryEntry{Value: "va", Class: relurpctx.MemoryClassWorking}},
		{TaskID: "task-a", Key: "beta", Entry: MemoryEntry{Value: 42, Class: relurpctx.MemoryClassStreamed}},
	}
}

// envelopeClassOf reads back the memory class recorded for a working key.
func envelopeClassOf(t *testing.T, env *contextdata.Envelope, key string) contextdata.MemoryClass {
	t.Helper()
	for _, ref := range env.ReferencesSnapshot().WorkingMemory {
		if ref.Key == key {
			return ref.Class
		}
	}
	t.Fatalf("no working memory reference for key %q", key)
	return ""
}

func TestHydrateWritesResultsIntoState(t *testing.T) {
	s := NewWorkingMemoryStore()
	state := map[string]any{"existing": true}

	require.NoError(t, s.Hydrate(t.Context(), state, sampleResults()))
	require.Len(t, state, 3)
	require.Equal(t, "va", state["alpha"])
	require.Equal(t, 42, state["beta"])
	require.Equal(t, true, state["existing"], "hydration must not clobber unrelated keys")

	// Hydrating the same results again overwrites by key, not duplicates.
	require.NoError(t, s.Hydrate(t.Context(), state, sampleResults()))
	require.Len(t, state, 3)

	// An empty result set leaves the state untouched.
	require.NoError(t, s.Hydrate(t.Context(), state, nil))
	require.Len(t, state, 3)
}

func TestHydrateRejectsNilState(t *testing.T) {
	s := NewWorkingMemoryStore()
	err := s.Hydrate(t.Context(), nil, sampleResults())
	require.EqualError(t, err, "state is nil")
}

func TestHydrateIntoEnvelopeWritesWorkingMemory(t *testing.T) {
	s := NewWorkingMemoryStore()
	env := contextdata.NewEnvelope("task-a", "session-1")

	require.NoError(t, s.HydrateIntoEnvelope(t.Context(), env, sampleResults()))

	snap := env.WorkingDataSnapshot()
	require.Equal(t, "va", snap["alpha"])
	require.Equal(t, 42, snap["beta"])
	require.Equal(t, contextdata.MemoryClass("working"), envelopeClassOf(t, env, "alpha"))
	require.Equal(t, contextdata.MemoryClass("streamed"), envelopeClassOf(t, env, "beta"))
}

func TestHydrateIntoEnvelopeRejectsNilEnvelope(t *testing.T) {
	s := NewWorkingMemoryStore()
	err := s.HydrateIntoEnvelope(t.Context(), nil, sampleResults())
	require.EqualError(t, err, "envelope is nil")
}

func TestHydrateWithMappingUsesIdentityWhenMapperMissing(t *testing.T) {
	s := NewWorkingMemoryStore()
	state := map[string]any{}

	require.NoError(t, s.HydrateWithMapping(t.Context(), state, sampleResults(), nil))
	require.Equal(t, "va", state["alpha"])
	require.Equal(t, 42, state["beta"])
}

func TestHydrateWithMappingAppliesMapper(t *testing.T) {
	s := NewWorkingMemoryStore()
	state := map[string]any{}
	mapper := func(k string) string { return "memory." + k }

	require.NoError(t, s.HydrateWithMapping(t.Context(), state, sampleResults(), mapper))
	require.Len(t, state, 2)
	require.Equal(t, "va", state["memory.alpha"])
	require.Equal(t, 42, state["memory.beta"])
	_, ok := state["alpha"]
	require.False(t, ok, "mapped keys must replace the original key")
}

func TestHydrateWithMappingRejectsNilState(t *testing.T) {
	s := NewWorkingMemoryStore()
	err := s.HydrateWithMapping(t.Context(), nil, sampleResults(), nil)
	require.EqualError(t, err, "state is nil")
}

func TestHydrateIntoEnvelopeWithMapping(t *testing.T) {
	s := NewWorkingMemoryStore()

	t.Run("custom mapper", func(t *testing.T) {
		env := contextdata.NewEnvelope("task-a", "session-1")
		mapper := func(k string) string { return "mem." + k }
		require.NoError(t, s.HydrateIntoEnvelopeWithMapping(t.Context(), env, sampleResults(), mapper))
		snap := env.WorkingDataSnapshot()
		require.Len(t, snap, 2)
		require.Equal(t, "va", snap["mem.alpha"])
		require.Equal(t, contextdata.MemoryClass("streamed"), envelopeClassOf(t, env, "mem.beta"))
	})

	t.Run("nil mapper falls back to identity", func(t *testing.T) {
		env := contextdata.NewEnvelope("task-a", "session-1")
		require.NoError(t, s.HydrateIntoEnvelopeWithMapping(t.Context(), env, sampleResults(), nil))
		require.Equal(t, "va", env.WorkingDataSnapshot()["alpha"])
	})

	t.Run("nil envelope is rejected", func(t *testing.T) {
		err := s.HydrateIntoEnvelopeWithMapping(t.Context(), nil, sampleResults(), nil)
		require.EqualError(t, err, "envelope is nil")
	})
}

func TestSimpleStateHydratorPrefixesKeys(t *testing.T) {
	h := NewSimpleStateHydrator("wm.")
	require.Equal(t, "wm.", h.KeyPrefix)

	state := map[string]any{}
	require.NoError(t, h.Hydrate(t.Context(), state, sampleResults()))
	require.Equal(t, "va", state["wm.alpha"])
	require.Equal(t, 42, state["wm.beta"])
	require.NotContains(t, state, "alpha")
}

func TestSimpleStateHydratorRejectsNilState(t *testing.T) {
	h := NewSimpleStateHydrator("")
	err := h.Hydrate(t.Context(), nil, sampleResults())
	require.EqualError(t, err, "state is nil")
}

func TestSimpleStateHydratorHydratesEnvelopeWithPrefix(t *testing.T) {
	h := NewSimpleStateHydrator("wm.")
	env := contextdata.NewEnvelope("task-a", "session-1")

	require.NoError(t, h.HydrateIntoEnvelope(t.Context(), env, sampleResults()))
	snap := env.WorkingDataSnapshot()
	require.Equal(t, "va", snap["wm.alpha"])
	require.Equal(t, 42, snap["wm.beta"])
	require.Equal(t, contextdata.MemoryClass("working"), envelopeClassOf(t, env, "wm.alpha"))
}

func TestSimpleStateHydratorRejectsNilEnvelope(t *testing.T) {
	h := NewSimpleStateHydrator("")
	err := h.HydrateIntoEnvelope(t.Context(), nil, sampleResults())
	require.EqualError(t, err, "envelope is nil")
}

// TestWorkingMemoryStoreSatisfiesInterfaces pins the contracts that agentgraph
// nodes depend on.
func TestWorkingMemoryStoreSatisfiesInterfaces(t *testing.T) {
	s := NewWorkingMemoryStore()
	var _ MemoryRetriever = s
	var _ StateHydrator = s
	var _ EnvelopeHydrator = s
	h := NewSimpleStateHydrator("")
	var _ StateHydrator = h
	var _ EnvelopeHydrator = h
}
