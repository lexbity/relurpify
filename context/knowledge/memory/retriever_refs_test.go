package memory

import (
	"testing"
	"time"

	relurpctx "codeburg.org/lexbit/relurpify/context"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	"github.com/stretchr/testify/require"
)

func TestToRetrievalReferenceMapsFields(t *testing.T) {
	before := time.Now().UTC().Add(-time.Second)
	entry := MemoryEntry{Value: "v", Class: relurpctx.MemoryClassWorking, CreatedAt: before, UpdatedAt: before}
	env := MemoryRecordEnvelope{TaskID: "task-a", Key: "notes/1", Entry: entry}

	ref := env.ToRetrievalReference("query-42")

	require.Equal(t, "query-42", ref.QueryID)
	require.Equal(t, "memory:task-a/notes/1", ref.QueryText)
	require.Equal(t, "working_memory", ref.Scope)
	require.Equal(t, 1, ref.TotalFound)
	require.Zero(t, ref.FilteredOut)
	require.Zero(t, ref.Duration)
	require.Equal(t, []string{"notes/1"}, chunkIDStrings(ref.ChunkIDs))
	require.False(t, ref.RetrievedAt.IsZero(), "RetrievedAt must be stamped at conversion time")
	require.True(t, ref.RetrievedAt.After(before), "RetrievedAt must be stamped at conversion time")
}

func TestAsRetrievalReferencesConvertsBatch(t *testing.T) {
	results := []MemoryRecordEnvelope{
		{TaskID: "task-a", Key: "k1"},
		{TaskID: "task-a", Key: "k2"},
	}

	refs := AsRetrievalReferences(results, "query-7")
	require.Len(t, refs, 2)
	for i, ref := range refs {
		require.Equal(t, "query-7", ref.QueryID)
		require.Equal(t, "memory:task-a/k"+string(rune('1'+i)), ref.QueryText)
		require.Equal(t, "working_memory", ref.Scope)
		require.Equal(t, []string{"k" + string(rune('1'+i))}, chunkIDStrings(ref.ChunkIDs))
		require.Equal(t, 1, ref.TotalFound)
	}
	// Batch conversion stamps a single timestamp across all references.
	require.Equal(t, refs[0].RetrievedAt, refs[1].RetrievedAt)
}

func TestAsRetrievalReferencesReturnsNilForEmptyBatch(t *testing.T) {
	require.Nil(t, AsRetrievalReferences(nil, "query-7"))
	require.Nil(t, AsRetrievalReferences([]MemoryRecordEnvelope{}, "query-7"))
}

// chunkIDStrings converts contextdata.ChunkID values for assertions.
func chunkIDStrings(ids []contextdata.ChunkID) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		out = append(out, string(id))
	}
	return out
}
