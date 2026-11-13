package graphdb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestV2RejectedBatchNeverPersists verifies that a rejected CAS batch leaves no
// persistent record in either the durable log or memory mirror (D-07).
func TestV2RejectedBatchNeverPersists(t *testing.T) {
	t.Run("validation rejection leaves no durable record", func(t *testing.T) {
		ctx := context.Background()
		opts := DefaultOptions(t.TempDir())
		engine, err := Open(ctx, opts)
		require.NoError(t, err)

		conflictingID := "test_node_conflict"
		require.NoError(t, engine.ApplyBatch(ctx, GraphBatch{Nodes: []NodeRecord{{
			ID:    conflictingID,
			Kind:  NodeKind("test"),
			Props: json.RawMessage(`{"initial":true}`),
		}}}))

		validate := func(get NodeGetter) error {
			if _, exists := get(conflictingID); exists {
				return ErrBatchConflict
			}
			return nil
		}
		err = engine.ApplyBatchValidated(ctx, GraphBatch{Nodes: []NodeRecord{{
			ID:    conflictingID,
			Kind:  NodeKind("test"),
			Props: json.RawMessage(`{"conflict":true}`),
		}}}, validate)
		require.ErrorIs(t, err, ErrBatchConflict)

		node, exists := engine.GetNode(conflictingID)
		require.True(t, exists)
		assert.JSONEq(t, `{"initial":true}`, string(node.Props))

		require.NoError(t, engine.Close(ctx))

		reopened, err := Open(ctx, opts)
		require.NoError(t, err)
		defer func() { _ = reopened.Close(ctx) }()

		node, exists = reopened.GetNode(conflictingID)
		require.True(t, exists)
		assert.JSONEq(t, `{"initial":true}`, string(node.Props))
	})

	t.Run("validation before commit - no rejected durable batches", func(t *testing.T) {
		ctx := context.Background()
		opts := DefaultOptions(t.TempDir())
		engine, err := Open(ctx, opts)
		require.NoError(t, err)

		validationCalled := false
		validate := func(NodeGetter) error {
			validationCalled = true
			return ErrBatchConflict
		}
		err = engine.ApplyBatchValidated(ctx, GraphBatch{Nodes: []NodeRecord{{
			ID:    "test_reject",
			Kind:  NodeKind("test"),
			Props: json.RawMessage(`{"test":true}`),
		}}}, validate)
		require.ErrorIs(t, err, ErrBatchConflict)
		require.True(t, validationCalled, "validation should have been called")

		_, exists := engine.GetNode("test_reject")
		assert.False(t, exists, "rejected node should not exist in memory")

		require.NoError(t, engine.Close(ctx))

		reopened, err := Open(ctx, opts)
		require.NoError(t, err)
		defer func() { _ = reopened.Close(ctx) }()

		_, exists = reopened.GetNode("test_reject")
		assert.False(t, exists, "rejected node should not exist after reopen")
	})
}

// TestV2MutationOrderMatchesReopen verifies that committed mutations survive a
// close/reopen cycle with their payloads intact (D-07).
func TestV2MutationOrderMatchesReopen(t *testing.T) {
	ctx := context.Background()
	opts := DefaultOptions(t.TempDir())
	engine, err := Open(ctx, opts)
	require.NoError(t, err)

	nodeIDs := []string{"node1", "node2", "node3"}
	for i, id := range nodeIDs {
		require.NoError(t, engine.ApplyBatch(ctx, GraphBatch{Nodes: []NodeRecord{{
			ID:    id,
			Kind:  NodeKind("test"),
			Props: json.RawMessage(fmt.Sprintf(`{"order":%d}`, i+1)),
		}}}))
	}

	for i, id := range nodeIDs {
		node, exists := engine.GetNode(id)
		require.True(t, exists)
		assert.JSONEq(t, fmt.Sprintf(`{"order":%d}`, i+1), string(node.Props))
	}

	require.NoError(t, engine.Close(ctx))

	reopened, err := Open(ctx, opts)
	require.NoError(t, err)
	defer func() { _ = reopened.Close(ctx) }()

	for i, id := range nodeIDs {
		node, exists := reopened.GetNode(id)
		require.True(t, exists)
		assert.JSONEq(t, fmt.Sprintf(`{"order":%d}`, i+1), string(node.Props))
	}
}

// TestV2DirtyAuthorityReadFails verifies that a post-commit memory apply
// failure marks the engine dirty and blocks authoritative work until rebuild.
func TestV2DirtyAuthorityReadFails(t *testing.T) {
	ctx := context.Background()
	opts := DefaultOptions(t.TempDir())
	engine, err := Open(ctx, opts)
	require.NoError(t, err)
	defer func() { _ = engine.Close(ctx) }()

	engine.testHookApplyFailure = errors.New("hook: apply failed")
	err = engine.ApplyBatch(ctx, GraphBatch{Nodes: []NodeRecord{{
		ID:   "test_node",
		Kind: NodeKind("test"),
	}}})
	require.Error(t, err)
	require.Error(t, engine.checkDirty(), "dirty authority gate must reject reads")

	engine.testHookApplyFailure = nil
	require.NoError(t, engine.Rebuild(ctx))
	require.NoError(t, engine.checkDirty())

	_, exists := engine.GetNode("test_node")
	require.True(t, exists, "committed node must be visible after rebuild")
}

// TestV2LRUValidationFindsUncachedRecord verifies that LRU cache misses fall
// back to the durable backend during validated batch decisions (D-07).
func TestV2LRUValidationFindsUncachedRecord(t *testing.T) {
	ctx := context.Background()
	opts := DefaultOptions(t.TempDir())
	opts.LRUCapacity = 1

	engine, err := Open(ctx, opts)
	require.NoError(t, err)
	for i := 0; i < 5; i++ {
		require.NoError(t, engine.ApplyBatch(ctx, GraphBatch{Nodes: []NodeRecord{{
			ID:    fmt.Sprintf("node_%d", i),
			Kind:  NodeKind("test"),
			Props: json.RawMessage(fmt.Sprintf(`{"index":%d}`, i)),
		}}}))
	}
	require.NoError(t, engine.Close(ctx))

	reopened, err := Open(ctx, opts)
	require.NoError(t, err)
	defer func() { _ = reopened.Close(ctx) }()

	// Load node_0, then node_1: capacity 1 evicts node_0 from memory.
	_, exists := reopened.GetNode("node_0")
	require.True(t, exists)
	_, exists = reopened.GetNode("node_1")
	require.True(t, exists)

	validator := func(get NodeGetter) error {
		if _, exists := get("node_0"); !exists {
			return errors.New("node_0 not found")
		}
		return ErrBatchConflict
	}
	err = reopened.ApplyBatchValidated(ctx, GraphBatch{Nodes: []NodeRecord{{
		ID:    "node_0",
		Kind:  NodeKind("test"),
		Props: json.RawMessage(`{"conflict":true}`),
	}}}, validator)
	require.ErrorIs(t, err, ErrBatchConflict, "validator must find the uncached record in the backend")
}
