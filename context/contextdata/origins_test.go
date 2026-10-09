package contextdata

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOriginClassValid(t *testing.T) {
	for _, class := range []OriginClass{OriginUser, OriginTool, OriginLLM} {
		require.True(t, class.Valid(), "%q must be valid", class)
	}
	require.False(t, OriginClass("derivation").Valid())
	require.False(t, OriginClass("").Valid())
}

func TestMostRestrictive(t *testing.T) {
	require.Equal(t, OriginLLM, MostRestrictive(OriginLLM, OriginUser))
	require.Equal(t, OriginTool, MostRestrictive(OriginUser, OriginTool))
	require.Equal(t, OriginTool, MostRestrictive(OriginTool, OriginTool))
	require.Equal(t, OriginUser, MostRestrictive(OriginUser, OriginUser))
	// Unknown classes are maximally restrictive so trust can never be elevated.
	require.Equal(t, OriginClass("mystery"), MostRestrictive(OriginClass("mystery"), OriginLLM))
}

func TestEnvelopeOriginRecording(t *testing.T) {
	env := NewEnvelope("t1", "s1")
	env.SetWorkingValueWithOrigin("input.prompt", "text", MemoryClassTask, OriginUser)
	env.SetWorkingValueWithClass("state.derived", "d", MemoryClassTask)

	require.Equal(t, OriginUser, env.OriginOf("input.prompt"))
	require.Equal(t, OriginLLM, env.OriginOf("state.derived"), "class writes default to llm origin")
	require.Equal(t, OriginLLM, env.OriginOf("absent"), "missing keys default to llm")

	origins := env.OriginsSnapshot()
	require.Len(t, origins, 2)
	if origins["input.prompt"] != OriginUser {
		t.Fatalf("origins snapshot missing user origin: %+v", origins)
	}
}

func TestEnvelopeCloneCopiesOrigins(t *testing.T) {
	env := NewEnvelope("t1", "s1")
	env.SetWorkingValueWithOrigin("user.prompt", "text", MemoryClassTask, OriginUser)
	env.SetWorkingValueWithClass("state.x", 1, MemoryClassTask)

	clone := env.Clone()
	require.Equal(t, OriginUser, clone.OriginOf("user.prompt"))
	require.Equal(t, OriginLLM, clone.OriginOf("state.x"))

	// Mutating the clone must not leak into the original.
	clone.SetWorkingValueWithOrigin("user.prompt", "changed", MemoryClassTask, OriginLLM)
	require.Equal(t, OriginUser, env.OriginOf("user.prompt"))
}

func TestEnvelopeHandoffSnapshotCopiesPreservedOrigins(t *testing.T) {
	env := NewEnvelope("t1", "s1")
	env.SetWorkingValueWithOrigin("user.prompt", "text", MemoryClassTask, OriginUser)
	env.SetWorkingValueWithOrigin("scratch.tmp", "noise", MemoryClassTask, OriginTool)

	snapshot := env.HandoffSnapshot(HandoffPolicy{PreserveWorkingMemory: true, WorkingKeys: []string{"user.prompt"}})
	require.Equal(t, OriginUser, snapshot.OriginOf("user.prompt"))
	require.Equal(t, OriginLLM, snapshot.OriginOf("scratch.tmp"), "unpreserved key must not carry origin")
}
