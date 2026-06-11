package graphdb

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// newTestEngine creates a Badger‑backed engine in a temp directory.
func newTestEngine(t *testing.T) (*Engine, Options) {
	t.Helper()
	opts := DefaultOptions(t.TempDir())
	engine, err := Open(context.Background(), opts)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, engine.Close(context.Background()))
	})
	return engine, opts
}

// allOutEdges returns all edges (including soft‑deleted) from the store.
func allOutEdges(t *testing.T, engine *Engine, nodeID string) []EdgeRecord {
	t.Helper()
	engine.store.mu.RLock()
	defer engine.store.mu.RUnlock()
	return cloneEdges(engine.store.forward[nodeID])
}

// allInEdges returns all incoming edges (including soft‑deleted) from the store.
func allInEdges(t *testing.T, engine *Engine, nodeID string) []EdgeRecord {
	t.Helper()
	engine.store.mu.RLock()
	defer engine.store.mu.RUnlock()
	return cloneEdges(engine.store.reverse[nodeID])
}
