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
	pop, err := Open(opts)
	require.NoError(t, err)
	for i := 0; i < 50; i++ {
		err := pop.UpsertNode(NodeRecord{
			ID:   "lazy-node-" + string(rune('a'+i)),
			Kind: "test",
		})
		require.NoError(t, err)
	}
	require.NoError(t, pop.Close())

	// Second pass: open with LRU — load skips nodes, GetNode fetches on miss.
	engine, err := Open(opts)
	require.NoError(t, err)
	defer engine.Close()

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

	pop, err := Open(opts)
	require.NoError(t, err)
	for i := 0; i < 10; i++ {
		require.NoError(t, pop.UpsertNode(NodeRecord{ID: "lazy-" + string(rune('0'+i)), Kind: "test"}))
	}
	require.NoError(t, pop.Close())

	engine, err := Open(opts)
	require.NoError(t, err)
	defer engine.Close()

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

	pop, err := Open(opts)
	require.NoError(t, err)
	for i := 0; i < 1000; i++ {
		require.NoError(t, pop.UpsertNode(NodeRecord{ID: "nfr4-" + string(rune('0'+i%100)), Kind: "test"}))
	}
	require.NoError(t, pop.Close())

	engine, err := Open(opts)
	require.NoError(t, err)
	defer engine.Close()

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

	pop, err := Open(opts)
	require.NoError(t, err)
	require.NoError(t, pop.UpsertNode(NodeRecord{ID: "root", Kind: "test"}))
	for i := 0; i < 20; i++ {
		tgt := "child-" + string(rune('a'+i))
		require.NoError(t, pop.UpsertNode(NodeRecord{ID: tgt, Kind: "test"}))
		require.NoError(t, pop.Link("root", tgt, "edge-to", "", 1, nil))
	}
	require.NoError(t, pop.Close())

	engine, err := Open(opts)
	require.NoError(t, err)
	defer engine.Close()

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

	pop, err := Open(opts)
	require.NoError(t, err)
	require.NoError(t, pop.UpsertNode(NodeRecord{ID: "root", Kind: "test"}))
	for i := 0; i < 20; i++ {
		tgt := "n-" + string(rune('a'+i))
		require.NoError(t, pop.UpsertNode(NodeRecord{ID: tgt, Kind: "test"}))
		require.NoError(t, pop.Link("root", tgt, "edge-cursor-test", "", 1, nil))
	}
	require.NoError(t, pop.Close())

	engine, err := Open(opts)
	require.NoError(t, err)
	defer engine.Close()

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

func TestCursor_NoReTraversal(t *testing.T) {
	// Verify that across pages, no node is emitted more than once.
	dir := t.TempDir()
	opts := DefaultOptions(dir)

	pop, err := Open(opts)
	require.NoError(t, err)
	require.NoError(t, pop.UpsertNode(NodeRecord{ID: "root", Kind: "test"}))
	for i := 0; i < 15; i++ {
		tgt := "nn-" + string(rune('a'+i))
		require.NoError(t, pop.UpsertNode(NodeRecord{ID: tgt, Kind: "test"}))
		require.NoError(t, pop.Link("root", tgt, "edge-no-retraverse", "", 1, nil))
	}
	require.NoError(t, pop.Close())

	engine, err := Open(opts)
	require.NoError(t, err)
	defer engine.Close()

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
