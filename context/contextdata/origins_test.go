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

func TestApplyBranchMergesPropagatesOrigins(t *testing.T) {
	parent := NewEnvelope(testTaskID, testSessionID)
	parent.SetWorkingValueWithOrigin("base", "base-value", MemoryClassTask, OriginUser)

	branchA := parent.Clone()
	branchA.SetWorkingValueWithOrigin("a.out", "a-value", MemoryClassTask, OriginTool)

	branchB := parent.Clone()
	branchB.SetWorkingValueWithOrigin("b.out", "b-value", MemoryClassTask, OriginLLM)
	branchB.DeleteWorkingValue("base")

	stats, err := parent.ApplyBranchMerges([]BranchMergeUnit{
		{Index: 0, ID: "a", Delta: ComputeBranchDelta(parent, branchA), Env: branchA},
		{Index: 1, ID: "b", Delta: ComputeBranchDelta(parent, branchB), Env: branchB},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	require.Equal(t, 2, stats.KeysWritten)
	require.Equal(t, 1, stats.KeysDeleted)

	if got := parent.OriginOf("a.out"); got != OriginTool {
		t.Fatalf("written key must carry its branch origin: got %q", got)
	}
	if got := parent.OriginOf("b.out"); got != OriginLLM {
		t.Fatalf("written key must carry its branch origin: got %q", got)
	}
	if got := parent.OriginOf("base"); got != OriginLLM {
		t.Fatalf("deleted key's origin must not survive: got %q", got)
	}
	if _, ok := parent.OriginsSnapshot()["base"]; ok {
		t.Fatal("deleted key must have no origin entry at all")
	}
}

func TestApplyBranchMergesConflictOriginMostRestrictive(t *testing.T) {
	parent := NewEnvelope(testTaskID, testSessionID)

	branchA := parent.Clone()
	branchA.SetWorkingValueWithOrigin("shared", "from-user", MemoryClassTask, OriginUser)

	branchB := parent.Clone()
	branchB.SetWorkingValueWithOrigin("shared", "from-llm", MemoryClassTask, OriginLLM)

	if _, err := parent.ApplyBranchMerges([]BranchMergeUnit{
		{Index: 0, ID: "a", Delta: ComputeBranchDelta(parent, branchA), Env: branchA},
		{Index: 1, ID: "b", Delta: ComputeBranchDelta(parent, branchB), Env: branchB},
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The value resolves by declaration order (branch B), but the origin is the
	// more restrictive of the two writers — trust only descends.
	require.Equal(t, "from-llm", parent.WorkingDataSnapshot()["shared"])
	require.Equal(t, OriginLLM, parent.OriginOf("shared"))
}

func TestEnvelopeMergeCarriesOrigins(t *testing.T) {
	dst := NewEnvelope(testTaskID, testSessionID)
	dst.SetWorkingValueWithOrigin("dst.existing", "v", MemoryClassTask, OriginTool)

	src := NewEnvelope(testTaskID, testSessionID)
	src.SetWorkingValueWithOrigin("user.prompt", "text", MemoryClassTask, OriginUser)
	src.SetWorkingValueWithClass("state.x", 1, MemoryClassTask)

	dst.Merge(src)
	require.Equal(t, OriginUser, dst.OriginOf("user.prompt"))
	require.Equal(t, OriginLLM, dst.OriginOf("state.x"))
	require.Equal(t, OriginTool, dst.OriginOf("dst.existing"), "keys absent from the source keep their origin")
}

func TestDeleteWorkingValueClearsOrigin(t *testing.T) {
	env := NewEnvelope(testTaskID, testSessionID)
	env.SetWorkingValueWithOrigin("user.prompt", "text", MemoryClassTask, OriginUser)

	env.DeleteWorkingValue("user.prompt")
	if _, ok := env.OriginsSnapshot()["user.prompt"]; ok {
		t.Fatal("deleted key must have no origin entry")
	}
	require.Equal(t, OriginLLM, env.OriginOf("user.prompt"), "effective origin falls back to the default")
}
