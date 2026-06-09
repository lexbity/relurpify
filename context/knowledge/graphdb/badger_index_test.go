package graphdb

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/dgraph-io/badger/v4"
	"github.com/stretchr/testify/require"
)

// hasKey reports whether badger has an entry for the given key.
func hasKey(t *testing.T, db *badger.DB, key []byte) bool {
	t.Helper()
	var found bool
	require.NoError(t, db.View(func(txn *badger.Txn) error {
		_, err := txn.Get(key)
		if errors.Is(err, badger.ErrKeyNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		found = true
		return nil
	}))
	return found
}

// countPrefix returns the number of keys matching the given prefix.
func countPrefix(t *testing.T, db *badger.DB, prefix []byte) int {
	t.Helper()
	var n int
	require.NoError(t, db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			n++
		}
		return nil
	}))
	return n
}

func requireHasKey(t *testing.T, db *badger.DB, key []byte) {
	t.Helper()
	require.True(t, hasKey(t, db, key), "expected key to exist")
}

func requireNotHasKey(t *testing.T, db *badger.DB, key []byte) {
	t.Helper()
	require.False(t, hasKey(t, db, key), "expected key to NOT exist")
}

// ────────────────────────────────────────────────────────────────────
// Index maintenance tests
// ────────────────────────────────────────────────────────────────────

func TestIndex_NodeKind(t *testing.T) {
	bb, err := newBadgerBackend(BadgerOptions{InMemory: true})
	require.NoError(t, err)
	defer bb.close()

	require.NoError(t, bb.commit(context.TODO(), mutationBatch{
		opName: "upsert_node",
		op:     nodeOp{Node: NodeRecord{ID: "n1", Kind: "function", SourceID: "s.go"}},
	}))

	requireHasKey(t, bb.db, keyNodeByKind("function", "n1"))
	requireHasKey(t, bb.db, keyNodeBySource("s.go", "n1"))
}

func TestIndex_NodeLabel(t *testing.T) {
	bb, err := newBadgerBackend(BadgerOptions{InMemory: true})
	require.NoError(t, err)
	defer bb.close()

	require.NoError(t, bb.commit(context.TODO(), mutationBatch{
		opName: "upsert_node",
		op:     nodeOp{Node: NodeRecord{ID: "n1", Kind: "function", Labels: []string{"tag:a", "tag:b"}}},
	}))

	requireHasKey(t, bb.db, keyNodeByLabel("tag:a", "n1"))
	requireHasKey(t, bb.db, keyNodeByLabel("tag:b", "n1"))
}

func TestIndex_NodeStableID(t *testing.T) {
	bb, err := newBadgerBackend(BadgerOptions{InMemory: true})
	require.NoError(t, err)
	defer bb.close()

	require.NoError(t, bb.commit(context.TODO(), mutationBatch{
		opName: "upsert_node",
		op:     nodeOp{Node: NodeRecord{ID: "n1", Kind: "function", StableID: "stable-1"}},
	}))

	requireHasKey(t, bb.db, keyNodeByStable("stable-1", "n1"))
}

func TestIndex_NodePathHashMedia(t *testing.T) {
	bb, err := newBadgerBackend(BadgerOptions{InMemory: true})
	require.NoError(t, err)
	defer bb.close()

	require.NoError(t, bb.commit(context.TODO(), mutationBatch{
		opName: "upsert_node",
		op: nodeOp{Node: NodeRecord{
			ID:    "n1",
			Kind:  "file",
			Props: json.RawMessage(`{"path":"src/main.go","content_hash":"sha256:abc","media_type":"text/x-go"}`),
		}},
	}))

	requireHasKey(t, bb.db, keyNodeByPath("src/main.go", "n1"))
	requireHasKey(t, bb.db, keyNodeByHash("sha256:abc", "n1"))
	requireHasKey(t, bb.db, keyNodeByMedia("text/x-go", "n1"))
}

func TestIndex_NodeDeleteRemovesIndexes(t *testing.T) {
	bb, err := newBadgerBackend(BadgerOptions{InMemory: true})
	require.NoError(t, err)
	defer bb.close()

	require.NoError(t, bb.commit(context.TODO(), mutationBatch{
		opName: "upsert_node",
		op:     nodeOp{Node: NodeRecord{ID: "n1", Kind: "function", SourceID: "s.go", Labels: []string{"tag:x"}}},
	}))

	requireHasKey(t, bb.db, keyNodeByKind("function", "n1"))
	requireHasKey(t, bb.db, keyNodeBySource("s.go", "n1"))
	requireHasKey(t, bb.db, keyNodeByLabel("tag:x", "n1"))

	require.NoError(t, bb.commit(context.TODO(), mutationBatch{
		opName: "delete_node",
		op:     deleteNodeOp{ID: "n1"},
	}))

	requireNotHasKey(t, bb.db, keyNodeByKind("function", "n1"))
	requireNotHasKey(t, bb.db, keyNodeBySource("s.go", "n1"))
	requireNotHasKey(t, bb.db, keyNodeByLabel("tag:x", "n1"))
}

func TestIndex_NodeUpdateRemovesStaleLabels(t *testing.T) {
	bb, err := newBadgerBackend(BadgerOptions{InMemory: true})
	require.NoError(t, err)
	defer bb.close()

	// Create with label tag:a
	require.NoError(t, bb.commit(context.TODO(), mutationBatch{
		opName: "upsert_node",
		op:     nodeOp{Node: NodeRecord{ID: "n1", Kind: "function", Labels: []string{"tag:a"}}},
	}))

	requireHasKey(t, bb.db, keyNodeByLabel("tag:a", "n1"))

	// Update with label tag:b instead
	require.NoError(t, bb.commit(context.TODO(), mutationBatch{
		opName: "upsert_node",
		op:     nodeOp{Node: NodeRecord{ID: "n1", Kind: "function", Labels: []string{"tag:b"}}},
	}))

	// Old label tag:a should be removed
	requireNotHasKey(t, bb.db, keyNodeByLabel("tag:a", "n1"))
	// New label tag:b should be present
	requireHasKey(t, bb.db, keyNodeByLabel("tag:b", "n1"))
}

func TestIndex_NodeUpdateRemovesStaleSource(t *testing.T) {
	bb, err := newBadgerBackend(BadgerOptions{InMemory: true})
	require.NoError(t, err)
	defer bb.close()

	require.NoError(t, bb.commit(context.TODO(), mutationBatch{
		opName: "upsert_node",
		op:     nodeOp{Node: NodeRecord{ID: "n1", Kind: "function", SourceID: "old.go"}},
	}))

	requireHasKey(t, bb.db, keyNodeBySource("old.go", "n1"))

	require.NoError(t, bb.commit(context.TODO(), mutationBatch{
		opName: "upsert_node",
		op:     nodeOp{Node: NodeRecord{ID: "n1", Kind: "function", SourceID: "new.go"}},
	}))

	requireNotHasKey(t, bb.db, keyNodeBySource("old.go", "n1"))
	requireHasKey(t, bb.db, keyNodeBySource("new.go", "n1"))
}

func TestIndex_NodeUpdateRemovesStalePath(t *testing.T) {
	bb, err := newBadgerBackend(BadgerOptions{InMemory: true})
	require.NoError(t, err)
	defer bb.close()

	require.NoError(t, bb.commit(context.TODO(), mutationBatch{
		opName: "upsert_node",
		op: nodeOp{Node: NodeRecord{
			ID:    "n1",
			Kind:  "file",
			Props: json.RawMessage(`{"path":"old.go"}`),
		}},
	}))

	requireHasKey(t, bb.db, keyNodeByPath("old.go", "n1"))

	require.NoError(t, bb.commit(context.TODO(), mutationBatch{
		opName: "upsert_node",
		op: nodeOp{Node: NodeRecord{
			ID:    "n1",
			Kind:  "file",
			Props: json.RawMessage(`{"path":"new.go"}`),
		}},
	}))

	requireNotHasKey(t, bb.db, keyNodeByPath("old.go", "n1"))
	requireHasKey(t, bb.db, keyNodeByPath("new.go", "n1"))
}

func TestIndex_EdgeStableID(t *testing.T) {
	bb, err := newBadgerBackend(BadgerOptions{InMemory: true})
	require.NoError(t, err)
	defer bb.close()

	require.NoError(t, bb.commit(context.TODO(), mutationBatch{
		opName: "link_edge",
		op: edgeOp{Edge: EdgeRecord{
			SourceID: "a",
			TargetID: "b",
			Kind:     "calls",
			StableID: "edge-stable-1",
		}},
	}))

	requireHasKey(t, bb.db, keyEdgeByStable("edge-stable-1", "a", "b", "calls"))
}

func TestIndex_EdgeHardUnlinkRemovesIndex(t *testing.T) {
	bb, err := newBadgerBackend(BadgerOptions{InMemory: true})
	require.NoError(t, err)
	defer bb.close()

	require.NoError(t, bb.commit(context.TODO(), mutationBatch{
		opName: "link_edge",
		op: edgeOp{Edge: EdgeRecord{
			SourceID: "a",
			TargetID: "b",
			Kind:     "calls",
			StableID: "stable-1",
		}},
	}))

	requireHasKey(t, bb.db, keyEdgeByStable("stable-1", "a", "b", "calls"))

	require.NoError(t, bb.commit(context.TODO(), mutationBatch{
		opName: "unlink_edge",
		op:     unlinkOp{SourceID: "a", TargetID: "b", Kind: "calls", Hard: true},
	}))

	requireNotHasKey(t, bb.db, keyEdgeByStable("stable-1", "a", "b", "calls"))
}

func TestIndex_EdgeNoStableIDNoIndex(t *testing.T) {
	bb, err := newBadgerBackend(BadgerOptions{InMemory: true})
	require.NoError(t, err)
	defer bb.close()

	require.NoError(t, bb.commit(context.TODO(), mutationBatch{
		opName: "link_edge",
		op: edgeOp{Edge: EdgeRecord{
			SourceID: "a",
			TargetID: "b",
			Kind:     "calls",
		}},
	}))

	// No stable index key should exist
	n := countPrefix(t, bb.db, keyPrefix(famEdgeStable))
	require.Zero(t, n)
}

func TestIndex_AnnotateNodeUpdatesPathHash(t *testing.T) {
	bb, err := newBadgerBackend(BadgerOptions{InMemory: true})
	require.NoError(t, err)
	defer bb.close()

	require.NoError(t, bb.commit(context.TODO(), mutationBatch{
		opName: "upsert_node",
		op: nodeOp{Node: NodeRecord{
			ID:    "n1",
			Kind:  "file",
			Props: json.RawMessage(`{"path":"a.go"}`),
		}},
	}))

	requireHasKey(t, bb.db, keyNodeByPath("a.go", "n1"))

	// Annotate with new path
	require.NoError(t, bb.commit(context.TODO(), mutationBatch{
		opName: "annotate_node",
		op:     annotateNodeOp{ID: "n1", Props: map[string]any{"path": "b.go"}},
	}))

	requireNotHasKey(t, bb.db, keyNodeByPath("a.go", "n1"))
	requireHasKey(t, bb.db, keyNodeByPath("b.go", "n1"))
}

func TestIndex_PropsWithoutIndexedFields(t *testing.T) {
	bb, err := newBadgerBackend(BadgerOptions{InMemory: true})
	require.NoError(t, err)
	defer bb.close()

	// Props with valid JSON but no expected indexable fields should not
	// create path/hash/media indexes.
	require.NoError(t, bb.commit(context.TODO(), mutationBatch{
		opName: "upsert_node",
		op: nodeOp{Node: NodeRecord{
			ID:    "n1",
			Kind:  "file",
			Props: json.RawMessage(`{"unrelated":42,"data":"hello"}`),
		}},
	}))

	// The node kind index should still be written.
	requireHasKey(t, bb.db, keyNodeByKind("file", "n1"))

	// No path/hash/media prefix keys should exist.
	n := countPrefix(t, bb.db, keyPrefix(famNodePath))
	require.Zero(t, n)
	n = countPrefix(t, bb.db, keyPrefix(famNodeHash))
	require.Zero(t, n)
	n = countPrefix(t, bb.db, keyPrefix(famNodeMedia))
	require.Zero(t, n)
}

func TestIndex_RebuildAfterDeletion(t *testing.T) {
	bb, err := newBadgerBackend(BadgerOptions{InMemory: true})
	require.NoError(t, err)
	defer bb.close()

	// Insert nodes and edges
	require.NoError(t, bb.commit(context.TODO(), mutationBatch{
		opName: "upsert_node",
		op:     nodeOp{Node: NodeRecord{ID: "n1", Kind: "function", SourceID: "s.go", Labels: []string{"tag:x"}}},
	}))
	require.NoError(t, bb.commit(context.TODO(), mutationBatch{
		opName: "upsert_node",
		op: nodeOp{Node: NodeRecord{
			ID:    "n2",
			Kind:  "file",
			Props: json.RawMessage(`{"path":"main.go"}`),
		}},
	}))
	require.NoError(t, bb.commit(context.TODO(), mutationBatch{
		opName: "link_edge",
		op: edgeOp{Edge: EdgeRecord{
			SourceID: "n1",
			TargetID: "n2",
			Kind:     "calls",
			StableID: "edge-s1",
		}},
	}))

	// Deliberately delete index keys
	require.NoError(t, bb.db.Update(func(txn *badger.Txn) error {
		_ = txn.Delete(keyNodeByKind("function", "n1"))
		_ = txn.Delete(keyNodeBySource("s.go", "n1"))
		_ = txn.Delete(keyNodeByLabel("tag:x", "n1"))
		_ = txn.Delete(keyNodeByPath("main.go", "n2"))
		_ = txn.Delete(keyEdgeByStable("edge-s1", "n1", "n2", "calls"))
		return nil
	}))

	// Verify indexes are gone
	requireNotHasKey(t, bb.db, keyNodeByKind("function", "n1"))

	// Rebuild
	require.NoError(t, bb.rebuildIndexes())

	// Verify indexes are back
	requireHasKey(t, bb.db, keyNodeByKind("function", "n1"))
	requireHasKey(t, bb.db, keyNodeBySource("s.go", "n1"))
	requireHasKey(t, bb.db, keyNodeByLabel("tag:x", "n1"))
	requireHasKey(t, bb.db, keyNodeByPath("main.go", "n2"))
	requireHasKey(t, bb.db, keyEdgeByStable("edge-s1", "n1", "n2", "calls"))
}

func TestIndex_BatchUpsertNodes(t *testing.T) {
	bb, err := newBadgerBackend(BadgerOptions{InMemory: true})
	require.NoError(t, err)
	defer bb.close()

	require.NoError(t, bb.commit(context.TODO(), mutationBatch{
		opName: "upsert_nodes",
		op: nodeBatchOp{Nodes: []NodeRecord{
			{ID: "n1", Kind: "function", SourceID: "a.go"},
			{ID: "n2", Kind: "method", SourceID: "b.go"},
		}},
	}))

	requireHasKey(t, bb.db, keyNodeByKind("function", "n1"))
	requireHasKey(t, bb.db, keyNodeByKind("method", "n2"))
	requireHasKey(t, bb.db, keyNodeBySource("a.go", "n1"))
	requireHasKey(t, bb.db, keyNodeBySource("b.go", "n2"))
}

func TestIndex_BatchDeleteNodes(t *testing.T) {
	bb, err := newBadgerBackend(BadgerOptions{InMemory: true})
	require.NoError(t, err)
	defer bb.close()

	require.NoError(t, bb.commit(context.TODO(), mutationBatch{
		opName: "upsert_nodes",
		op: nodeBatchOp{Nodes: []NodeRecord{
			{ID: "n1", Kind: "function", SourceID: "a.go"},
			{ID: "n2", Kind: "function", SourceID: "b.go"},
		}},
	}))

	require.NoError(t, bb.commit(context.TODO(), mutationBatch{
		opName: "delete_nodes",
		op:     deleteNodesOp{IDs: []string{"n1", "n2"}},
	}))

	requireNotHasKey(t, bb.db, keyNodeByKind("function", "n1"))
	requireNotHasKey(t, bb.db, keyNodeBySource("a.go", "n1"))
	requireNotHasKey(t, bb.db, keyNodeByKind("function", "n2"))
	requireNotHasKey(t, bb.db, keyNodeBySource("b.go", "n2"))
}

func TestIndex_BatchLinkEdges(t *testing.T) {
	bb, err := newBadgerBackend(BadgerOptions{InMemory: true})
	require.NoError(t, err)
	defer bb.close()

	require.NoError(t, bb.commit(context.TODO(), mutationBatch{
		opName: "link_edges",
		op: edgeBatchOp{Edges: []EdgeRecord{
			{SourceID: "a", TargetID: "b", Kind: "calls", StableID: "s1"},
			{SourceID: "b", TargetID: "c", Kind: "calls", StableID: "s2"},
		}},
	}))

	requireHasKey(t, bb.db, keyEdgeByStable("s1", "a", "b", "calls"))
	requireHasKey(t, bb.db, keyEdgeByStable("s2", "b", "c", "calls"))
}

// ────────────────────────────────────────────────────────────────────
// Props extraction tests
// ────────────────────────────────────────────────────────────────────

func TestExtractIndexedProps(t *testing.T) {
	path, hash, media := extractIndexedProps(json.RawMessage(`{"path":"a.go","content_hash":"sha256:x","media_type":"text/plain"}`))
	require.Equal(t, "a.go", path)
	require.Equal(t, "sha256:x", hash)
	require.Equal(t, "text/plain", media)
}

func TestExtractIndexedProps_Partial(t *testing.T) {
	path, hash, media := extractIndexedProps(json.RawMessage(`{"path":"a.go"}`))
	require.Equal(t, "a.go", path)
	require.Empty(t, hash)
	require.Empty(t, media)
}

func TestExtractIndexedProps_Empty(t *testing.T) {
	path, hash, media := extractIndexedProps(nil)
	require.Empty(t, path)
	require.Empty(t, hash)
	require.Empty(t, media)

	path, hash, media = extractIndexedProps(json.RawMessage(`{}`))
	require.Empty(t, path)
	require.Empty(t, hash)
	require.Empty(t, media)
}

func TestExtractIndexedProps_Malformed(t *testing.T) {
	path, hash, media := extractIndexedProps(json.RawMessage(`{bad json}`))
	require.Empty(t, path)
	require.Empty(t, hash)
	require.Empty(t, media)
}

func TestExtractIndexedProps_UnknownFields(t *testing.T) {
	path, hash, media := extractIndexedProps(json.RawMessage(`{"unknown":"value"}`))
	require.Empty(t, path)
	require.Empty(t, hash)
	require.Empty(t, media)
}
