package memory

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHybridMemorySearchUsesVectorStoreRanking(t *testing.T) {
	t.Parallel()

	store, err := NewHybridMemory(t.TempDir())
	require.NoError(t, err)
	store.WithVectorStore(NewInMemoryVectorStore())

	ctx := context.Background()
	require.NoError(t, store.Remember(ctx, "incident", map[string]interface{}{
		"summary": "database transaction rollback failure during payment processing",
	}, MemoryScopeProject))
	require.NoError(t, store.Remember(ctx, "noise", map[string]interface{}{
		"summary": "frontend button color discussion",
	}, MemoryScopeProject))

	results, err := store.Search(ctx, "database rollback payment", MemoryScopeProject)
	require.NoError(t, err)
	require.NotEmpty(t, results)
	require.Equal(t, "incident", results[0].Key)
}

func TestHybridMemorySearchFallsBackToSubstringWithoutVectorStore(t *testing.T) {
	t.Parallel()

	store, err := NewHybridMemory(t.TempDir())
	require.NoError(t, err)

	ctx := context.Background()
	require.NoError(t, store.Remember(ctx, "release", map[string]interface{}{
		"summary": "release checklist for qa signoff",
	}, MemoryScopeProject))

	results, err := store.Search(ctx, "qa signoff", MemoryScopeProject)
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, "release", results[0].Key)
}
