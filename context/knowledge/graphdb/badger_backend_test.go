package graphdb

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// newBadgerTestEngine opens an Engine backed by an in‑memory Badger instance.
func newBadgerTestEngine(t *testing.T) (*Engine, Options) {
	t.Helper()
	opts := Options{
		DataDir:                  t.TempDir(),
		AOFFileName:              "dummy.aof",
		SnapshotFileName:         "dummy.snap",
		AutoSaveInterval:         0,
		AutoSaveThreshold:        0,
		AOFRewriteThresholdBytes: 0,
	}

	bb, err := newBadgerBackend(BadgerOptions{InMemory: true})
	require.NoError(t, err)

	engine := &Engine{
		opts:   opts,
		store:  newAdjacencyStore(),
		bk:     bb,
		stopCh: make(chan struct{}),
	}
	engine.lastSave.Store(time.Now().UnixNano())
	engine.wg.Add(1)
	go engine.background(context.Background())

	t.Cleanup(func() {
		engine.stopOnce.Do(func() {
			close(engine.stopCh)
			engine.wg.Wait()
		})
		if engine.bk != nil {
			_ = engine.bk.close()
		}
	})

	return engine, opts
}

// ────────────────────────────────────────────────────────────────────
// BadgerBackend tests
// ────────────────────────────────────────────────────────────────────

func TestBadgerBackend_OpenClose(t *testing.T) {
	bb, err := newBadgerBackend(BadgerOptions{InMemory: true})
	require.NoError(t, err)
	require.NotNil(t, bb)
	require.NoError(t, bb.close())
}

func TestBadgerBackend_OpenDir(t *testing.T) {
	dir := t.TempDir()
	bb, err := newBadgerBackend(BadgerOptions{Dir: dir})
	require.NoError(t, err)
	require.NotNil(t, bb)
	require.NoError(t, bb.close())
}

func TestBadgerBackend_OpenNoDir(t *testing.T) {
	_, err := newBadgerBackend(BadgerOptions{Dir: "", InMemory: false})
	require.Error(t, err)
}

func TestBadgerBackend_UpsertNodeAndReopen(t *testing.T) {
	dir := t.TempDir()

	bb, err := newBadgerBackend(BadgerOptions{Dir: dir})
	require.NoError(t, err)
	require.NoError(t, bb.commit(context.TODO(), mutationBatch{
		opName: "upsert_node",
		op:     nodeOp{Node: NodeRecord{ID: "n1", Kind: "function", SourceID: "a.go"}},
	}))
	require.NoError(t, bb.close())

	bb2, err := newBadgerBackend(BadgerOptions{Dir: dir})
	require.NoError(t, err)
	defer bb2.close()

	store2 := newAdjacencyStore()
	require.NoError(t, bb2.load(context.TODO(), store2))

	node, ok := store2.nodes["n1"]
	require.True(t, ok)
	require.Equal(t, NodeKind("function"), node.Kind)
	require.Equal(t, "a.go", node.SourceID)
}

func TestBadgerBackend_LinkEdgeAndReopen(t *testing.T) {
	dir := t.TempDir()

	bb, err := newBadgerBackend(BadgerOptions{Dir: dir})
	require.NoError(t, err)

	require.NoError(t, bb.commit(context.TODO(), mutationBatch{
		opName: "upsert_node",
		op:     nodeOp{Node: NodeRecord{ID: "a", Kind: "function"}},
	}))
	require.NoError(t, bb.commit(context.TODO(), mutationBatch{
		opName: "upsert_node",
		op:     nodeOp{Node: NodeRecord{ID: "b", Kind: "function"}},
	}))
	require.NoError(t, bb.commit(context.TODO(), mutationBatch{
		opName: "link_edge",
		op:     edgeOp{Edge: EdgeRecord{SourceID: "a", TargetID: "b", Kind: "calls", Weight: 1}},
	}))
	require.NoError(t, bb.close())

	bb2, err := newBadgerBackend(BadgerOptions{Dir: dir})
	require.NoError(t, err)
	defer bb2.close()

	store2 := newAdjacencyStore()
	require.NoError(t, bb2.load(context.TODO(), store2))

	require.Contains(t, store2.nodes, "a")
	require.Contains(t, store2.nodes, "b")
	require.Len(t, store2.forward["a"], 1)
	require.Equal(t, "b", store2.forward["a"][0].TargetID)
	require.Len(t, store2.reverse["b"], 1)
	require.Equal(t, "a", store2.reverse["b"][0].SourceID)
}

func TestBadgerBackend_DeleteNodeRemovesRecord(t *testing.T) {
	dir := t.TempDir()

	bb, err := newBadgerBackend(BadgerOptions{Dir: dir})
	require.NoError(t, err)

	require.NoError(t, bb.commit(context.TODO(), mutationBatch{
		opName: "upsert_node",
		op:     nodeOp{Node: NodeRecord{ID: "del", Kind: "function"}},
	}))
	require.NoError(t, bb.commit(context.TODO(), mutationBatch{
		opName: "delete_node",
		op:     deleteNodeOp{ID: "del"},
	}))
	require.NoError(t, bb.close())

	bb2, err := newBadgerBackend(BadgerOptions{Dir: dir})
	require.NoError(t, err)
	defer bb2.close()

	store2 := newAdjacencyStore()
	require.NoError(t, bb2.load(context.TODO(), store2))
	// Node should be present but soft-deleted (DeletedAt != 0).
	node, ok := store2.nodes["del"]
	require.True(t, ok, "node should be present after soft-delete")
	require.NotZero(t, node.DeletedAt, "node should be marked as deleted")
}

func TestBadgerBackend_HardUnlinkEdge(t *testing.T) {
	dir := t.TempDir()

	bb, err := newBadgerBackend(BadgerOptions{Dir: dir})
	require.NoError(t, err)

	require.NoError(t, bb.commit(context.TODO(), mutationBatch{
		opName: "upsert_node",
		op:     nodeOp{Node: NodeRecord{ID: "x", Kind: "function"}},
	}))
	require.NoError(t, bb.commit(context.TODO(), mutationBatch{
		opName: "upsert_node",
		op:     nodeOp{Node: NodeRecord{ID: "y", Kind: "function"}},
	}))
	require.NoError(t, bb.commit(context.TODO(), mutationBatch{
		opName: "link_edge",
		op:     edgeOp{Edge: EdgeRecord{SourceID: "x", TargetID: "y", Kind: "calls"}},
	}))
	require.NoError(t, bb.commit(context.TODO(), mutationBatch{
		opName: "unlink_edge",
		op:     unlinkOp{SourceID: "x", TargetID: "y", Kind: "calls", Hard: true},
	}))
	require.NoError(t, bb.close())

	bb2, err := newBadgerBackend(BadgerOptions{Dir: dir})
	require.NoError(t, err)
	defer bb2.close()

	store2 := newAdjacencyStore()
	require.NoError(t, bb2.load(context.TODO(), store2))
	require.Empty(t, store2.forward["x"])
	require.Empty(t, store2.reverse["y"])
}

func TestBadgerBackend_MutationResult(t *testing.T) {
	dir := t.TempDir()

	bb, err := newBadgerBackend(BadgerOptions{Dir: dir})
	require.NoError(t, err)

	result := MutationResult{
		StableID:  "mut-1",
		Scope:     MutationScopeNode,
		Status:    MutationStatusCreated,
		TaskID:    "t1",
		SessionID: "s1",
	}
	require.NoError(t, bb.commit(context.TODO(), mutationBatch{
		opName: "record_mutation_result",
		op:     mutationResultOp{Result: result},
	}))
	require.NoError(t, bb.close())

	bb2, err := newBadgerBackend(BadgerOptions{Dir: dir})
	require.NoError(t, err)
	defer bb2.close()

	store2 := newAdjacencyStore()
	require.NoError(t, bb2.load(context.TODO(), store2))
	// Mutation results are no longer loaded into the adjacency store (FR-11).
	// Read from Badger directly instead.
	mutPtr, getErr := bb2.getMutationResult("mut-1")
	require.NoError(t, getErr)
	require.NotNil(t, mutPtr)
	require.Equal(t, MutationScopeNode, mutPtr.Scope)
}

func TestBadgerBackend_AnnotateNode(t *testing.T) {
	dir := t.TempDir()

	bb, err := newBadgerBackend(BadgerOptions{Dir: dir})
	require.NoError(t, err)

	require.NoError(t, bb.commit(context.TODO(), mutationBatch{
		opName: "upsert_node",
		op:     nodeOp{Node: NodeRecord{ID: "n", Kind: "function", Props: []byte(`{"a":1}`)}},
	}))
	require.NoError(t, bb.commit(context.TODO(), mutationBatch{
		opName: "annotate_node",
		op:     annotateNodeOp{ID: "n", Props: map[string]any{"b": 2}},
	}))
	require.NoError(t, bb.close())

	bb2, err := newBadgerBackend(BadgerOptions{Dir: dir})
	require.NoError(t, err)
	defer bb2.close()

	store2 := newAdjacencyStore()
	require.NoError(t, bb2.load(context.TODO(), store2))
	node := store2.nodes["n"]
	require.NotNil(t, node)
	require.JSONEq(t, `{"a":1,"b":2}`, string(node.Props))
}

func TestBadgerBackend_BatchNodesAndEdges(t *testing.T) {
	dir := t.TempDir()

	bb, err := newBadgerBackend(BadgerOptions{Dir: dir})
	require.NoError(t, err)

	require.NoError(t, bb.commit(context.TODO(), mutationBatch{
		opName: "upsert_nodes",
		op: nodeBatchOp{Nodes: []NodeRecord{
			{ID: "n1", Kind: "function"},
			{ID: "n2", Kind: "function"},
			{ID: "n3", Kind: "function"},
		}},
	}))
	require.NoError(t, bb.commit(context.TODO(), mutationBatch{
		opName: "link_edges",
		op: edgeBatchOp{Edges: []EdgeRecord{
			{SourceID: "n1", TargetID: "n2", Kind: "calls", Weight: 1},
			{SourceID: "n2", TargetID: "n3", Kind: "calls", Weight: 1},
		}},
	}))
	require.NoError(t, bb.close())

	bb2, err := newBadgerBackend(BadgerOptions{Dir: dir})
	require.NoError(t, err)
	defer bb2.close()

	store2 := newAdjacencyStore()
	require.NoError(t, bb2.load(context.TODO(), store2))
	require.Len(t, store2.nodes, 3)
	require.Len(t, store2.forward["n1"], 1)
	require.Len(t, store2.forward["n2"], 1)
	require.Len(t, store2.reverse["n3"], 1)
}

func TestBadgerBackend_LoadEmptyStore(t *testing.T) {
	bb, err := newBadgerBackend(BadgerOptions{InMemory: true})
	require.NoError(t, err)
	defer bb.close()

	store := newAdjacencyStore()
	require.NoError(t, bb.load(context.TODO(), store))
	require.Empty(t, store.nodes)
	require.Empty(t, store.forward)
	require.Empty(t, store.reverse)
}

func TestBadgerBackend_UpsertNodeThroughEngine(t *testing.T) {
	engine, _ := newBadgerTestEngine(t)
	require.NoError(t, engine.UpsertNode(context.TODO(), NodeRecord{ID: "n1", Kind: "function", SourceID: "x.go"}))

	node, ok := engine.GetNode("n1")
	require.True(t, ok)
	require.Equal(t, "x.go", node.SourceID)
}

func TestBadgerBackend_LinkEdgeThroughEngine(t *testing.T) {
	engine, _ := newBadgerTestEngine(t)
	require.NoError(t, engine.UpsertNode(context.TODO(), NodeRecord{ID: "a", Kind: "function"}))
	require.NoError(t, engine.UpsertNode(context.TODO(), NodeRecord{ID: "b", Kind: "function"}))
	require.NoError(t, engine.Link(context.TODO(), "a", "b", "calls", "", 1, nil))

	out := engine.GetOutEdges("a", "calls")
	require.Len(t, out, 1)
	require.Equal(t, "b", out[0].TargetID)

	in := engine.GetInEdges("b", "calls")
	require.Len(t, in, 1)
	require.Equal(t, "a", in[0].SourceID)
}

func TestBadgerBackend_DeleteNodeThroughEngine(t *testing.T) {
	engine, _ := newBadgerTestEngine(t)
	require.NoError(t, engine.UpsertNode(context.TODO(), NodeRecord{ID: "del", Kind: "function"}))
	_, ok := engine.GetNode("del")
	require.True(t, ok)

	require.NoError(t, engine.DeleteNode(context.TODO(), "del"))
	_, ok = engine.GetNode("del")
	require.False(t, ok)
}

func TestBadgerBackend_CommitFailsOnInvalidOp(t *testing.T) {
	bb, err := newBadgerBackend(BadgerOptions{InMemory: true})
	require.NoError(t, err)
	defer bb.close()

	err = bb.commit(context.TODO(), mutationBatch{
		opName: "upsert_node",
		op:     "not a nodeOp",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid upsert_node payload")
}

func TestBadgerBackend_FailedCommitLeavesStoreUnchanged(t *testing.T) {
	bb, err := newBadgerBackend(BadgerOptions{InMemory: true})
	require.NoError(t, err)
	defer bb.close()

	store := newAdjacencyStore()
	require.NoError(t, bb.load(context.TODO(), store))

	err = bb.commit(context.TODO(), mutationBatch{
		opName: "upsert_node",
		op:     "not a nodeOp",
	})
	require.Error(t, err)
	require.Empty(t, store.nodes)
}

func TestBadgerBackend_AnnotateNonexistentNode(t *testing.T) {
	bb, err := newBadgerBackend(BadgerOptions{InMemory: true})
	require.NoError(t, err)
	defer bb.close()

	err = bb.commit(context.TODO(), mutationBatch{
		opName: "annotate_node",
		op:     annotateNodeOp{ID: "ghost", Props: map[string]any{"x": 1}},
	})
	require.NoError(t, err)
}
