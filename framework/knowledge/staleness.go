package knowledge

import "sort"

// StalenessManager applies freshness transitions and propagation.
type StalenessManager struct {
	Store     *ChunkStore
	MaxDepth  int
	Propagate bool
}

func (m *StalenessManager) MarkStale(id ChunkID) error {
	_, err := m.markOne(id, FreshnessStale)
	return err
}

func (m *StalenessManager) MarkInvalid(id ChunkID) error {
	_, err := m.markOne(id, FreshnessInvalid)
	return err
}

func (m *StalenessManager) BulkMarkStale(ids []ChunkID) error {
	_, err := m.BulkMarkStaleCollect(ids)
	return err
}

func (m *StalenessManager) BulkMarkStaleCollect(ids []ChunkID) ([]ChunkID, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	seen := make(map[ChunkID]struct{}, len(ids))
	changed := make([]ChunkID, 0, len(ids))
	for _, id := range ids {
		marked, err := m.markOne(id, FreshnessStale)
		if err != nil {
			return nil, err
		}
		for _, chunkID := range marked {
			if _, ok := seen[chunkID]; ok {
				continue
			}
			seen[chunkID] = struct{}{}
			changed = append(changed, chunkID)
		}
	}
	sort.Slice(changed, func(i, j int) bool { return changed[i] < changed[j] })
	return changed, nil
}

func (m *StalenessManager) markOne(id ChunkID, freshness FreshnessState) ([]ChunkID, error) {
	if m == nil || m.Store == nil || id == "" {
		return nil, nil
	}
	chunk, ok, err := m.Store.Load(id)
	if err != nil || !ok || chunk == nil {
		return nil, err
	}
	if chunk.Freshness == freshness {
		return nil, nil
	}
	chunk.Freshness = freshness
	if _, err := m.Store.Save(*chunk); err != nil {
		return nil, err
	}
	changed := []ChunkID{id}
	if !m.Propagate {
		return changed, nil
	}
	propagated, err := m.propagate(id, freshness)
	if err != nil {
		return nil, err
	}
	return append(changed, propagated...), nil
}

func (m *StalenessManager) propagate(origin ChunkID, freshness FreshnessState) ([]ChunkID, error) {
	type entry struct {
		id    ChunkID
		depth int
	}
	limit := m.MaxDepth
	if limit <= 0 {
		limit = 3
	}
	queue := []entry{{id: origin, depth: 0}}
	seen := map[ChunkID]struct{}{origin: {}}
	changed := make([]ChunkID, 0)
	for len(queue) > 0 {
		item := queue[0]
		queue = queue[1:]
		if item.depth >= limit {
			continue
		}
		edges, err := m.Store.LoadEdgesFrom(item.id, EdgeKindInvalidates)
		if err != nil {
			return nil, err
		}
		for _, edge := range edges {
			if edge.ToChunk == "" {
				continue
			}
			if _, ok := seen[edge.ToChunk]; ok {
				continue
			}
			seen[edge.ToChunk] = struct{}{}
			chunk, ok, err := m.Store.Load(edge.ToChunk)
			if err != nil {
				return nil, err
			}
			if ok && chunk != nil && chunk.Freshness != freshness {
				chunk.Freshness = freshness
				if _, err := m.Store.Save(*chunk); err != nil {
					return nil, err
				}
				changed = append(changed, edge.ToChunk)
			}
			queue = append(queue, entry{id: edge.ToChunk, depth: item.depth + 1})
		}
	}
	return changed, nil
}
