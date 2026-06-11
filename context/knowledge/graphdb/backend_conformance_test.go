package graphdb

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// reopenFunc creates a new Engine backed by the same durable store. The
// caller does not need to register cleanup; the function does it.
type reopenFunc func(t *testing.T) *Engine

// engineFactory opens a persistent Engine for testing and returns a
// reopen function for the same store location.
type engineFactory func(t *testing.T) (*Engine, reopenFunc)

// testBackendConformance runs the full backend behaviour suite against
// the given factory. Each sub‑test is a real test.
func testBackendConformance(t *testing.T, factory engineFactory) {
	t.Helper()

	t.Run("node_upsert_and_get", func(t *testing.T) {
		eng, _ := factory(t)
		require.NoError(t, eng.UpsertNode(context.TODO(), NodeRecord{ID: "n1", Kind: "function", SourceID: "a.go"}))
		node, ok := eng.GetNode("n1")
		require.True(t, ok)
		require.Equal(t, "n1", node.ID)
		require.Equal(t, NodeKind("function"), node.Kind)
		require.Equal(t, "a.go", node.SourceID)
		require.NotZero(t, node.CreatedAt)
		require.NotZero(t, node.UpdatedAt)
		require.Zero(t, node.DeletedAt)
	})

	t.Run("node_upsert_update_existing", func(t *testing.T) {
		eng, _ := factory(t)
		require.NoError(t, eng.UpsertNode(context.TODO(), NodeRecord{ID: "n", Kind: "function", SourceID: "old.go"}))
		require.NoError(t, eng.UpsertNode(context.TODO(), NodeRecord{ID: "n", Kind: "method", SourceID: "new.go"}))

		node, ok := eng.GetNode("n")
		require.True(t, ok)
		require.Equal(t, NodeKind("method"), node.Kind)
		require.Equal(t, "new.go", node.SourceID)
		require.NotZero(t, node.CreatedAt)
		require.NotZero(t, node.UpdatedAt)
	})

	t.Run("node_upsert_and_reopen", func(t *testing.T) {
		eng, reopen := factory(t)
		require.NoError(t, eng.UpsertNode(context.TODO(), NodeRecord{ID: "persist", Kind: "function", SourceID: "x.go"}))
		require.NoError(t, eng.Close(context.Background()))

		eng2 := reopen(t)
		node, ok := eng2.GetNode("persist")
		require.True(t, ok)
		require.Equal(t, "x.go", node.SourceID)
	})

	t.Run("edge_link_and_get", func(t *testing.T) {
		eng, _ := factory(t)
		require.NoError(t, eng.UpsertNode(context.TODO(), NodeRecord{ID: "a", Kind: "function"}))
		require.NoError(t, eng.UpsertNode(context.TODO(), NodeRecord{ID: "b", Kind: "function"}))
		require.NoError(t, eng.Link(context.TODO(), "a", "b", "calls", "", 1, map[string]any{"line": 42}))

		out := eng.GetOutEdges("a", "calls")
		require.Len(t, out, 1)
		require.Equal(t, "b", out[0].TargetID)
		require.Equal(t, EdgeKind("calls"), out[0].Kind)
		require.JSONEq(t, `{"line":42}`, string(out[0].Props))

		in := eng.GetInEdges("b", "calls")
		require.Len(t, in, 1)
		require.Equal(t, "a", in[0].SourceID)
	})

	t.Run("edge_link_and_reopen", func(t *testing.T) {
		eng, reopen := factory(t)
		require.NoError(t, eng.UpsertNode(context.TODO(), NodeRecord{ID: "x", Kind: "function"}))
		require.NoError(t, eng.UpsertNode(context.TODO(), NodeRecord{ID: "y", Kind: "function"}))
		require.NoError(t, eng.Link(context.TODO(), "x", "y", "depends", "", 1, nil))
		require.NoError(t, eng.Close(context.Background()))

		eng2 := reopen(t)
		out := eng2.GetOutEdges("x", "depends")
		require.Len(t, out, 1)
		require.Equal(t, "y", out[0].TargetID)

		in := eng2.GetInEdges("y", "depends")
		require.Len(t, in, 1)
		require.Equal(t, "x", in[0].SourceID)
	})

	t.Run("edge_link_with_inverse", func(t *testing.T) {
		eng, _ := factory(t)
		require.NoError(t, eng.UpsertNode(context.TODO(), NodeRecord{ID: "a", Kind: "function"}))
		require.NoError(t, eng.UpsertNode(context.TODO(), NodeRecord{ID: "b", Kind: "function"}))
		require.NoError(t, eng.Link(context.TODO(), "a", "b", "calls", "called_by", 1, nil))

		outA := eng.GetOutEdges("a", "calls")
		require.Len(t, outA, 1)

		outB := eng.GetOutEdges("b", "called_by")
		require.Len(t, outB, 1)
		require.Equal(t, "a", outB[0].TargetID)
	})

	t.Run("soft_delete_node_soft_deletes_edges", func(t *testing.T) {
		eng, _ := factory(t)
		for _, id := range []string{"a", "b", "c"} {
			require.NoError(t, eng.UpsertNode(context.TODO(), NodeRecord{ID: id, Kind: "function"}))
		}
		require.NoError(t, eng.Link(context.TODO(), "a", "b", "calls", "", 1, nil))
		require.NoError(t, eng.Link(context.TODO(), "c", "b", "calls", "", 1, nil))

		require.NoError(t, eng.DeleteNode(context.TODO(), "b"))

		_, ok := eng.GetNode("b")
		require.False(t, ok)

		out := allOutEdges(t, eng, "a")
		require.Len(t, out, 1)
		require.False(t, out[0].IsActive())

		in := allInEdges(t, eng, "b")
		require.Len(t, in, 2)
		for _, e := range in {
			require.False(t, e.IsActive())
		}
	})

	t.Run("soft_delete_node_reopen", func(t *testing.T) {
		eng, reopen := factory(t)
		require.NoError(t, eng.UpsertNode(context.TODO(), NodeRecord{ID: "del", Kind: "function"}))
		require.NoError(t, eng.UpsertNode(context.TODO(), NodeRecord{ID: "keep", Kind: "function"}))
		require.NoError(t, eng.Link(context.TODO(), "del", "keep", "calls", "", 1, nil))
		require.NoError(t, eng.DeleteNode(context.TODO(), "del"))
		require.NoError(t, eng.Close(context.Background()))

		eng2 := reopen(t)
		_, ok := eng2.GetNode("del")
		require.False(t, ok)

		_, ok = eng2.GetNode("keep")
		require.True(t, ok)

		out := allOutEdges(t, eng2, "del")
		require.Len(t, out, 1)
		require.False(t, out[0].IsActive())
	})

	t.Run("label_lookup", func(t *testing.T) {
		eng, _ := factory(t)
		require.NoError(t, eng.UpsertNode(context.TODO(), NodeRecord{
			ID:     "n1",
			Kind:   "function",
			Labels: []string{"tag:a", "tag:b"},
		}))
		require.NoError(t, eng.UpsertNode(context.TODO(), NodeRecord{
			ID:     "n2",
			Kind:   "method",
			Labels: []string{"tag:a"},
		}))
		require.NoError(t, eng.UpsertNode(context.TODO(), NodeRecord{
			ID:     "n3",
			Kind:   "function",
			Labels: []string{"tag:c"},
		}))

		nodes := eng.ListNodesByLabel("", "tag:a")
		require.Len(t, nodes, 2)

		nodes = eng.ListNodesByLabel("function", "tag:a")
		require.Len(t, nodes, 1)
		require.Equal(t, "n1", nodes[0].ID)
	})

	t.Run("label_prefix_lookup", func(t *testing.T) {
		eng, _ := factory(t)
		require.NoError(t, eng.UpsertNode(context.TODO(), NodeRecord{
			ID:     "n1",
			Kind:   "file",
			Labels: []string{"path:/src/main.go"},
		}))
		require.NoError(t, eng.UpsertNode(context.TODO(), NodeRecord{
			ID:     "n2",
			Kind:   "file",
			Labels: []string{"path:/src/util.go"},
		}))

		nodes := eng.ListNodesByLabelPrefix("", "path:/src")
		require.Len(t, nodes, 2)

		nodes = eng.ListNodesByLabelPrefix("", "path:/src/main")
		require.Len(t, nodes, 1)
		require.Equal(t, "n1", nodes[0].ID)
	})

	t.Run("label_lookup_reopen", func(t *testing.T) {
		eng, reopen := factory(t)
		require.NoError(t, eng.UpsertNode(context.TODO(), NodeRecord{
			ID:     "keep",
			Kind:   "file",
			Labels: []string{"tag:persist"},
		}))
		require.NoError(t, eng.UpsertNode(context.TODO(), NodeRecord{
			ID:     "gone",
			Kind:   "file",
			Labels: []string{"tag:removed"},
		}))
		require.NoError(t, eng.DeleteNode(context.TODO(), "gone"))
		require.NoError(t, eng.Close(context.Background()))

		eng2 := reopen(t)
		nodes := eng2.ListNodesByLabel("", "tag:persist")
		require.Len(t, nodes, 1)
		require.Equal(t, "keep", nodes[0].ID)

		nodes = eng2.ListNodesByLabel("", "tag:removed")
		require.Empty(t, nodes)
	})

	t.Run("nodes_by_source", func(t *testing.T) {
		eng, _ := factory(t)
		require.NoError(t, eng.UpsertNode(context.TODO(), NodeRecord{ID: "a", Kind: "function", SourceID: "src.go"}))
		require.NoError(t, eng.UpsertNode(context.TODO(), NodeRecord{ID: "b", Kind: "method", SourceID: "src.go"}))
		require.NoError(t, eng.UpsertNode(context.TODO(), NodeRecord{ID: "c", Kind: "function", SourceID: "other.go"}))

		nodes := eng.NodesBySource("src.go")
		require.Len(t, nodes, 2)

		ids := make([]string, 0, 2)
		for _, n := range nodes {
			ids = append(ids, n.ID)
		}
		require.ElementsMatch(t, []string{"a", "b"}, ids)
	})

	t.Run("nodes_by_source_reopen", func(t *testing.T) {
		eng, reopen := factory(t)
		require.NoError(t, eng.UpsertNode(context.TODO(), NodeRecord{ID: "x", Kind: "function", SourceID: "work.go"}))
		require.NoError(t, eng.UpsertNode(context.TODO(), NodeRecord{ID: "y", Kind: "function", SourceID: "work.go"}))
		require.NoError(t, eng.Close(context.Background()))

		eng2 := reopen(t)
		nodes := eng2.NodesBySource("work.go")
		require.Len(t, nodes, 2)
	})

	t.Run("mutation_result_persistence", func(t *testing.T) {
		eng, reopen := factory(t)
		result := MutationResult{
			Scope:        MutationScopeNode,
			Status:       MutationStatusCreated,
			Reason:       "test",
			TaskID:       "task-1",
			SessionID:    "session-1",
			StateVersion: 1,
			Details:      map[string]any{"key": "val"},
		}
		result.Normalize(result.TaskID, result.SessionID)
		stableID := result.StableID
		require.NotEmpty(t, stableID)
		require.NoError(t, eng.RecordMutationResult(context.TODO(), result))

		got, ok := eng.MutationResult(stableID)
		require.True(t, ok)
		require.Equal(t, stableID, got.StableID)
		require.Equal(t, MutationScopeNode, got.Scope)
		require.Equal(t, "val", got.Details["key"])
		require.NotEmpty(t, got.AppliedAt)
		require.NoError(t, eng.Close(context.Background()))

		eng2 := reopen(t)
		got2, ok := eng2.MutationResult(stableID)
		require.True(t, ok)
		require.Equal(t, stableID, got2.StableID)
	})

	t.Run("mutation_result_multiple", func(t *testing.T) {
		eng, _ := factory(t)
		r1 := MutationResult{Scope: MutationScopeNode, Status: MutationStatusCreated, TaskID: "t1", SessionID: "s1"}
		r1.Normalize(r1.TaskID, r1.SessionID)
		require.NoError(t, eng.RecordMutationResult(context.TODO(), r1))

		r2 := MutationResult{Scope: MutationScopeEdge, Status: MutationStatusUpdated, TaskID: "t1", SessionID: "s1"}
		r2.Normalize(r2.TaskID, r2.SessionID)
		require.NoError(t, eng.RecordMutationResult(context.TODO(), r2))

		results := eng.MutationResults()
		require.Len(t, results, 2)
		require.Equal(t, r1.StableID, results[0].StableID)
		require.Equal(t, r2.StableID, results[1].StableID)
	})

	t.Run("node_revision_history", func(t *testing.T) {
		eng, _ := factory(t)
		require.NoError(t, eng.UpsertNode(context.TODO(), NodeRecord{
			ID:    "n",
			Kind:  "function",
			Props: json.RawMessage(`{"v":1}`),
		}))
		require.NoError(t, eng.AnnotateNode(context.TODO(), "n", map[string]any{"note": "a"}))
		require.NoError(t, eng.UpsertNode(context.TODO(), NodeRecord{
			ID:    "n",
			Kind:  "function",
			Props: json.RawMessage(`{"v":2}`),
		}))

		revs := eng.NodeRevisions("n")
		require.Len(t, revs, 2)
		require.JSONEq(t, `{"v":1}`, string(revs[0].Props))
		require.JSONEq(t, `{"v":1,"note":"a"}`, string(revs[1].Props))
	})

	t.Run("node_revision_history_reopen", func(t *testing.T) {
		eng, reopen := factory(t)
		require.NoError(t, eng.UpsertNode(context.TODO(), NodeRecord{
			ID:    "n",
			Kind:  "function",
			Props: json.RawMessage(`{"v":1}`),
		}))
		require.NoError(t, eng.AnnotateNode(context.TODO(), "n", map[string]any{"note": "x"}))
		require.NoError(t, eng.Close(context.Background()))

		eng2 := reopen(t)
		revs := eng2.NodeRevisions("n")
		require.Len(t, revs, 1)
		require.JSONEq(t, `{"v":1}`, string(revs[0].Props))
	})

	t.Run("edge_revision_history", func(t *testing.T) {
		eng, _ := factory(t)
		require.NoError(t, eng.UpsertNode(context.TODO(), NodeRecord{ID: "a", Kind: "function"}))
		require.NoError(t, eng.UpsertNode(context.TODO(), NodeRecord{ID: "b", Kind: "function"}))
		require.NoError(t, eng.Link(context.TODO(), "a", "b", "calls", "", 1, map[string]any{"v": 1}))
		require.NoError(t, eng.AnnotateEdge(context.TODO(), "a", "b", "calls", map[string]any{"note": "x"}))
		require.NoError(t, eng.LinkEdges(context.TODO(), []EdgeRecord{
			{SourceID: "a", TargetID: "b", Kind: "calls", Weight: 2, Props: json.RawMessage(`{"v":2}`)},
		}))

		revs := eng.EdgeRevisions("a", "b", "calls")
		require.Len(t, revs, 2)
		require.JSONEq(t, `{"v":1}`, string(revs[0].Props))
		require.JSONEq(t, `{"note":"x","v":1}`, string(revs[1].Props))
	})

	t.Run("impact_set", func(t *testing.T) {
		eng, _ := factory(t)
		for _, id := range []string{"a", "b", "c", "d"} {
			require.NoError(t, eng.UpsertNode(context.TODO(), NodeRecord{ID: id, Kind: "function"}))
		}
		require.NoError(t, eng.Link(context.TODO(), "a", "b", "calls", "", 1, nil))
		require.NoError(t, eng.Link(context.TODO(), "b", "c", "calls", "", 1, nil))
		require.NoError(t, eng.Link(context.TODO(), "a", "d", "imports", "", 1, nil))

		page, err := eng.SubgraphPage(context.Background(), GraphPageQuery{
			GraphQuery: GraphQuery{
				RootIDs:   []string{"a"},
				EdgeKinds: []EdgeKind{"calls"},
				MaxDepth:  3,
				Direction: DirectionOut,
				Limit:     10000,
			},
			PageSize: 10000,
		})
		require.NoError(t, err)
		var affected []string
		for _, elem := range page.Items {
			if elem.Node.ID != "" && elem.Node.ID != "a" {
				affected = append(affected, elem.Node.ID)
			}
		}
		require.ElementsMatch(t, []string{"b", "c"}, affected)
	})

	t.Run("impact_set_empty_origin", func(t *testing.T) {
		eng, _ := factory(t)
		page, err := eng.SubgraphPage(context.Background(), GraphPageQuery{
			GraphQuery: GraphQuery{
				RootIDs:   []string{},
				EdgeKinds: []EdgeKind{"calls"},
				MaxDepth:  2,
				Direction: DirectionOut,
				Limit:     10000,
			},
			PageSize: 10000,
		})
		require.NoError(t, err)
		require.Empty(t, page.Items)
	})

	t.Run("neighbors", func(t *testing.T) {
		eng, _ := factory(t)
		for _, id := range []string{"a", "b", "c"} {
			require.NoError(t, eng.UpsertNode(context.TODO(), NodeRecord{ID: id, Kind: "function"}))
		}
		require.NoError(t, eng.Link(context.TODO(), "a", "b", "calls", "", 1, nil))
		require.NoError(t, eng.Link(context.TODO(), "a", "c", "calls", "", 1, nil))

		n := eng.Neighbors("a", DirectionOut)
		require.ElementsMatch(t, []string{"b", "c"}, n)
	})

	t.Run("neighbors_direction_in", func(t *testing.T) {
		eng, _ := factory(t)
		for _, id := range []string{"a", "b", "c"} {
			require.NoError(t, eng.UpsertNode(context.TODO(), NodeRecord{ID: id, Kind: "function"}))
		}
		require.NoError(t, eng.Link(context.TODO(), "a", "b", "calls", "", 1, nil))
		require.NoError(t, eng.Link(context.TODO(), "c", "b", "calls", "", 1, nil))

		n := eng.Neighbors("b", DirectionIn)
		require.ElementsMatch(t, []string{"a", "c"}, n)
	})

	t.Run("subgraph", func(t *testing.T) {
		eng, _ := factory(t)
		for _, id := range []string{"a", "b", "c"} {
			require.NoError(t, eng.UpsertNode(context.TODO(), NodeRecord{ID: id, Kind: "function"}))
		}
		require.NoError(t, eng.Link(context.TODO(), "a", "b", "calls", "", 1, nil))
		require.NoError(t, eng.Link(context.TODO(), "b", "c", "calls", "", 1, nil))

		page, err := eng.SubgraphPage(context.Background(), GraphPageQuery{
			GraphQuery: GraphQuery{
				RootIDs:   []string{"a"},
				Direction: DirectionOut,
				MaxDepth:  2,
				Limit:     10000,
			},
			PageSize: 10000,
		})
		require.NoError(t, err)
		var nodes []NodeRecord
		var edges []EdgeRecord
		for _, elem := range page.Items {
			if elem.Node.ID != "" {
				nodes = append(nodes, elem.Node)
			}
			if elem.Edge.SourceID != "" {
				edges = append(edges, elem.Edge)
			}
		}
		require.Len(t, nodes, 3)
		require.Len(t, edges, 2)
	})

	t.Run("subgraph_depth_zero", func(t *testing.T) {
		eng, _ := factory(t)
		require.NoError(t, eng.UpsertNode(context.TODO(), NodeRecord{ID: "root", Kind: "function"}))
		require.NoError(t, eng.LinkEdges(context.TODO(), []EdgeRecord{
			{SourceID: "root", TargetID: "leaf", Kind: "calls"},
		}))

		page, err := eng.SubgraphPage(context.Background(), GraphPageQuery{
			GraphQuery: GraphQuery{
				RootIDs:   []string{"root"},
				Direction: DirectionOut,
				MaxDepth:  0,
				Limit:     10000,
			},
			PageSize: 10000,
		})
		require.NoError(t, err)
		var nodes []NodeRecord
		for _, elem := range page.Items {
			if elem.Node.ID != "" {
				nodes = append(nodes, elem.Node)
			}
		}
		require.Len(t, nodes, 1)
		require.Equal(t, "root", nodes[0].ID)
	})

	t.Run("subgraph_direction_both", func(t *testing.T) {
		eng, _ := factory(t)
		for _, id := range []string{"a", "b", "c"} {
			require.NoError(t, eng.UpsertNode(context.TODO(), NodeRecord{ID: id, Kind: "function"}))
		}
		require.NoError(t, eng.Link(context.TODO(), "a", "b", "calls", "", 1, nil))
		require.NoError(t, eng.Link(context.TODO(), "c", "b", "calls", "", 1, nil))

		page, err := eng.SubgraphPage(context.Background(), GraphPageQuery{
			GraphQuery: GraphQuery{
				RootIDs:   []string{"b"},
				Direction: DirectionBoth,
				MaxDepth:  1,
				EdgeKinds: []EdgeKind{"calls"},
				Limit:     10000,
			},
			PageSize: 10000,
		})
		require.NoError(t, err)
		var nodes []NodeRecord
		var edges []EdgeRecord
		for _, elem := range page.Items {
			if elem.Node.ID != "" {
				nodes = append(nodes, elem.Node)
			}
			if elem.Edge.SourceID != "" {
				edges = append(edges, elem.Edge)
			}
		}
		require.Len(t, nodes, 3)
		require.Len(t, edges, 2)
	})

	t.Run("batch_upsert_nodes", func(t *testing.T) {
		eng, _ := factory(t)
		require.NoError(t, eng.UpsertNodes(context.TODO(), []NodeRecord{
			{ID: "a", Kind: "function", SourceID: "src.go"},
			{ID: "b", Kind: "function", SourceID: "src.go"},
			{ID: "c", Kind: "method", SourceID: "src.go"},
		}))
		require.Len(t, eng.ListNodes("function"), 2)
		require.Len(t, eng.ListNodes("method"), 1)
	})

	t.Run("batch_link_edges", func(t *testing.T) {
		eng, _ := factory(t)
		require.NoError(t, eng.UpsertNodes(context.TODO(), []NodeRecord{
			{ID: "a", Kind: "function"},
			{ID: "b", Kind: "function"},
			{ID: "c", Kind: "function"},
		}))
		require.NoError(t, eng.LinkEdges(context.TODO(), []EdgeRecord{
			{SourceID: "a", TargetID: "b", Kind: "calls", Weight: 1},
			{SourceID: "b", TargetID: "c", Kind: "calls", Weight: 1},
		}))
		require.Len(t, eng.GetOutEdges("a"), 1)
		require.Len(t, eng.GetOutEdges("b"), 1)
		require.Len(t, eng.GetInEdges("c"), 1)
	})

	t.Run("soft_unlink_edge", func(t *testing.T) {
		eng, _ := factory(t)
		require.NoError(t, eng.UpsertNode(context.TODO(), NodeRecord{ID: "a", Kind: "function"}))
		require.NoError(t, eng.UpsertNode(context.TODO(), NodeRecord{ID: "b", Kind: "function"}))
		require.NoError(t, eng.Link(context.TODO(), "a", "b", "calls", "", 1, nil))
		require.NoError(t, eng.Unlink(context.TODO(), "a", "b", "calls", false))

		out := eng.GetOutEdges("a")
		require.Empty(t, out)

		raw := allOutEdges(t, eng, "a")
		require.Len(t, raw, 1)
		require.False(t, raw[0].IsActive())
	})

	t.Run("hard_unlink_edge", func(t *testing.T) {
		eng, _ := factory(t)
		require.NoError(t, eng.UpsertNode(context.TODO(), NodeRecord{ID: "x", Kind: "function"}))
		require.NoError(t, eng.UpsertNode(context.TODO(), NodeRecord{ID: "y", Kind: "function"}))
		require.NoError(t, eng.Link(context.TODO(), "x", "y", "imports", "", 1, nil))
		require.NoError(t, eng.Unlink(context.TODO(), "x", "y", "imports", true))

		raw := allOutEdges(t, eng, "x")
		require.Empty(t, raw)
	})

	t.Run("annotate_node", func(t *testing.T) {
		eng, _ := factory(t)
		require.NoError(t, eng.UpsertNode(context.TODO(), NodeRecord{ID: "n", Kind: "function", Props: json.RawMessage(`{"a":1}`)}))
		require.NoError(t, eng.AnnotateNode(context.TODO(), "n", map[string]any{"b": 2}))

		node, ok := eng.GetNode("n")
		require.True(t, ok)
		require.JSONEq(t, `{"a":1,"b":2}`, string(node.Props))
	})

	t.Run("edge_kind_filter", func(t *testing.T) {
		eng, _ := factory(t)
		require.NoError(t, eng.UpsertNode(context.TODO(), NodeRecord{ID: "a", Kind: "function"}))
		require.NoError(t, eng.UpsertNode(context.TODO(), NodeRecord{ID: "b", Kind: "function"}))
		require.NoError(t, eng.Link(context.TODO(), "a", "b", "calls", "", 1, nil))
		require.NoError(t, eng.Link(context.TODO(), "a", "b", "imports", "", 1, nil))

		outCalls := eng.GetOutEdges("a", "calls")
		require.Len(t, outCalls, 1)
		require.Equal(t, EdgeKind("calls"), outCalls[0].Kind)

		outAll := eng.GetOutEdges("a")
		require.Len(t, outAll, 2)
	})

	t.Run("delete_then_reopen", func(t *testing.T) {
		eng, reopen := factory(t)
		require.NoError(t, eng.UpsertNode(context.TODO(), NodeRecord{ID: "n1", Kind: "function"}))
		require.NoError(t, eng.UpsertNode(context.TODO(), NodeRecord{ID: "n2", Kind: "function"}))
		require.NoError(t, eng.Link(context.TODO(), "n1", "n2", "calls", "", 1, nil))
		require.NoError(t, eng.DeleteNode(context.TODO(), "n1"))
		require.NoError(t, eng.Close(context.Background()))

		eng2 := reopen(t)
		_, ok := eng2.GetNode("n1")
		require.False(t, ok)

		_, ok = eng2.GetNode("n2")
		require.True(t, ok)

		out := allOutEdges(t, eng2, "n1")
		require.Len(t, out, 1)
		require.False(t, out[0].IsActive())
	})

	t.Run("snapshot_and_recover", func(t *testing.T) {
		eng, reopen := factory(t)
		require.NoError(t, eng.UpsertNode(context.TODO(), NodeRecord{ID: "s1", Kind: "function"}))
		require.NoError(t, eng.UpsertNode(context.TODO(), NodeRecord{ID: "s2", Kind: "function"}))
		require.NoError(t, eng.Link(context.TODO(), "s1", "s2", "calls", "", 1, nil))
		require.NoError(t, eng.Snapshot(context.Background()))
		require.NoError(t, eng.Close(context.Background()))

		eng2 := reopen(t)
		_, ok := eng2.GetNode("s1")
		require.True(t, ok)
		_, ok = eng2.GetNode("s2")
		require.True(t, ok)
		require.Len(t, eng2.GetOutEdges("s1"), 1)
	})
}

// ────────────────────────────────────────────────────────────────────
// Backend‑specific entry points
// ────────────────────────────────────────────────────────────────────

func TestBackendConformance(t *testing.T) {
	runBadgerConformance(t)
}

func runBadgerConformance(t *testing.T) {
	// node_upsert_and_get
	t.Run("node_upsert_and_get", func(t *testing.T) {
		eng := newBadgerEngine(t)
		require.NoError(t, eng.UpsertNode(context.TODO(), NodeRecord{ID: "n1", Kind: "function", SourceID: "a.go"}))
		node, ok := eng.GetNode("n1")
		require.True(t, ok)
		require.Equal(t, "n1", node.ID)
		require.Equal(t, NodeKind("function"), node.Kind)
		require.Equal(t, "a.go", node.SourceID)
		require.NotZero(t, node.CreatedAt)
		require.NotZero(t, node.UpdatedAt)
		require.Zero(t, node.DeletedAt)
	})

	// node_upsert_update_existing
	t.Run("node_upsert_update_existing", func(t *testing.T) {
		eng := newBadgerEngine(t)
		require.NoError(t, eng.UpsertNode(context.TODO(), NodeRecord{ID: "n", Kind: "function", SourceID: "old.go"}))
		require.NoError(t, eng.UpsertNode(context.TODO(), NodeRecord{ID: "n", Kind: "method", SourceID: "new.go"}))
		node, ok := eng.GetNode("n")
		require.True(t, ok)
		require.Equal(t, NodeKind("method"), node.Kind)
		require.Equal(t, "new.go", node.SourceID)
		require.NotZero(t, node.CreatedAt)
		require.NotZero(t, node.UpdatedAt)
	})

	// node_upsert_and_reopen
	t.Run("node_upsert_and_reopen", func(t *testing.T) {
		dir := t.TempDir()
		eng := newBadgerEngineAt(t, dir)
		require.NoError(t, eng.UpsertNode(context.TODO(), NodeRecord{ID: "persist", Kind: "function", SourceID: "x.go"}))
		require.NoError(t, eng.Close(context.Background()))
		eng2 := newBadgerEngineAt(t, dir)
		node, ok := eng2.GetNode("persist")
		require.True(t, ok)
		require.Equal(t, "x.go", node.SourceID)
	})

	// edge_link_and_get
	t.Run("edge_link_and_get", func(t *testing.T) {
		eng := newBadgerEngine(t)
		require.NoError(t, eng.UpsertNode(context.TODO(), NodeRecord{ID: "a", Kind: "function"}))
		require.NoError(t, eng.UpsertNode(context.TODO(), NodeRecord{ID: "b", Kind: "function"}))
		require.NoError(t, eng.Link(context.TODO(), "a", "b", "calls", "", 1, map[string]any{"line": 42}))
		out := eng.GetOutEdges("a", "calls")
		require.Len(t, out, 1)
		require.Equal(t, "b", out[0].TargetID)
		require.Equal(t, EdgeKind("calls"), out[0].Kind)
		require.JSONEq(t, `{"line":42}`, string(out[0].Props))
		in := eng.GetInEdges("b", "calls")
		require.Len(t, in, 1)
		require.Equal(t, "a", in[0].SourceID)
	})

	// edge_link_and_reopen
	t.Run("edge_link_and_reopen", func(t *testing.T) {
		dir := t.TempDir()
		eng := newBadgerEngineAt(t, dir)
		require.NoError(t, eng.UpsertNode(context.TODO(), NodeRecord{ID: "x", Kind: "function"}))
		require.NoError(t, eng.UpsertNode(context.TODO(), NodeRecord{ID: "y", Kind: "function"}))
		require.NoError(t, eng.Link(context.TODO(), "x", "y", "depends", "", 1, nil))
		require.NoError(t, eng.Close(context.Background()))
		eng2 := newBadgerEngineAt(t, dir)
		out := eng2.GetOutEdges("x", "depends")
		require.Len(t, out, 1)
		require.Equal(t, "y", out[0].TargetID)
		in := eng2.GetInEdges("y", "depends")
		require.Len(t, in, 1)
		require.Equal(t, "x", in[0].SourceID)
	})

	// edge_link_with_inverse
	t.Run("edge_link_with_inverse", func(t *testing.T) {
		eng := newBadgerEngine(t)
		require.NoError(t, eng.UpsertNode(context.TODO(), NodeRecord{ID: "a", Kind: "function"}))
		require.NoError(t, eng.UpsertNode(context.TODO(), NodeRecord{ID: "b", Kind: "function"}))
		require.NoError(t, eng.Link(context.TODO(), "a", "b", "calls", "called_by", 1, nil))
		outA := eng.GetOutEdges("a", "calls")
		require.Len(t, outA, 1)
		outB := eng.GetOutEdges("b", "called_by")
		require.Len(t, outB, 1)
		require.Equal(t, "a", outB[0].TargetID)
	})

	// soft_delete_node_soft_deletes_edges
	t.Run("soft_delete_node_soft_deletes_edges", func(t *testing.T) {
		eng := newBadgerEngine(t)
		for _, id := range []string{"a", "b", "c"} {
			require.NoError(t, eng.UpsertNode(context.TODO(), NodeRecord{ID: id, Kind: "function"}))
		}
		require.NoError(t, eng.Link(context.TODO(), "a", "b", "calls", "", 1, nil))
		require.NoError(t, eng.Link(context.TODO(), "c", "b", "calls", "", 1, nil))
		require.NoError(t, eng.DeleteNode(context.TODO(), "b"))
		_, ok := eng.GetNode("b")
		require.False(t, ok)
		out := allOutEdges(t, eng, "a")
		require.Len(t, out, 1)
		require.False(t, out[0].IsActive())
		in := allInEdges(t, eng, "b")
		require.Len(t, in, 2)
		for _, e := range in {
			require.False(t, e.IsActive())
		}
	})

	// soft_delete_node_reopen
	t.Run("soft_delete_node_reopen", func(t *testing.T) {
		dir := t.TempDir()
		eng := newBadgerEngineAt(t, dir)
		require.NoError(t, eng.UpsertNode(context.TODO(), NodeRecord{ID: "del", Kind: "function"}))
		require.NoError(t, eng.UpsertNode(context.TODO(), NodeRecord{ID: "keep", Kind: "function"}))
		require.NoError(t, eng.Link(context.TODO(), "del", "keep", "calls", "", 1, nil))
		require.NoError(t, eng.DeleteNode(context.TODO(), "del"))
		require.NoError(t, eng.Close(context.Background()))
		eng2 := newBadgerEngineAt(t, dir)
		_, ok := eng2.GetNode("del")
		require.False(t, ok)
		_, ok = eng2.GetNode("keep")
		require.True(t, ok)
		out := allOutEdges(t, eng2, "del")
		require.Len(t, out, 1)
		require.False(t, out[0].IsActive())
	})

	// label_lookup
	t.Run("label_lookup", func(t *testing.T) {
		eng := newBadgerEngine(t)
		require.NoError(t, eng.UpsertNode(context.TODO(), NodeRecord{ID: "n1", Kind: "function", Labels: []string{"tag:a", "tag:b"}}))
		require.NoError(t, eng.UpsertNode(context.TODO(), NodeRecord{ID: "n2", Kind: "method", Labels: []string{"tag:a"}}))
		require.NoError(t, eng.UpsertNode(context.TODO(), NodeRecord{ID: "n3", Kind: "function", Labels: []string{"tag:c"}}))
		nodes := eng.ListNodesByLabel("", "tag:a")
		require.Len(t, nodes, 2)
		nodes = eng.ListNodesByLabel("function", "tag:a")
		require.Len(t, nodes, 1)
		require.Equal(t, "n1", nodes[0].ID)
	})

	// label_prefix_lookup
	t.Run("label_prefix_lookup", func(t *testing.T) {
		eng := newBadgerEngine(t)
		require.NoError(t, eng.UpsertNode(context.TODO(), NodeRecord{ID: "n1", Kind: "file", Labels: []string{"path:/src/main.go"}}))
		require.NoError(t, eng.UpsertNode(context.TODO(), NodeRecord{ID: "n2", Kind: "file", Labels: []string{"path:/src/util.go"}}))
		nodes := eng.ListNodesByLabelPrefix("", "path:/src")
		require.Len(t, nodes, 2)
		nodes = eng.ListNodesByLabelPrefix("", "path:/src/main")
		require.Len(t, nodes, 1)
		require.Equal(t, "n1", nodes[0].ID)
	})

	// label_lookup_reopen
	t.Run("label_lookup_reopen", func(t *testing.T) {
		dir := t.TempDir()
		eng := newBadgerEngineAt(t, dir)
		require.NoError(t, eng.UpsertNode(context.TODO(), NodeRecord{ID: "keep", Kind: "file", Labels: []string{"tag:persist"}}))
		require.NoError(t, eng.UpsertNode(context.TODO(), NodeRecord{ID: "gone", Kind: "file", Labels: []string{"tag:removed"}}))
		require.NoError(t, eng.DeleteNode(context.TODO(), "gone"))
		require.NoError(t, eng.Close(context.Background()))
		eng2 := newBadgerEngineAt(t, dir)
		nodes := eng2.ListNodesByLabel("", "tag:persist")
		require.Len(t, nodes, 1)
		require.Equal(t, "keep", nodes[0].ID)
		nodes = eng2.ListNodesByLabel("", "tag:removed")
		require.Empty(t, nodes)
	})

	// nodes_by_source
	t.Run("nodes_by_source", func(t *testing.T) {
		eng := newBadgerEngine(t)
		require.NoError(t, eng.UpsertNode(context.TODO(), NodeRecord{ID: "a", Kind: "function", SourceID: "src.go"}))
		require.NoError(t, eng.UpsertNode(context.TODO(), NodeRecord{ID: "b", Kind: "method", SourceID: "src.go"}))
		require.NoError(t, eng.UpsertNode(context.TODO(), NodeRecord{ID: "c", Kind: "function", SourceID: "other.go"}))
		nodes := eng.NodesBySource("src.go")
		require.Len(t, nodes, 2)
	})

	// nodes_by_source_reopen
	t.Run("nodes_by_source_reopen", func(t *testing.T) {
		dir := t.TempDir()
		eng := newBadgerEngineAt(t, dir)
		require.NoError(t, eng.UpsertNode(context.TODO(), NodeRecord{ID: "x", Kind: "function", SourceID: "work.go"}))
		require.NoError(t, eng.UpsertNode(context.TODO(), NodeRecord{ID: "y", Kind: "function", SourceID: "work.go"}))
		require.NoError(t, eng.Close(context.Background()))
		eng2 := newBadgerEngineAt(t, dir)
		nodes := eng2.NodesBySource("work.go")
		require.Len(t, nodes, 2)
	})

	// mutation_result_persistence
	t.Run("mutation_result_persistence", func(t *testing.T) {
		dir := t.TempDir()
		eng := newBadgerEngineAt(t, dir)
		result := MutationResult{
			Scope: MutationScopeNode, Status: MutationStatusCreated, Reason: "test",
			TaskID: "task-1", SessionID: "session-1", StateVersion: 1,
			Details: map[string]any{"key": "val"},
		}
		result.Normalize(result.TaskID, result.SessionID)
		stableID := result.StableID
		require.NotEmpty(t, stableID)
		require.NoError(t, eng.RecordMutationResult(context.TODO(), result))
		got, ok := eng.MutationResult(stableID)
		require.True(t, ok)
		require.Equal(t, stableID, got.StableID)
		require.Equal(t, MutationScopeNode, got.Scope)
		require.Equal(t, "val", got.Details["key"])
		require.NotEmpty(t, got.AppliedAt)
		require.NoError(t, eng.Close(context.Background()))
		eng2 := newBadgerEngineAt(t, dir)
		got2, ok := eng2.MutationResult(stableID)
		require.True(t, ok)
		require.Equal(t, stableID, got2.StableID)
	})

	// mutation_result_multiple
	t.Run("mutation_result_multiple", func(t *testing.T) {
		eng := newBadgerEngine(t)
		r1 := MutationResult{Scope: MutationScopeNode, Status: MutationStatusCreated, TaskID: "t1", SessionID: "s1"}
		r1.Normalize(r1.TaskID, r1.SessionID)
		require.NoError(t, eng.RecordMutationResult(context.TODO(), r1))
		r2 := MutationResult{Scope: MutationScopeEdge, Status: MutationStatusUpdated, TaskID: "t1", SessionID: "s1"}
		r2.Normalize(r2.TaskID, r2.SessionID)
		require.NoError(t, eng.RecordMutationResult(context.TODO(), r2))
		results := eng.MutationResults()
		require.Len(t, results, 2)
		require.Equal(t, r1.StableID, results[0].StableID)
		require.Equal(t, r2.StableID, results[1].StableID)
	})

	// node_revision_history
	t.Run("node_revision_history", func(t *testing.T) {
		eng := newBadgerEngine(t)
		require.NoError(t, eng.UpsertNode(context.TODO(), NodeRecord{ID: "n", Kind: "function", Props: []byte(`{"v":1}`)}))
		require.NoError(t, eng.AnnotateNode(context.TODO(), "n", map[string]any{"note": "a"}))
		require.NoError(t, eng.UpsertNode(context.TODO(), NodeRecord{ID: "n", Kind: "function", Props: []byte(`{"v":2}`)}))
		revs := eng.NodeRevisions("n")
		require.Len(t, revs, 2)
		require.JSONEq(t, `{"v":1}`, string(revs[0].Props))
		require.JSONEq(t, `{"v":1,"note":"a"}`, string(revs[1].Props))
	})

	// node_revision_history_reopen
	t.Run("node_revision_history_reopen", func(t *testing.T) {
		dir := t.TempDir()
		eng := newBadgerEngineAt(t, dir)
		require.NoError(t, eng.UpsertNode(context.TODO(), NodeRecord{ID: "n", Kind: "function", Props: []byte(`{"v":1}`)}))
		require.NoError(t, eng.AnnotateNode(context.TODO(), "n", map[string]any{"note": "x"}))
		require.NoError(t, eng.Close(context.Background()))
		eng2 := newBadgerEngineAt(t, dir)
		revs := eng2.NodeRevisions("n")
		require.Len(t, revs, 1)
		require.JSONEq(t, `{"v":1}`, string(revs[0].Props))
	})

	// edge_revision_history
	t.Run("edge_revision_history", func(t *testing.T) {
		eng := newBadgerEngine(t)
		require.NoError(t, eng.UpsertNode(context.TODO(), NodeRecord{ID: "a", Kind: "function"}))
		require.NoError(t, eng.UpsertNode(context.TODO(), NodeRecord{ID: "b", Kind: "function"}))
		require.NoError(t, eng.Link(context.TODO(), "a", "b", "calls", "", 1, map[string]any{"v": 1}))
		require.NoError(t, eng.AnnotateEdge(context.TODO(), "a", "b", "calls", map[string]any{"note": "x"}))
		require.NoError(t, eng.LinkEdges(context.TODO(), []EdgeRecord{
			{SourceID: "a", TargetID: "b", Kind: "calls", Weight: 2, Props: []byte(`{"v":2}`)},
		}))
		revs := eng.EdgeRevisions("a", "b", "calls")
		require.Len(t, revs, 2)
		require.JSONEq(t, `{"v":1}`, string(revs[0].Props))
		require.JSONEq(t, `{"note":"x","v":1}`, string(revs[1].Props))
	})

	// impact_set
	t.Run("impact_set", func(t *testing.T) {
		eng := newBadgerEngine(t)
		for _, id := range []string{"a", "b", "c", "d"} {
			require.NoError(t, eng.UpsertNode(context.TODO(), NodeRecord{ID: id, Kind: "function"}))
		}
		require.NoError(t, eng.Link(context.TODO(), "a", "b", "calls", "", 1, nil))
		require.NoError(t, eng.Link(context.TODO(), "b", "c", "calls", "", 1, nil))
		require.NoError(t, eng.Link(context.TODO(), "a", "d", "imports", "", 1, nil))
		page, err := eng.SubgraphPage(context.Background(), GraphPageQuery{
			GraphQuery: GraphQuery{
				RootIDs:   []string{"a"},
				EdgeKinds: []EdgeKind{"calls"},
				MaxDepth:  3,
				Direction: DirectionOut,
				Limit:     10000,
			},
			PageSize: 10000,
		})
		require.NoError(t, err)
		var affected []string
		for _, elem := range page.Items {
			if elem.Node.ID != "" && elem.Node.ID != "a" {
				affected = append(affected, elem.Node.ID)
			}
		}
		require.ElementsMatch(t, []string{"b", "c"}, affected)
	})

	// impact_set_empty_origin
	t.Run("impact_set_empty_origin", func(t *testing.T) {
		eng := newBadgerEngine(t)
		page, err := eng.SubgraphPage(context.Background(), GraphPageQuery{
			GraphQuery: GraphQuery{
				RootIDs:   []string{},
				EdgeKinds: []EdgeKind{"calls"},
				MaxDepth:  2,
				Direction: DirectionOut,
				Limit:     10000,
			},
			PageSize: 10000,
		})
		require.NoError(t, err)
		require.Empty(t, page.Items)
	})

	// neighbors
	t.Run("neighbors", func(t *testing.T) {
		eng := newBadgerEngine(t)
		for _, id := range []string{"a", "b", "c"} {
			require.NoError(t, eng.UpsertNode(context.TODO(), NodeRecord{ID: id, Kind: "function"}))
		}
		require.NoError(t, eng.Link(context.TODO(), "a", "b", "calls", "", 1, nil))
		require.NoError(t, eng.Link(context.TODO(), "a", "c", "calls", "", 1, nil))
		n := eng.Neighbors("a", DirectionOut)
		require.ElementsMatch(t, []string{"b", "c"}, n)
	})

	// neighbors_direction_in
	t.Run("neighbors_direction_in", func(t *testing.T) {
		eng := newBadgerEngine(t)
		for _, id := range []string{"a", "b", "c"} {
			require.NoError(t, eng.UpsertNode(context.TODO(), NodeRecord{ID: id, Kind: "function"}))
		}
		require.NoError(t, eng.Link(context.TODO(), "a", "b", "calls", "", 1, nil))
		require.NoError(t, eng.Link(context.TODO(), "c", "b", "calls", "", 1, nil))
		n := eng.Neighbors("b", DirectionIn)
		require.ElementsMatch(t, []string{"a", "c"}, n)
	})

	// subgraph
	t.Run("subgraph", func(t *testing.T) {
		eng := newBadgerEngine(t)
		for _, id := range []string{"a", "b", "c"} {
			require.NoError(t, eng.UpsertNode(context.TODO(), NodeRecord{ID: id, Kind: "function"}))
		}
		require.NoError(t, eng.Link(context.TODO(), "a", "b", "calls", "", 1, nil))
		require.NoError(t, eng.Link(context.TODO(), "b", "c", "calls", "", 1, nil))
		page, err := eng.SubgraphPage(context.Background(), GraphPageQuery{
			GraphQuery: GraphQuery{
				RootIDs:   []string{"a"},
				Direction: DirectionOut,
				MaxDepth:  2,
				Limit:     10000,
			},
			PageSize: 10000,
		})
		require.NoError(t, err)
		var nodes []NodeRecord
		var edges []EdgeRecord
		for _, elem := range page.Items {
			if elem.Node.ID != "" {
				nodes = append(nodes, elem.Node)
			}
			if elem.Edge.SourceID != "" {
				edges = append(edges, elem.Edge)
			}
		}
		require.Len(t, nodes, 3)
		require.Len(t, edges, 2)
	})

	// subgraph_depth_zero
	t.Run("subgraph_depth_zero", func(t *testing.T) {
		eng := newBadgerEngine(t)
		require.NoError(t, eng.UpsertNode(context.TODO(), NodeRecord{ID: "root", Kind: "function"}))
		require.NoError(t, eng.LinkEdges(context.TODO(), []EdgeRecord{
			{SourceID: "root", TargetID: "leaf", Kind: "calls"},
		}))
		page, err := eng.SubgraphPage(context.Background(), GraphPageQuery{
			GraphQuery: GraphQuery{
				RootIDs:   []string{"root"},
				Direction: DirectionOut,
				MaxDepth:  0,
				Limit:     10000,
			},
			PageSize: 10000,
		})
		require.NoError(t, err)
		var nodes []NodeRecord
		for _, elem := range page.Items {
			if elem.Node.ID != "" {
				nodes = append(nodes, elem.Node)
			}
		}
		require.Len(t, nodes, 1)
		require.Equal(t, "root", nodes[0].ID)
	})

	// subgraph_direction_both
	t.Run("subgraph_direction_both", func(t *testing.T) {
		eng := newBadgerEngine(t)
		for _, id := range []string{"a", "b", "c"} {
			require.NoError(t, eng.UpsertNode(context.TODO(), NodeRecord{ID: id, Kind: "function"}))
		}
		require.NoError(t, eng.Link(context.TODO(), "a", "b", "calls", "", 1, nil))
		require.NoError(t, eng.Link(context.TODO(), "c", "b", "calls", "", 1, nil))
		page, err := eng.SubgraphPage(context.Background(), GraphPageQuery{
			GraphQuery: GraphQuery{
				RootIDs:   []string{"b"},
				Direction: DirectionBoth,
				MaxDepth:  1,
				EdgeKinds: []EdgeKind{"calls"},
				Limit:     10000,
			},
			PageSize: 10000,
		})
		require.NoError(t, err)
		var nodes []NodeRecord
		var edges []EdgeRecord
		for _, elem := range page.Items {
			if elem.Node.ID != "" {
				nodes = append(nodes, elem.Node)
			}
			if elem.Edge.SourceID != "" {
				edges = append(edges, elem.Edge)
			}
		}
		require.Len(t, nodes, 3)
		require.Len(t, edges, 2)
	})

	// batch_upsert_nodes
	t.Run("batch_upsert_nodes", func(t *testing.T) {
		eng := newBadgerEngine(t)
		require.NoError(t, eng.UpsertNodes(context.TODO(), []NodeRecord{
			{ID: "a", Kind: "function", SourceID: "src.go"},
			{ID: "b", Kind: "function", SourceID: "src.go"},
			{ID: "c", Kind: "method", SourceID: "src.go"},
		}))
		require.Len(t, eng.ListNodes("function"), 2)
		require.Len(t, eng.ListNodes("method"), 1)
	})

	// batch_link_edges
	t.Run("batch_link_edges", func(t *testing.T) {
		eng := newBadgerEngine(t)
		require.NoError(t, eng.UpsertNodes(context.TODO(), []NodeRecord{
			{ID: "a", Kind: "function"}, {ID: "b", Kind: "function"}, {ID: "c", Kind: "function"},
		}))
		require.NoError(t, eng.LinkEdges(context.TODO(), []EdgeRecord{
			{SourceID: "a", TargetID: "b", Kind: "calls", Weight: 1},
			{SourceID: "b", TargetID: "c", Kind: "calls", Weight: 1},
		}))
		require.Len(t, eng.GetOutEdges("a"), 1)
		require.Len(t, eng.GetOutEdges("b"), 1)
		require.Len(t, eng.GetInEdges("c"), 1)
	})

	// soft_unlink_edge
	t.Run("soft_unlink_edge", func(t *testing.T) {
		eng := newBadgerEngine(t)
		require.NoError(t, eng.UpsertNode(context.TODO(), NodeRecord{ID: "a", Kind: "function"}))
		require.NoError(t, eng.UpsertNode(context.TODO(), NodeRecord{ID: "b", Kind: "function"}))
		require.NoError(t, eng.Link(context.TODO(), "a", "b", "calls", "", 1, nil))
		require.NoError(t, eng.Unlink(context.TODO(), "a", "b", "calls", false))
		out := eng.GetOutEdges("a")
		require.Empty(t, out)
		raw := allOutEdges(t, eng, "a")
		require.Len(t, raw, 1)
		require.False(t, raw[0].IsActive())
	})

	// hard_unlink_edge
	t.Run("hard_unlink_edge", func(t *testing.T) {
		eng := newBadgerEngine(t)
		require.NoError(t, eng.UpsertNode(context.TODO(), NodeRecord{ID: "x", Kind: "function"}))
		require.NoError(t, eng.UpsertNode(context.TODO(), NodeRecord{ID: "y", Kind: "function"}))
		require.NoError(t, eng.Link(context.TODO(), "x", "y", "imports", "", 1, nil))
		require.NoError(t, eng.Unlink(context.TODO(), "x", "y", "imports", true))
		raw := allOutEdges(t, eng, "x")
		require.Empty(t, raw)
	})

	// annotate_node
	t.Run("annotate_node", func(t *testing.T) {
		eng := newBadgerEngine(t)
		require.NoError(t, eng.UpsertNode(context.TODO(), NodeRecord{ID: "n", Kind: "function", Props: []byte(`{"a":1}`)}))
		require.NoError(t, eng.AnnotateNode(context.TODO(), "n", map[string]any{"b": 2}))
		node, ok := eng.GetNode("n")
		require.True(t, ok)
		require.JSONEq(t, `{"a":1,"b":2}`, string(node.Props))
	})

	// edge_kind_filter
	t.Run("edge_kind_filter", func(t *testing.T) {
		eng := newBadgerEngine(t)
		require.NoError(t, eng.UpsertNode(context.TODO(), NodeRecord{ID: "a", Kind: "function"}))
		require.NoError(t, eng.UpsertNode(context.TODO(), NodeRecord{ID: "b", Kind: "function"}))
		require.NoError(t, eng.Link(context.TODO(), "a", "b", "calls", "", 1, nil))
		require.NoError(t, eng.Link(context.TODO(), "a", "b", "imports", "", 1, nil))
		outCalls := eng.GetOutEdges("a", "calls")
		require.Len(t, outCalls, 1)
		outAll := eng.GetOutEdges("a")
		require.Len(t, outAll, 2)
	})

	// delete_then_reopen
	t.Run("delete_then_reopen", func(t *testing.T) {
		dir := t.TempDir()
		eng := newBadgerEngineAt(t, dir)
		require.NoError(t, eng.UpsertNode(context.TODO(), NodeRecord{ID: "n1", Kind: "function"}))
		require.NoError(t, eng.UpsertNode(context.TODO(), NodeRecord{ID: "n2", Kind: "function"}))
		require.NoError(t, eng.Link(context.TODO(), "n1", "n2", "calls", "", 1, nil))
		require.NoError(t, eng.DeleteNode(context.TODO(), "n1"))
		require.NoError(t, eng.Close(context.Background()))
		eng2 := newBadgerEngineAt(t, dir)
		_, ok := eng2.GetNode("n1")
		require.False(t, ok)
		_, ok = eng2.GetNode("n2")
		require.True(t, ok)
		out := allOutEdges(t, eng2, "n1")
		require.Len(t, out, 1)
		require.False(t, out[0].IsActive())
	})

	// snapshot_and_recover
	t.Run("snapshot_and_recover", func(t *testing.T) {
		dir := t.TempDir()
		eng := newBadgerEngineAt(t, dir)
		require.NoError(t, eng.UpsertNode(context.TODO(), NodeRecord{ID: "s1", Kind: "function"}))
		require.NoError(t, eng.UpsertNode(context.TODO(), NodeRecord{ID: "s2", Kind: "function"}))
		require.NoError(t, eng.Link(context.TODO(), "s1", "s2", "calls", "", 1, nil))
		require.NoError(t, eng.Snapshot(context.Background()))
		require.NoError(t, eng.Close(context.Background()))
		eng2 := newBadgerEngineAt(t, dir)
		_, ok := eng2.GetNode("s1")
		require.True(t, ok)
		_, ok = eng2.GetNode("s2")
		require.True(t, ok)
		require.Len(t, eng2.GetOutEdges("s1"), 1)
	})
}

// newBadgerEngine creates a temporary-dir Badger engine (no reopen support).
func newBadgerEngine(t *testing.T) *Engine {
	t.Helper()
	return newBadgerEngineAt(t, t.TempDir())
}

// newBadgerEngineAt creates a Badger-backed Engine at the given directory.
// The engine is cleaned up when the test finishes.
func newBadgerEngineAt(t *testing.T, dir string) *Engine {
	t.Helper()
	bb, err := newBadgerBackend(BadgerOptions{Dir: dir})
	require.NoError(t, err)

	store := newAdjacencyStore()
	require.NoError(t, bb.load(context.Background(), store))

	eng := &Engine{
		opts: Options{
			AOFFileName: "dummy.aof", SnapshotFileName: "dummy.snap",
			SnapshotOnClose: false,
		},
		store:  store,
		bk:     bb,
		stopCh: make(chan struct{}),
	}
	eng.lastSave.Store(time.Now().UnixNano())
	eng.wg.Add(1)
	go eng.background(context.Background())

	t.Cleanup(func() {
		eng.stopOnce.Do(func() {
			close(eng.stopCh)
			eng.wg.Wait()
		})
		if eng.bk != nil {
			_ = eng.bk.close()
		}
	})
	return eng
}
