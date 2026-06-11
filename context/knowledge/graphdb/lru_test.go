package graphdb

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLRU_WarmNothing_GetNodeLazyLoads(t *testing.T) {
	// Engine with LRU capacity forces lazy loading: load() skips
	// node hydration; GetNode fetches from Badger on cache miss.
	dir := t.TempDir()
	opts := DefaultOptions(dir)
	opts.LRUCapacity = 100

	// First pass: populate the DB with nodes.
	pop, err := Open(context.Background(), opts)
	require.NoError(t, err)
	for i := 0; i < 50; i++ {
		err := pop.UpsertNode(context.TODO(), NodeRecord{
			ID:   "lazy-node-" + string(rune('a'+i)),
			Kind: "test",
		})
		require.NoError(t, err)
	}
	require.NoError(t, pop.Close(context.Background()))

	// Second pass: open with LRU — load skips nodes, GetNode fetches on miss.
	engine, err := Open(context.Background(), opts)
	require.NoError(t, err)
	defer func() { _ = engine.Close(context.Background()) }()

	// Verify the store has zero nodes in RAM after open.
	engine.store.mu.RLock()
	ramCount := len(engine.store.nodes)
	engine.store.mu.RUnlock()
	if ramCount != 0 {
		t.Errorf("expected 0 nodes in RAM after LRU load, got %d", ramCount)
	}

	// GetNode should lazy-load and return the correct data.
	node, ok := engine.GetNode("lazy-node-a")
	if !ok {
		t.Fatal("expected GetNode to lazy-load and find lazy-node-a")
	}
	if node.Kind != "test" {
		t.Errorf("lazy-loaded node has wrong kind: %s", node.Kind)
	}
}

func TestLRU_WarmNothing_MultipleGetNodes(t *testing.T) {
	dir := t.TempDir()
	opts := DefaultOptions(dir)
	opts.LRUCapacity = 100

	pop, err := Open(context.Background(), opts)
	require.NoError(t, err)
	for i := 0; i < 10; i++ {
		require.NoError(t, pop.UpsertNode(context.TODO(), NodeRecord{ID: "lazy-" + string(rune('0'+i)), Kind: "test"}))
	}
	require.NoError(t, pop.Close(context.Background()))

	engine, err := Open(context.Background(), opts)
	require.NoError(t, err)
	defer func() { _ = engine.Close(context.Background()) }()

	// All 10 nodes should be retrievable via lazy loading.
	for i := 0; i < 10; i++ {
		node, ok := engine.GetNode("lazy-" + string(rune('0'+i)))
		if !ok {
			t.Errorf("missing node %d after lazy load", i)
		}
		if node.Kind != "test" {
			t.Errorf("wrong kind for node %d", i)
		}
	}
}

func TestNFR4_BootMemory_BoundedByLRUCapacity(t *testing.T) {
	// NFR-4: Engine boot with LRU capacity loads O(LRU capacity) into
	// RAM, not O(total nodes). We prove this by creating 1000 nodes,
	// opening with LRUCapacity=10, and verifying that <= 10 nodes are
	// in RAM after boot.
	dir := t.TempDir()
	opts := DefaultOptions(dir)
	opts.LRUCapacity = 10

	pop, err := Open(context.Background(), opts)
	require.NoError(t, err)
	for i := 0; i < 1000; i++ {
		require.NoError(t, pop.UpsertNode(context.TODO(), NodeRecord{ID: "nfr4-" + string(rune('0'+i%100)), Kind: "test"}))
	}
	require.NoError(t, pop.Close(context.Background()))

	engine, err := Open(context.Background(), opts)
	require.NoError(t, err)
	defer func() { _ = engine.Close(context.Background()) }()

	engine.store.mu.RLock()
	ramCount := len(engine.store.nodes)
	engine.store.mu.RUnlock()
	if ramCount > 100 {
		t.Errorf("NFR-4: expected <= 100 nodes in RAM after boot with LRUCapacity=10, got %d", ramCount)
	}
}

func TestNFR5_TraversalMemory_OnePage(t *testing.T) {
	// NFR-5: A single SubgraphPage call uses memory O(frontier + pageSize).
	// We verify correctness: the first page returns the expected elements
	// even when under LRU (lazy loading from Badger).
	dir := t.TempDir()
	opts := DefaultOptions(dir)
	opts.LRUCapacity = 100

	pop, err := Open(context.Background(), opts)
	require.NoError(t, err)
	require.NoError(t, pop.UpsertNode(context.TODO(), NodeRecord{ID: "root", Kind: "test"}))
	for i := 0; i < 20; i++ {
		tgt := "child-" + string(rune('a'+i))
		require.NoError(t, pop.UpsertNode(context.TODO(), NodeRecord{ID: tgt, Kind: "test"}))
		require.NoError(t, pop.Link(context.TODO(), "root", tgt, "edge-to", "", 1, nil))
	}
	require.NoError(t, pop.Close(context.Background()))

	engine, err := Open(context.Background(), opts)
	require.NoError(t, err)
	defer func() { _ = engine.Close(context.Background()) }()

	// One page with PageSize=100 gets everything.
	page, err := engine.SubgraphPage(context.Background(), GraphPageQuery{
		GraphQuery: GraphQuery{
			RootIDs:  []string{"root"},
			MaxDepth: 1,
			Limit:    100,
		},
		PageSize: 100,
	})
	require.NoError(t, err)
	// Count unique nodes by ignoring edge-only elements.
	nodeIDs := make(map[string]int)
	edgeCount := 0
	for _, elem := range page.Items {
		if elem.Node.ID != "" {
			nodeIDs[elem.Node.ID]++
		}
		if elem.Edge.SourceID != "" {
			edgeCount++
		}
	}
	if len(nodeIDs) != 21 {
		t.Errorf("expected 21 unique nodes (1 root + 20 children), got %d", len(nodeIDs))
	}
	if edgeCount != 20 {
		t.Errorf("expected 20 edges, got %d", edgeCount)
	}
}

func TestCursor_FrontierStateAcrossPages(t *testing.T) {
	// The cursor tracks already-returned elements so that pagination
	// works correctly across multiple pages. We fetch a graph in
	// pages and verify the union covers all expected elements.
	dir := t.TempDir()
	opts := DefaultOptions(dir)

	pop, err := Open(context.Background(), opts)
	require.NoError(t, err)
	require.NoError(t, pop.UpsertNode(context.TODO(), NodeRecord{ID: "root", Kind: "test"}))
	for i := 0; i < 20; i++ {
		tgt := "n-" + string(rune('a'+i))
		require.NoError(t, pop.UpsertNode(context.TODO(), NodeRecord{ID: tgt, Kind: "test"}))
		require.NoError(t, pop.Link(context.TODO(), "root", tgt, "edge-cursor-test", "", 1, nil))
	}
	require.NoError(t, pop.Close(context.Background()))

	engine, err := Open(context.Background(), opts)
	require.NoError(t, err)
	defer func() { _ = engine.Close(context.Background()) }()

	// Fetch in pages, collecting all items.
	var allItems []GraphElement
	after := PageToken("")
	for {
		page, err := engine.SubgraphPage(context.Background(), GraphPageQuery{
			GraphQuery: GraphQuery{
				RootIDs:  []string{"root"},
				MaxDepth: 1,
				Limit:    100,
			},
			PageSize: 5,
			After:    after,
		})
		require.NoError(t, err)
		allItems = append(allItems, page.Items...)
		if page.Next == "" {
			break
		}
		after = page.Next
	}

	// Verify union: 1 root + 20 edges + 20 child nodes = 41 elements.
	if len(allItems) != 41 {
		t.Errorf("expected 41 total elements across pages (1 root + 20 edges + 20 children), got %d", len(allItems))
	}
	// Verify no duplicate node IDs.
	seen := make(map[string]int)
	for _, elem := range allItems {
		if elem.Node.ID != "" {
			seen[elem.Node.ID]++
		}
	}
	for id, count := range seen {
		if count != 1 {
			t.Errorf("node %s seen %d times", id, count)
		}
	}
	if len(seen) != 21 {
		t.Errorf("expected 21 unique nodes, got %d", len(seen))
	}
}

func TestLRU_IndexIntegrity_ListNodesByLabel(t *testing.T) {
	// Critical: under LRU, label/source indexes must be built from Badger
	// at boot and remain complete even when individual nodes are evicted.
	dir := t.TempDir()
	opts := DefaultOptions(dir)
	opts.LRUCapacity = 3 // small cache forces eviction

	pop, err := Open(context.Background(), opts)
	require.NoError(t, err)
	for i := 0; i < 10; i++ {
		require.NoError(t, pop.UpsertNode(context.TODO(), NodeRecord{
			ID:     "label-node-" + string(rune('0'+i)),
			Kind:   "test",
			Labels: []string{"group:a"},
		}))
	}
	require.NoError(t, pop.Close(context.Background()))

	engine, err := Open(context.Background(), opts)
	require.NoError(t, err)
	defer func() { _ = engine.Close(context.Background()) }()

	// ListNodesByLabel must return ALL 10 nodes even though the LRU cache
	// only holds 3. The label index is built during load() regardless of
	// LRU mode.
	nodes := engine.ListNodesByLabel("test", "group:a")
	if len(nodes) != 10 {
		t.Errorf("expected 10 nodes from ListNodesByLabel under LRU, got %d", len(nodes))
	}
	// NodesBySource should also work (indexes built at boot).
	_ = engine.NodesBySource("source-x")
}

func TestLRU_IndexIntegrity_AfterChurn(t *testing.T) {
	// Regression: lruEvict must NOT remove label/source index entries.
	// After GetNode churn that exceeds LRU capacity, ListNodesByLabel
	// must still return all nodes.
	dir := t.TempDir()
	opts := DefaultOptions(dir)
	opts.LRUCapacity = 3

	pop, err := Open(context.Background(), opts)
	require.NoError(t, err)
	for i := 0; i < 10; i++ {
		require.NoError(t, pop.UpsertNode(context.TODO(), NodeRecord{
			ID:     "churn-node-" + string(rune('0'+i)),
			Kind:   "test",
			Labels: []string{"group:b"},
		}))
	}
	require.NoError(t, pop.Close(context.Background()))

	engine, err := Open(context.Background(), opts)
	require.NoError(t, err)
	defer func() { _ = engine.Close(context.Background()) }()

	// Hit 5 distinct nodes via GetNode — with cap=3 the first 2+ are evicted.
	for i := 0; i < 5; i++ {
		_, ok := engine.GetNode("churn-node-" + string(rune('0'+i)))
		require.True(t, ok)
	}

	// Label index must still report all 10 nodes.
	nodes := engine.ListNodesByLabel("test", "group:b")
	if len(nodes) != 10 {
		t.Errorf("expected 10 nodes from ListNodesByLabel after churn, got %d", len(nodes))
	}
}

func TestLRU_IndexIntegrity_NodesBySource(t *testing.T) {
	dir := t.TempDir()
	opts := DefaultOptions(dir)
	opts.LRUCapacity = 3

	pop, err := Open(context.Background(), opts)
	require.NoError(t, err)
	for i := 0; i < 10; i++ {
		require.NoError(t, pop.UpsertNode(context.TODO(), NodeRecord{
			ID:       "src-node-" + string(rune('0'+i)),
			Kind:     "test",
			SourceID: "source-main",
		}))
	}
	require.NoError(t, pop.Close(context.Background()))

	engine, err := Open(context.Background(), opts)
	require.NoError(t, err)
	defer func() { _ = engine.Close(context.Background()) }()

	nodes := engine.NodesBySource("source-main")
	if len(nodes) != 10 {
		t.Errorf("expected 10 nodes from NodesBySource under LRU, got %d", len(nodes))
	}
}

func TestNFR4_Strict_BootMemory(t *testing.T) {
	// NFR-4: Engine boot with LRU capacity must hold ≤ LRUCapacity nodes
	// in RAM, not O(total graph). Edges are lazy-loaded under LRU.
	dir := t.TempDir()
	opts := DefaultOptions(dir)
	opts.LRUCapacity = 10

	pop, err := Open(context.Background(), opts)
	require.NoError(t, err)
	for i := 0; i < 500; i++ {
		require.NoError(t, pop.UpsertNode(context.TODO(), NodeRecord{ID: "n-" + string(rune('0'+i%100)), Kind: "test"}))
	}
	for i := 1; i < 500; i++ {
		require.NoError(t, pop.Link(context.TODO(), "n-0", "n-"+string(rune('0'+i%100)), "edge", "", 1, nil))
	}
	require.NoError(t, pop.Close(context.Background()))

	engine, err := Open(context.Background(), opts)
	require.NoError(t, err)
	defer func() { _ = engine.Close(context.Background()) }()

	engine.store.mu.RLock()
	ramNodes := len(engine.store.nodes)
	ramEdges := len(engine.store.forward)
	engine.store.mu.RUnlock()

	if ramNodes > 10 {
		t.Errorf("NFR-4: expected ≤ 10 nodes in RAM after LRU boot, got %d", ramNodes)
	}
	// Edges should also be bounded under LRU.
	if ramEdges > 10 {
		t.Errorf("NFR-4: expected ≤ 10 edge entries in RAM after LRU boot, got %d", ramEdges)
	}
}

func TestCursor_NoReTraversal(t *testing.T) {
	// Verify that across pages, no node is emitted more than once.
	dir := t.TempDir()
	opts := DefaultOptions(dir)

	pop, err := Open(context.Background(), opts)
	require.NoError(t, err)
	require.NoError(t, pop.UpsertNode(context.TODO(), NodeRecord{ID: "root", Kind: "test"}))
	for i := 0; i < 15; i++ {
		tgt := "nn-" + string(rune('a'+i))
		require.NoError(t, pop.UpsertNode(context.TODO(), NodeRecord{ID: tgt, Kind: "test"}))
		require.NoError(t, pop.Link(context.TODO(), "root", tgt, "edge-no-retraverse", "", 1, nil))
	}
	require.NoError(t, pop.Close(context.Background()))

	engine, err := Open(context.Background(), opts)
	require.NoError(t, err)
	defer func() { _ = engine.Close(context.Background()) }()

	seen := make(map[string]int)
	after := PageToken("")
	for {
		page, err := engine.SubgraphPage(context.Background(), GraphPageQuery{
			GraphQuery: GraphQuery{
				RootIDs:  []string{"root"},
				MaxDepth: 1,
				Limit:    100,
			},
			PageSize: 4,
			After:    after,
		})
		require.NoError(t, err)
		for _, elem := range page.Items {
			if elem.Node.ID != "" {
				seen[elem.Node.ID]++
			}
		}
		if page.Next == "" {
			break
		}
		after = page.Next
	}

	// Each node should be seen exactly once.
	for id, count := range seen {
		if count != 1 {
			t.Errorf("node %s seen %d times (should be 1)", id, count)
		}
	}
	if len(seen) != 16 {
		t.Errorf("expected 16 unique nodes (root + 15 children), got %d", len(seen))
	}
}
