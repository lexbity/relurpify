package knowledge

import "sort"

// StalenessManager applies freshness transitions and propagation.
type StalenessManager struct {
	Store         *ChunkStore
	MaxDepth      int
	Propagate     bool
	propagateHook func([]ChunkID, int)
}

func (m *StalenessManager) MarkStale(id ChunkID) error {
	_, err := m.MarkOneSync(id, FreshnessStale)
	return err
}

func (m *StalenessManager) MarkInvalid(id ChunkID) error {
	_, err := m.MarkOneSync(id, FreshnessInvalid)
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
		marked, err := m.MarkOneSync(id, FreshnessStale)
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

// MarkOneSync updates a single chunk freshness without propagation.
func (m *StalenessManager) MarkOneSync(id ChunkID, freshness FreshnessState) ([]ChunkID, error) {
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
	return []ChunkID{id}, nil
}

// PropagateAsync schedules propagation by emitting a chunk-staled event.
func (m *StalenessManager) PropagateAsync(bus *EventBus, ids []ChunkID, reason string) {
	if bus == nil || len(ids) == 0 {
		return
	}
	bus.EmitChunkStaled(ChunkStaledPayload{
		ChunkIDs: chunkIDsToStrings(ids),
		Reason:   reason,
	})
}

// PropagateSync performs BFS propagation over invalidates and derives-from links.
// It returns newly marked chunk IDs, excluding the origin set.
func (m *StalenessManager) PropagateSync(ids []ChunkID, depth int) ([]ChunkID, error) {
	if m == nil || m.Store == nil || len(ids) == 0 {
		return nil, nil
	}
	if m.propagateHook != nil {
		m.propagateHook(ids, depth)
	}
	type entry struct {
		id    ChunkID
		depth int
	}
	limit := depth
	if limit <= 0 {
		limit = m.MaxDepth
	}
	if limit <= 0 {
		limit = 3
	}
	queue := make([]entry, 0, len(ids))
	seen := make(map[ChunkID]struct{}, len(ids))
	for _, id := range ids {
		if id == "" {
			continue
		}
		queue = append(queue, entry{id: id, depth: 0})
		seen[id] = struct{}{}
	}
	changed := make([]ChunkID, 0)
	for len(queue) > 0 {
		item := queue[0]
		queue = queue[1:]
		if item.depth >= limit {
			continue
		}
		edges, err := m.Store.LoadEdgesFrom(item.id, EdgeKindInvalidates, EdgeKindDerivesFrom)
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
			nextChunk, ok, err := m.Store.Load(edge.ToChunk)
			if err != nil {
				return nil, err
			}
			if ok && nextChunk != nil && nextChunk.Freshness != FreshnessStale {
				nextChunk.Freshness = FreshnessStale
				if _, err := m.Store.Save(*nextChunk); err != nil {
					return nil, err
				}
				changed = append(changed, edge.ToChunk)
			}
			queue = append(queue, entry{id: edge.ToChunk, depth: item.depth + 1})
		}
	}
	return changed, nil
}
