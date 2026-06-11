package graphdb

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"os"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNodeOperationsRoundTrip(t *testing.T) {
	engine, _ := newTestEngine(t)
	require.NoError(t, engine.UpsertNode(context.TODO(), NodeRecord{ID: "n1", Kind: "function", SourceID: "a.go", Labels: []string{"a"}}))
	require.NoError(t, engine.UpsertNode(context.TODO(), NodeRecord{ID: "n2", Kind: "method", SourceID: "a.go"}))
	require.NoError(t, engine.UpsertNode(context.TODO(), NodeRecord{ID: "n3", Kind: "function", SourceID: "b.go"}))

	node, ok := engine.GetNode("n1")
	require.True(t, ok)
	require.Equal(t, NodeKind("function"), node.Kind)
	require.Equal(t, "a.go", node.SourceID)

	byKind := engine.ListNodes("function")
	require.Len(t, byKind, 2)

	bySource := engine.NodesBySource("a.go")
	require.Len(t, bySource, 2)
}

func TestBatchNodeAndEdgeOperations(t *testing.T) {
	engine, _ := newTestEngine(t)
	require.NoError(t, engine.UpsertNodes(context.TODO(), []NodeRecord{
		{ID: "a", Kind: "function", SourceID: "a.go"},
		{ID: "b", Kind: "function", SourceID: "a.go"},
		{ID: "c", Kind: "function", SourceID: "b.go"},
	}))
	require.Len(t, engine.NodesBySource("a.go"), 2)

	require.NoError(t, engine.LinkEdges(context.TODO(), []EdgeRecord{
		{SourceID: "a", TargetID: "b", Kind: "calls", Weight: 1},
		{SourceID: "b", TargetID: "c", Kind: "calls", Weight: 1},
	}))
	require.Len(t, engine.GetOutEdges("a", "calls"), 1)
	require.Len(t, engine.GetOutEdges("b", "calls"), 1)

	require.NoError(t, engine.DeleteNodes(context.TODO(), []string{"a", "b"}))
	_, okA := engine.GetNode("a")
	_, okB := engine.GetNode("b")
	require.False(t, okA)
	require.False(t, okB)
}

func TestLinkAndUnlinkOperations(t *testing.T) {
	engine, _ := newTestEngine(t)
	require.NoError(t, engine.UpsertNode(context.TODO(), NodeRecord{ID: "a", Kind: "function"}))
	require.NoError(t, engine.UpsertNode(context.TODO(), NodeRecord{ID: "b", Kind: "function"}))

	require.NoError(t, engine.Link(context.TODO(), "a", "b", "calls", "called_by", 1, map[string]any{"site": "x"}))
	out := engine.GetOutEdges("a")
	in := engine.GetInEdges("b")
	require.Len(t, out, 1)
	require.Len(t, in, 1)
	require.Equal(t, EdgeKind("calls"), out[0].Kind)
	require.Equal(t, EdgeKind("calls"), in[0].Kind)
	require.Len(t, engine.GetOutEdges("b"), 1)
	require.Equal(t, EdgeKind("called_by"), engine.GetOutEdges("b")[0].Kind)

	require.NoError(t, engine.Unlink(context.TODO(), "a", "b", "calls", false))
	soft := allOutEdges(t, engine, "a")
	require.Len(t, soft, 1)
	require.False(t, soft[0].IsActive())
	require.Empty(t, engine.GetOutEdges("a"))

	require.NoError(t, engine.Unlink(context.TODO(), "b", "a", "called_by", true))
	require.Empty(t, allOutEdges(t, engine, "b"))
}

func TestDeleteNodeSoftDeletesConnectedEdges(t *testing.T) {
	engine, _ := newTestEngine(t)
	for _, id := range []string{"a", "b", "c"} {
		require.NoError(t, engine.UpsertNode(context.TODO(), NodeRecord{ID: id, Kind: "function"}))
	}
	require.NoError(t, engine.Link(context.TODO(), "a", "b", "calls", "", 1, nil))
	require.NoError(t, engine.Link(context.TODO(), "c", "b", "calls", "", 1, nil))

	require.NoError(t, engine.DeleteNode(context.TODO(), "b"))
	_, ok := engine.GetNode("b")
	require.False(t, ok)
	require.False(t, allOutEdges(t, engine, "a")[0].IsActive())
	require.False(t, allInEdges(t, engine, "b")[0].IsActive())
}

func TestDeleteAndReopen(t *testing.T) {
	engine, opts := newTestEngine(t)
	require.NoError(t, engine.UpsertNode(context.TODO(), NodeRecord{ID: "n1", Kind: "function"}))
	require.NoError(t, engine.UpsertNode(context.TODO(), NodeRecord{ID: "n2", Kind: "function"}))
	require.NoError(t, engine.Link(context.TODO(), "n1", "n2", "calls", "", 1, nil))
	require.NoError(t, engine.DeleteNode(context.TODO(), "n1"))
	require.NoError(t, engine.Close(context.Background()))

	reopened, err := Open(context.Background(), opts)
	require.NoError(t, err)
	defer reopened.Close(context.Background())

	_, ok := reopened.GetNode("n1")
	require.False(t, ok)
	out := allOutEdges(t, reopened, "n1")
	require.Len(t, out, 1)
	require.False(t, out[0].IsActive())
}

func TestMutationResultRoundTrip(t *testing.T) {
	engine, opts := newTestEngine(t)
	result := MutationResult{
		Scope:        MutationScopeProjection,
		Status:       MutationStatusCreated,
		Reason:       "projection pass completed",
		TaskID:       "task-1",
		SessionID:    "session-1",
		TurnID:       "turn-7",
		StateVersion: 3,
		Details:      map[string]any{"plan_id": "plan-1"},
	}
	result.Normalize(result.TaskID, result.SessionID)
	require.NoError(t, engine.RecordMutationResult(context.TODO(), result))

	got, ok := engine.MutationResult(result.StableID)
	require.True(t, ok)
	require.Equal(t, result.StableID, got.StableID)
	require.Equal(t, result.Scope, got.Scope)
	require.Equal(t, result.Status, got.Status)
	require.Equal(t, "plan-1", got.Details["plan_id"])
	require.NotEmpty(t, got.AppliedAt)

	results := engine.MutationResults()
	require.Len(t, results, 1)
	require.Equal(t, result.StableID, results[0].StableID)

	require.NoError(t, engine.Snapshot(context.Background()))
	require.NoError(t, engine.Close(context.Background()))

	reopened, err := Open(context.Background(), opts)
	require.NoError(t, err)
	defer reopened.Close(context.Background())

	reopenedResult, ok := reopened.MutationResult(result.StableID)
	require.True(t, ok)
	require.Equal(t, result.StableID, reopenedResult.StableID)
	require.Equal(t, result.Scope, reopenedResult.Scope)
	require.Equal(t, result.Status, reopenedResult.Status)
	require.Equal(t, "plan-1", reopenedResult.Details["plan_id"])
}

func TestImpactSetFindPathNeighborsAndSubgraph(t *testing.T) {
	engine, _ := newTestEngine(t)
	for _, id := range []string{"a", "b", "c", "d"} {
		require.NoError(t, engine.UpsertNode(context.TODO(), NodeRecord{ID: id, Kind: "function"}))
	}
	require.NoError(t, engine.Link(context.TODO(), "a", "b", "calls", "", 1, nil))
	require.NoError(t, engine.Link(context.TODO(), "b", "c", "calls", "", 1, nil))
	require.NoError(t, engine.Link(context.TODO(), "a", "d", "imports", "", 1, nil))

	// ImpactSet via SubgraphPage
	page, err := engine.SubgraphPage(context.Background(), GraphPageQuery{
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

	require.Equal(t, []string{"b", "d"}, engine.Neighbors("a", DirectionOut))

	// Subgraph via SubgraphPage
	page2, err := engine.SubgraphPage(context.Background(), GraphPageQuery{
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
	for _, elem := range page2.Items {
		if elem.Node.ID != "" {
			nodes = append(nodes, elem.Node)
		}
		if elem.Edge.SourceID != "" {
			edges = append(edges, elem.Edge)
		}
	}
	require.Len(t, nodes, 4)
	require.Len(t, edges, 3)
}

func TestConcurrentUpsertAndLink(t *testing.T) {
	engine, _ := newTestEngine(t)
	const workers = 12
	var wg sync.WaitGroup
	for i := range workers {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := string(rune('a' + i))
			require.NoError(t, engine.UpsertNode(context.TODO(), NodeRecord{ID: id, Kind: "function"}))
			if i > 0 {
				prev := string(rune('a' + i - 1))
				require.NoError(t, engine.Link(context.TODO(), prev, id, "calls", "", 1, nil))
			}
		}(i)
	}
	wg.Wait()
	require.Len(t, engine.ListNodes("function"), workers)
}

func TestStableMutationIdentityAndProvenanceRoundTrip(t *testing.T) {
	engine, _ := newTestEngine(t)
	nodeID := StableNodeID("task-1", "session-1", "turn-7", "node", "n1")
	edgeID := StableEdgeID("task-1", "session-1", "turn-7", "edge", "n1", "n2")
	node := NodeRecord{
		ID:             "n1",
		Kind:           "function",
		StableID:       nodeID,
		RevisionRootID: "root-1",
		RevisionOf:     "rev-0",
		IdempotencyKey: "idem-1",
		TaskID:         "task-1",
		SessionID:      "session-1",
		TurnID:         "turn-7",
		StateVersion:   11,
		Props:          json.RawMessage(`{"kind":"node"}`),
	}
	require.NoError(t, engine.UpsertNode(context.TODO(), node))
	require.NoError(t, engine.UpsertNode(context.TODO(), NodeRecord{ID: "n2", Kind: "function"}))
	require.NoError(t, engine.LinkEdges(context.TODO(), []EdgeRecord{{
		SourceID:       "n1",
		TargetID:       "n2",
		Kind:           "calls",
		StableID:       edgeID,
		RevisionRootID: "root-1",
		RevisionOf:     "rev-0",
		IdempotencyKey: "idem-edge-1",
		TaskID:         "task-1",
		SessionID:      "session-1",
		TurnID:         "turn-7",
		StateVersion:   11,
		Weight:         1,
		Props:          json.RawMessage(`{"source":"clarification"}`),
	}}))

	gotNode, ok := engine.GetNode("n1")
	require.True(t, ok)
	require.Equal(t, nodeID, gotNode.StableID)
	require.Equal(t, "root-1", gotNode.RevisionRootID)
	require.Equal(t, "rev-0", gotNode.RevisionOf)
	require.Equal(t, "idem-1", gotNode.IdempotencyKey)
	require.Equal(t, uint64(11), gotNode.StateVersion)

	out := engine.GetOutEdges("n1", "calls")
	require.Len(t, out, 1)
	require.Equal(t, edgeID, out[0].StableID)
	require.Equal(t, "root-1", out[0].RevisionRootID)
	require.Equal(t, "rev-0", out[0].RevisionOf)
	require.Equal(t, "idem-edge-1", out[0].IdempotencyKey)
	require.Equal(t, uint64(11), out[0].StateVersion)
}

func TestRevisionHistoryAndPersistence(t *testing.T) {
	engine, opts := newTestEngine(t)

	require.NoError(t, engine.UpsertNode(context.TODO(), NodeRecord{
		ID:    "n1",
		Kind:  "function",
		Props: json.RawMessage(`{"a":1}`),
	}))
	require.NoError(t, engine.AnnotateNode(context.TODO(), "n1", map[string]any{"b": 2}))
	require.NoError(t, engine.UpsertNode(context.TODO(), NodeRecord{
		ID:    "n1",
		Kind:  "function",
		Props: json.RawMessage(`{"c":3}`),
	}))

	nodeRevisions := engine.NodeRevisions("n1")
	require.Len(t, nodeRevisions, 2)
	require.JSONEq(t, `{"a":1}`, string(nodeRevisions[0].Props))
	require.JSONEq(t, `{"a":1,"b":2}`, string(nodeRevisions[1].Props))

	edgeProps := map[string]any{"site": "x"}
	require.NoError(t, engine.UpsertNode(context.TODO(), NodeRecord{ID: "n2", Kind: "function"}))
	require.NoError(t, engine.Link(context.TODO(), "n1", "n2", "calls", "", 1, edgeProps))
	require.NoError(t, engine.AnnotateEdge(context.TODO(), "n1", "n2", "calls", map[string]any{"note": "y"}))
	require.NoError(t, engine.LinkEdges(context.TODO(), []EdgeRecord{{
		SourceID: "n1",
		TargetID: "n2",
		Kind:     "calls",
		Weight:   2,
		Props:    json.RawMessage(`{"site":"z"}`),
	}}))

	edgeRevisions := engine.EdgeRevisions("n1", "n2", "calls")
	require.Len(t, edgeRevisions, 2)
	require.JSONEq(t, `{"site":"x"}`, string(edgeRevisions[0].Props))
	require.JSONEq(t, `{"note":"y","site":"x"}`, string(edgeRevisions[1].Props))
	require.Equal(t, float32(2), engine.GetOutEdges("n1", "calls")[0].Weight)
	require.JSONEq(t, `{"site":"z"}`, string(engine.GetOutEdges("n1", "calls")[0].Props))

	require.NoError(t, engine.Close(context.Background()))

	reopened, err := Open(context.Background(), opts)
	require.NoError(t, err)
	defer reopened.Close(context.Background())

	reopenedNodeRevisions := reopened.NodeRevisions("n1")
	require.Len(t, reopenedNodeRevisions, 2)
	require.JSONEq(t, `{"a":1}`, string(reopenedNodeRevisions[0].Props))
	require.JSONEq(t, `{"a":1,"b":2}`, string(reopenedNodeRevisions[1].Props))

	reopenedEdgeRevisions := reopened.EdgeRevisions("n1", "n2", "calls")
	require.Len(t, reopenedEdgeRevisions, 2)
	require.JSONEq(t, `{"site":"x"}`, string(reopenedEdgeRevisions[0].Props))
	require.JSONEq(t, `{"note":"y","site":"x"}`, string(reopenedEdgeRevisions[1].Props))
	require.JSONEq(t, `{"site":"z"}`, string(reopened.GetOutEdges("n1", "calls")[0].Props))
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	require.NoError(t, err)
	return data
}

func TestReadFrameRejectsCorruptChecksum(t *testing.T) {
	payload := []byte(`{"kind":"noop"}`)
	frame := encodeFrame(frameTypeOp, payload)
	binary.LittleEndian.PutUint32(frame[len(frame)-4:], 1)
	frameType, out, err := readFrame(bytesReader(frame))
	require.ErrorIs(t, err, errCorruptFrame)
	require.Zero(t, frameType)
	require.Nil(t, out)
}

type byteReader struct {
	data []byte
	pos  int
}

func bytesReader(data []byte) *byteReader {
	return &byteReader{data: data}
}

func (r *byteReader) Read(p []byte) (int, error) {
	if r.pos >= len(r.data) {
		return 0, os.ErrClosed
	}
	n := copy(p, r.data[r.pos:])
	r.pos += n
	if r.pos >= len(r.data) {
		return n, nil
	}
	return n, nil
}
