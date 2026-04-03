package graphdb

import (
	"encoding/binary"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)


func TestNodeOperationsRoundTrip(t *testing.T) {
	engine, _ := newTestEngine(t)
	require.NoError(t, engine.UpsertNode(NodeRecord{ID: "n1", Kind: "function", SourceID: "a.go", Labels: []string{"a"}}))
	require.NoError(t, engine.UpsertNode(NodeRecord{ID: "n2", Kind: "method", SourceID: "a.go"}))
	require.NoError(t, engine.UpsertNode(NodeRecord{ID: "n3", Kind: "function", SourceID: "b.go"}))

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
	require.NoError(t, engine.UpsertNodes([]NodeRecord{
		{ID: "a", Kind: "function", SourceID: "a.go"},
		{ID: "b", Kind: "function", SourceID: "a.go"},
		{ID: "c", Kind: "function", SourceID: "b.go"},
	}))
	require.Len(t, engine.NodesBySource("a.go"), 2)

	require.NoError(t, engine.LinkEdges([]EdgeRecord{
		{SourceID: "a", TargetID: "b", Kind: "calls", Weight: 1},
		{SourceID: "b", TargetID: "c", Kind: "calls", Weight: 1},
	}))
	require.Len(t, engine.GetOutEdges("a", "calls"), 1)
	require.Len(t, engine.GetOutEdges("b", "calls"), 1)

	require.NoError(t, engine.DeleteNodes([]string{"a", "b"}))
	_, okA := engine.GetNode("a")
	_, okB := engine.GetNode("b")
	require.False(t, okA)
	require.False(t, okB)
}

func TestLinkAndUnlinkOperations(t *testing.T) {
	engine, _ := newTestEngine(t)
	require.NoError(t, engine.UpsertNode(NodeRecord{ID: "a", Kind: "function"}))
	require.NoError(t, engine.UpsertNode(NodeRecord{ID: "b", Kind: "function"}))

	require.NoError(t, engine.Link("a", "b", "calls", "called_by", 1, map[string]any{"site": "x"}))
	out := engine.GetOutEdges("a")
	in := engine.GetInEdges("b")
	require.Len(t, out, 1)
	require.Len(t, in, 1)
	require.Equal(t, EdgeKind("calls"), out[0].Kind)
	require.Equal(t, EdgeKind("calls"), in[0].Kind)
	require.Len(t, engine.GetOutEdges("b"), 1)
	require.Equal(t, EdgeKind("called_by"), engine.GetOutEdges("b")[0].Kind)

	require.NoError(t, engine.Unlink("a", "b", "calls", false))
	soft := allOutEdges(t, engine, "a")
	require.Len(t, soft, 1)
	require.False(t, soft[0].IsActive())
	require.Empty(t, engine.GetOutEdges("a"))

	require.NoError(t, engine.Unlink("b", "a", "called_by", true))
	require.Empty(t, allOutEdges(t, engine, "b"))
}

func TestDeleteNodeSoftDeletesConnectedEdges(t *testing.T) {
	engine, _ := newTestEngine(t)
	for _, id := range []string{"a", "b", "c"} {
		require.NoError(t, engine.UpsertNode(NodeRecord{ID: id, Kind: "function"}))
	}
	require.NoError(t, engine.Link("a", "b", "calls", "", 1, nil))
	require.NoError(t, engine.Link("c", "b", "calls", "", 1, nil))

	require.NoError(t, engine.DeleteNode("b"))
	_, ok := engine.GetNode("b")
	require.False(t, ok)
	require.False(t, allOutEdges(t, engine, "a")[0].IsActive())
	require.False(t, allInEdges(t, engine, "b")[0].IsActive())
}

func TestReplayAndLastWriteWins(t *testing.T) {
	engine, opts := newTestEngine(t)
	require.NoError(t, engine.UpsertNode(NodeRecord{ID: "n1", Kind: "function"}))
	require.NoError(t, engine.UpsertNode(NodeRecord{ID: "n2", Kind: "function"}))
	require.NoError(t, engine.Link("n1", "n2", "calls", "", 1, nil))
	require.NoError(t, engine.DeleteNode("n1"))
	require.NoError(t, engine.Close())

	reopened, err := Open(opts)
	require.NoError(t, err)
	defer reopened.Close()

	_, ok := reopened.GetNode("n1")
	require.False(t, ok)
	out := allOutEdges(t, reopened, "n1")
	require.Len(t, out, 1)
	require.False(t, out[0].IsActive())
}

func TestPartialFrameAtEOFIsDiscarded(t *testing.T) {
	engine, opts := newTestEngine(t)
	require.NoError(t, engine.UpsertNode(NodeRecord{ID: "n1", Kind: "function"}))
	require.NoError(t, engine.Close())

	opBytes, err := json.Marshal(struct {
		Kind string          `json:"kind"`
		Data json.RawMessage `json:"data"`
	}{
		Kind: "upsert_node",
		Data: mustJSON(t, nodeOp{Node: NodeRecord{ID: "n2", Kind: "function"}}),
	})
	require.NoError(t, err)
	frame := encodeFrame(frameTypeOp, opBytes)
	path := filepath.Join(opts.DataDir, opts.AOFFileName)
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	require.NoError(t, err)
	_, err = file.Write(frame[:len(frame)-2])
	require.NoError(t, err)
	require.NoError(t, file.Close())

	reopened, err := Open(opts)
	require.NoError(t, err)
	defer reopened.Close()

	_, ok1 := reopened.GetNode("n1")
	_, ok2 := reopened.GetNode("n2")
	require.True(t, ok1)
	require.False(t, ok2)
}

func TestImpactSetFindPathNeighborsAndSubgraph(t *testing.T) {
	engine, _ := newTestEngine(t)
	for _, id := range []string{"a", "b", "c", "d"} {
		require.NoError(t, engine.UpsertNode(NodeRecord{ID: id, Kind: "function"}))
	}
	require.NoError(t, engine.Link("a", "b", "calls", "", 1, nil))
	require.NoError(t, engine.Link("b", "c", "calls", "", 1, nil))
	require.NoError(t, engine.Link("a", "d", "imports", "", 1, nil))

	impact := engine.ImpactSet([]string{"a"}, []EdgeKind{"calls"}, 3)
	require.ElementsMatch(t, []string{"b", "c"}, impact.Affected)
	require.Equal(t, []string{"a"}, impact.ByDepth[0])
	require.Equal(t, []string{"b"}, impact.ByDepth[1])
	require.Equal(t, []string{"c"}, impact.ByDepth[2])

	path, err := engine.FindPath("a", "c", []EdgeKind{"calls"}, 3)
	require.NoError(t, err)
	require.Equal(t, []string{"a", "b", "c"}, path.Path)
	noPath, err := engine.FindPath("d", "c", []EdgeKind{"calls"}, 2)
	require.NoError(t, err)
	require.Nil(t, noPath)

	require.Equal(t, []string{"b", "d"}, engine.Neighbors("a", DirectionOut))

	nodes, edges := engine.Subgraph(GraphQuery{RootIDs: []string{"a"}, Direction: DirectionOut, MaxDepth: 2})
	require.Len(t, nodes, 4)
	require.Len(t, edges, 3)
}

func TestSnapshotRecovery(t *testing.T) {
	engine, opts := newTestEngine(t)
	require.NoError(t, engine.UpsertNode(NodeRecord{ID: "n1", Kind: "function"}))
	require.NoError(t, engine.UpsertNode(NodeRecord{ID: "n2", Kind: "function"}))
	require.NoError(t, engine.Link("n1", "n2", "calls", "", 1, nil))
	require.NoError(t, engine.Snapshot())
	require.NoError(t, engine.Close())

	require.NoError(t, os.Truncate(filepath.Join(opts.DataDir, opts.AOFFileName), 0))
	reopened, err := Open(opts)
	require.NoError(t, err)
	defer reopened.Close()

	node, ok := reopened.GetNode("n1")
	require.True(t, ok)
	require.Equal(t, "n1", node.ID)
	require.Len(t, reopened.GetOutEdges("n1"), 1)
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
			require.NoError(t, engine.UpsertNode(NodeRecord{ID: id, Kind: "function"}))
			if i > 0 {
				prev := string(rune('a' + i - 1))
				require.NoError(t, engine.Link(prev, id, "calls", "", 1, nil))
			}
		}(i)
	}
	wg.Wait()
	require.Len(t, engine.ListNodes("function"), workers)
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

func TestLinkWithInversePersistsAsSingleBatchFrame(t *testing.T) {
	engine, opts := newTestEngine(t)
	require.NoError(t, engine.UpsertNode(NodeRecord{ID: "a", Kind: "function"}))
	require.NoError(t, engine.UpsertNode(NodeRecord{ID: "b", Kind: "function"}))
	require.NoError(t, engine.Link("a", "b", "calls", "called_by", 1, nil))
	require.NoError(t, engine.Close())

	file, err := os.Open(filepath.Join(opts.DataDir, opts.AOFFileName))
	require.NoError(t, err)
	defer file.Close()

	var opKinds []string
	for {
		_, payload, err := readFrame(file)
		if err != nil {
			require.ErrorIs(t, err, io.EOF)
			break
		}
		kind, err := opKindForPayload(payload)
		require.NoError(t, err)
		opKinds = append(opKinds, kind)
	}
	require.Contains(t, opKinds, "link_edges")
}

func TestCloseWithSnapshotOnCloseRewritesAOF(t *testing.T) {
	engine, opts := newTestEngine(t)
	engine.opts.SnapshotOnClose = true
	for i := 0; i < 32; i++ {
		require.NoError(t, engine.UpsertNode(NodeRecord{ID: "n" + string(rune('a'+i)), Kind: "function"}))
	}
	require.NoError(t, engine.Close())

	info, err := os.Stat(filepath.Join(opts.DataDir, opts.AOFFileName))
	require.NoError(t, err)
	require.Zero(t, info.Size())

	reopened, err := Open(opts)
	require.NoError(t, err)
	defer reopened.Close()
	require.Len(t, reopened.ListNodes("function"), 32)
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
