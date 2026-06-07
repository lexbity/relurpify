package knowledge

import "sort"

// ChunkGraph provides traversal helpers over chunk edges.
type ChunkGraph struct {
	Store *ChunkStore
}

// ExtractRequiresContextSubgraph returns all reachable chunks and edges from the
// seed set following requires_context dependencies.
func (g *ChunkGraph) ExtractRequiresContextSubgraph(seeds []ChunkID) ([]KnowledgeChunk, []ChunkEdge, error) {
	if g == nil || g.Store == nil || len(seeds) == 0 {
		return nil, nil, nil
	}
	visited := make(map[ChunkID]struct{}, len(seeds))
	edgeSeen := make(map[EdgeID]struct{})
	queue := append([]ChunkID(nil), seeds...)
	chunks := make([]KnowledgeChunk, 0)
	edges := make([]ChunkEdge, 0)
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		if _, ok := visited[id]; ok {
			continue
		}
		visited[id] = struct{}{}
		chunk, ok, err := g.Store.Load(id)
		if err != nil {
			return nil, nil, err
		}
		if !ok || chunk == nil {
			continue
		}
		chunks = append(chunks, *chunk)
		out, err := g.Store.LoadEdgesFrom(id, EdgeKindRequiresContext)
		if err != nil {
			return nil, nil, err
		}
		for _, edge := range out {
			if _, ok := edgeSeen[edge.ID]; !ok {
				edgeSeen[edge.ID] = struct{}{}
				edges = append(edges, edge)
			}
			if edge.ToChunk != "" {
				queue = append(queue, edge.ToChunk)
			}
		}
	}
	return chunks, edges, nil
}

// OrderRequiresContext returns dependency-first chunk ordering. Cycles are
// handled safely by breaking them in traversal order.
func (g *ChunkGraph) OrderRequiresContext(seeds []ChunkID) ([]KnowledgeChunk, error) {
	if g == nil || g.Store == nil || len(seeds) == 0 {
		return nil, nil
	}
	perm := make(map[ChunkID]bool)
	temp := make(map[ChunkID]bool)
	loaded := make(map[ChunkID]KnowledgeChunk)
	order := make([]KnowledgeChunk, 0)
	var visit func(ChunkID) error
	visit = func(id ChunkID) error {
		if id == "" || perm[id] {
			return nil
		}
		if temp[id] {
			return nil
		}
		temp[id] = true
		chunk, ok, err := g.Store.Load(id)
		if err != nil {
			return err
		}
		if !ok || chunk == nil {
			delete(temp, id)
			perm[id] = true
			return nil
		}
		loaded[id] = *chunk
		out, err := g.Store.LoadEdgesFrom(id, EdgeKindRequiresContext)
		if err != nil {
			return err
		}
		sort.Slice(out, func(i, j int) bool {
			if out[i].Weight == out[j].Weight {
				return out[i].ToChunk < out[j].ToChunk
			}
			return out[i].Weight > out[j].Weight
		})
		for _, edge := range out {
			if err := visit(edge.ToChunk); err != nil {
				return err
			}
		}
		delete(temp, id)
		perm[id] = true
		order = append(order, loaded[id])
		return nil
	}
	for _, seed := range seeds {
		if err := visit(seed); err != nil {
			return nil, err
		}
	}
	return order, nil
}

// AmplifyFrom returns optional enrichment chunks following amplifies edges.
func (g *ChunkGraph) AmplifyFrom(seeds []ChunkID, maxDepth int) ([]KnowledgeChunk, error) {
	if g == nil || g.Store == nil || len(seeds) == 0 {
		return nil, nil
	}
	if maxDepth < 0 {
		maxDepth = 0
	}
	type entry struct {
		id    ChunkID
		depth int
	}
	queue := make([]entry, 0, len(seeds))
	for _, seed := range seeds {
		queue = append(queue, entry{id: seed, depth: 0})
	}
	seen := make(map[ChunkID]struct{})
	out := make([]KnowledgeChunk, 0)
	for len(queue) > 0 {
		item := queue[0]
		queue = queue[1:]
		if item.depth >= maxDepth {
			continue
		}
		edges, err := g.Store.LoadEdgesFrom(item.id, EdgeKindAmplifies)
		if err != nil {
			return nil, err
		}
		sort.Slice(edges, func(i, j int) bool {
			return edges[i].Weight > edges[j].Weight
		})
		for _, edge := range edges {
			if edge.ToChunk == "" {
				continue
			}
			if _, ok := seen[edge.ToChunk]; ok {
				continue
			}
			chunk, ok, err := g.Store.Load(edge.ToChunk)
			if err != nil {
				return nil, err
			}
			if !ok || chunk == nil {
				continue
			}
			seen[edge.ToChunk] = struct{}{}
			out = append(out, *chunk)
			queue = append(queue, entry{id: edge.ToChunk, depth: item.depth + 1})
		}
	}
	return out, nil
}
