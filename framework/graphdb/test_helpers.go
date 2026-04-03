package graphdb

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// newTestEngine creates an engine with test‑friendly options.
func newTestEngine(t *testing.T) (*Engine, Options) {
	t.Helper()
	opts := DefaultOptions(t.TempDir())
	opts.AutoSaveInterval = 10 * time.Millisecond
	opts.AutoSaveThreshold = 100
	opts.AOFRewriteThresholdBytes = 1 << 20
	engine, err := Open(opts)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, engine.Close())
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
