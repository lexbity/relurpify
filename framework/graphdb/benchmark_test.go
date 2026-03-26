package graphdb

import (
	"fmt"
	"path/filepath"
	"testing"
)

func BenchmarkNodesBySource(b *testing.B) {
	engine := newBenchmarkEngine()
	const (
		sourceCount       = 200
		nodesPerSource    = 64
		targetSourceIndex = 117
	)
	for sourceIdx := 0; sourceIdx < sourceCount; sourceIdx++ {
		sourceID := fmt.Sprintf("file-%03d.go", sourceIdx)
		for nodeIdx := 0; nodeIdx < nodesPerSource; nodeIdx++ {
			node := NodeRecord{
				ID:       fmt.Sprintf("%s#n-%03d", sourceID, nodeIdx),
				Kind:     "function",
				SourceID: sourceID,
			}
			engine.applyUpsertNode(node)
		}
	}
	targetSource := fmt.Sprintf("file-%03d.go", targetSourceIndex)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		nodes := engine.NodesBySource(targetSource)
		if len(nodes) != nodesPerSource {
			b.Fatalf("NodesBySource(%q) = %d, want %d", targetSource, len(nodes), nodesPerSource)
		}
	}
}

func BenchmarkDeleteNodeHighDegree(b *testing.B) {
	const degree = 512

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		engine := newBenchmarkEngine()
		engine.applyUpsertNode(NodeRecord{ID: "hub", Kind: "function"})
		for edgeIdx := 0; edgeIdx < degree; edgeIdx++ {
			leafID := fmt.Sprintf("leaf-%03d", edgeIdx)
			engine.applyUpsertNode(NodeRecord{ID: leafID, Kind: "function"})
			engine.applyLinkEdge(EdgeRecord{SourceID: "hub", TargetID: leafID, Kind: "calls"})
			engine.applyLinkEdge(EdgeRecord{SourceID: leafID, TargetID: "hub", Kind: "called_by"})
		}
		b.StartTimer()
		engine.applyDeleteNode("hub", 1)
	}
}

func BenchmarkImpactSetWideGraph(b *testing.B) {
	engine := newBenchmarkEngine()
	buildBranchingGraph(engine, "root", 5, 5)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result := engine.ImpactSet([]string{"root"}, []EdgeKind{"calls"}, 5)
		if len(result.Affected) == 0 {
			b.Fatal("expected affected nodes")
		}
	}
}

func BenchmarkFindPathLinearGraph(b *testing.B) {
	engine := newBenchmarkEngine()
	const nodeCount = 2048
	for i := 0; i < nodeCount; i++ {
		id := fmt.Sprintf("n-%04d", i)
		engine.applyUpsertNode(NodeRecord{ID: id, Kind: "function"})
		if i > 0 {
			engine.applyLinkEdge(EdgeRecord{
				SourceID: fmt.Sprintf("n-%04d", i-1),
				TargetID: id,
				Kind:     "calls",
			})
		}
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		path, err := engine.FindPath("n-0000", fmt.Sprintf("n-%04d", nodeCount-1), []EdgeKind{"calls"}, nodeCount)
		if err != nil {
			b.Fatal(err)
		}
		if path == nil || len(path.Path) != nodeCount {
			b.Fatalf("unexpected path length: got %d want %d", len(path.Path), nodeCount)
		}
	}
}

func BenchmarkFindPathBranchingGraph(b *testing.B) {
	engine := newBenchmarkEngine()
	buildBranchingGraph(engine, "root", 6, 4)
	targetID := "root-0-3-1-3-2-3-3-3-4-3-5-3"

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		path, err := engine.FindPath("root", targetID, []EdgeKind{"calls"}, 6)
		if err != nil {
			b.Fatal(err)
		}
		if path == nil || len(path.Path) == 0 {
			b.Fatal("expected path")
		}
	}
}

func BenchmarkSubgraphBranchingGraph(b *testing.B) {
	engine := newBenchmarkEngine()
	buildBranchingGraph(engine, "root", 4, 6)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		nodes, edges := engine.Subgraph(GraphQuery{
			RootIDs:   []string{"root"},
			Direction: DirectionOut,
			MaxDepth:  4,
			EdgeKinds: []EdgeKind{"calls"},
		})
		if len(nodes) == 0 || len(edges) == 0 {
			b.Fatal("expected non-empty subgraph")
		}
	}
}

func BenchmarkOpenReplay(b *testing.B) {
	for _, snapshotOnClose := range []bool{false, true} {
		name := "aof_only"
		if snapshotOnClose {
			name = "snapshot_on_close"
		}
		b.Run(name, func(b *testing.B) {
			dir := b.TempDir()
			opts := DefaultOptions(filepath.Join(dir, "graphdb"))
			opts.SnapshotOnClose = snapshotOnClose
			engine, err := Open(opts)
			if err != nil {
				b.Fatal(err)
			}
			for i := 0; i < 512; i++ {
				sourceID := fmt.Sprintf("node-%03d", i)
				targetID := fmt.Sprintf("node-%03d", i+1)
				if err := engine.UpsertNode(NodeRecord{ID: sourceID, Kind: "function"}); err != nil {
					b.Fatal(err)
				}
				if err := engine.UpsertNode(NodeRecord{ID: targetID, Kind: "function"}); err != nil {
					b.Fatal(err)
				}
				if err := engine.Link(sourceID, targetID, "calls", "", 1, nil); err != nil {
					b.Fatal(err)
				}
			}
			if err := engine.Close(); err != nil {
				b.Fatal(err)
			}

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				reopened, err := Open(opts)
				if err != nil {
					b.Fatal(err)
				}
				if err := reopened.Close(); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkBatchUpsertAndLink(b *testing.B) {
	const batchSize = 512
	nodes := make([]NodeRecord, 0, batchSize)
	edges := make([]EdgeRecord, 0, batchSize-1)
	for i := 0; i < batchSize; i++ {
		id := fmt.Sprintf("node-%03d", i)
		nodes = append(nodes, NodeRecord{ID: id, Kind: "function", SourceID: "bench.go"})
		if i > 0 {
			edges = append(edges, EdgeRecord{
				SourceID: fmt.Sprintf("node-%03d", i-1),
				TargetID: id,
				Kind:     "calls",
				Weight:   1,
			})
		}
	}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		engine := newBenchmarkEngine()
		b.StartTimer()
		if err := engine.UpsertNodes(nodes); err != nil {
			b.Fatal(err)
		}
		if err := engine.LinkEdges(edges); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkPersistSyncMode(b *testing.B) {
	for _, mode := range []SyncMode{SyncAlways, SyncOnFlush} {
		b.Run(string(mode), func(b *testing.B) {
			dir := b.TempDir()
			opts := DefaultOptions(filepath.Join(dir, "graphdb"))
			opts.SyncMode = mode
			engine, err := Open(opts)
			if err != nil {
				b.Fatal(err)
			}
			defer engine.Close()

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if err := engine.UpsertNode(NodeRecord{ID: fmt.Sprintf("node-%d", i), Kind: "function"}); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func newBenchmarkEngine() *Engine {
	return &Engine{
		opts:  DefaultOptions(""),
		store: newAdjacencyStore(),
	}
}

func buildBranchingGraph(engine *Engine, root string, depth, fanout int) {
	engine.applyUpsertNode(NodeRecord{ID: root, Kind: "function"})
	currentLevel := []string{root}
	for level := 0; level < depth; level++ {
		nextLevel := make([]string, 0, len(currentLevel)*fanout)
		for _, parent := range currentLevel {
			for branch := 0; branch < fanout; branch++ {
				childID := fmt.Sprintf("%s-%d-%d", parent, level, branch)
				engine.applyUpsertNode(NodeRecord{ID: childID, Kind: "function"})
				engine.applyLinkEdge(EdgeRecord{
					SourceID: parent,
					TargetID: childID,
					Kind:     "calls",
				})
				nextLevel = append(nextLevel, childID)
			}
		}
		currentLevel = nextLevel
	}
}
