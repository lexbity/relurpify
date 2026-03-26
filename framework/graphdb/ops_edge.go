package graphdb

import (
	"encoding/json"
	"time"
)

// Link creates a directed edge and optionally its inverse.
func (e *Engine) Link(sourceID, targetID string, kind, inverseKind EdgeKind, weight float32, props map[string]any) error {
	raw, err := json.Marshal(props)
	if err != nil {
		return err
	}
	now := time.Now().UnixNano()
	edge := EdgeRecord{
		SourceID:  sourceID,
		TargetID:  targetID,
		Kind:      kind,
		Weight:    weight,
		Props:     raw,
		CreatedAt: now,
	}
	edges := []EdgeRecord{edge}
	var inverse EdgeRecord
	if inverseKind != "" {
		inverse = EdgeRecord{
			SourceID:  targetID,
			TargetID:  sourceID,
			Kind:      inverseKind,
			Weight:    weight,
			Props:     raw,
			CreatedAt: now,
		}
		edges = append(edges, inverse)
	}
	return e.LinkEdges(edges)
}

// LinkEdges creates or updates directed edges in one durable batch.
func (e *Engine) LinkEdges(edges []EdgeRecord) error {
	if len(edges) == 0 {
		return nil
	}
	if len(edges) == 1 {
		if err := e.persist("link_edge", edgeOp{Edge: edges[0]}); err != nil {
			return err
		}
	} else {
		if err := e.persist("link_edges", edgeBatchOp{Edges: edges}); err != nil {
			return err
		}
	}
	e.store.mu.Lock()
	defer e.store.mu.Unlock()
	for _, edge := range edges {
		e.applyLinkEdge(edge)
	}
	return nil
}

// Unlink soft-deletes or hard-removes an edge.
func (e *Engine) Unlink(sourceID, targetID string, kind EdgeKind, hard bool) error {
	if err := e.persist("unlink_edge", unlinkOp{SourceID: sourceID, TargetID: targetID, Kind: kind, Hard: hard}); err != nil {
		return err
	}
	e.store.mu.Lock()
	defer e.store.mu.Unlock()
	e.applyUnlink(sourceID, targetID, kind, hard, time.Now().UnixNano())
	return nil
}

// GetOutEdges returns active outgoing edges.
func (e *Engine) GetOutEdges(nodeID string, kinds ...EdgeKind) []EdgeRecord {
	e.store.mu.RLock()
	defer e.store.mu.RUnlock()
	return filterEdges(e.store.forward[nodeID], kinds)
}

// GetInEdges returns active incoming edges.
func (e *Engine) GetInEdges(nodeID string, kinds ...EdgeKind) []EdgeRecord {
	e.store.mu.RLock()
	defer e.store.mu.RUnlock()
	return filterEdges(e.store.reverse[nodeID], kinds)
}

func filterEdges(edges []EdgeRecord, kinds []EdgeKind) []EdgeRecord {
	allowed := kindSet(kinds)
	out := make([]EdgeRecord, 0, len(edges))
	for _, edge := range edges {
		if !edge.IsActive() || !matchKinds(edge.Kind, allowed) {
			continue
		}
		out = append(out, cloneEdge(edge))
	}
	return out
}

func (e *Engine) applyLinkEdge(edge EdgeRecord) {
	e.store.forward[edge.SourceID] = upsertEdge(e.store.forward[edge.SourceID], edge)
	e.store.reverse[edge.TargetID] = upsertEdge(e.store.reverse[edge.TargetID], edge)
}

func upsertEdge(edges []EdgeRecord, edge EdgeRecord) []EdgeRecord {
	for i := range edges {
		if edges[i].SourceID == edge.SourceID && edges[i].TargetID == edge.TargetID && edges[i].Kind == edge.Kind {
			edges[i] = cloneEdge(edge)
			return edges
		}
	}
	return append(edges, cloneEdge(edge))
}

func (e *Engine) applyUnlink(sourceID, targetID string, kind EdgeKind, hard bool, deletedAt int64) {
	if deletedAt == 0 {
		deletedAt = time.Now().UnixNano()
	}
	e.store.forward[sourceID] = mutateEdgeSlice(e.store.forward[sourceID], sourceID, targetID, kind, hard, deletedAt)
	e.store.reverse[targetID] = mutateEdgeSlice(e.store.reverse[targetID], sourceID, targetID, kind, hard, deletedAt)
}

func mutateEdgeSlice(edges []EdgeRecord, sourceID, targetID string, kind EdgeKind, hard bool, deletedAt int64) []EdgeRecord {
	if len(edges) == 0 {
		return edges
	}
	out := edges[:0]
	for _, edge := range edges {
		if edge.SourceID == sourceID && edge.TargetID == targetID && edge.Kind == kind {
			if hard {
				continue
			}
			edge.DeletedAt = deletedAt
		}
		out = append(out, edge)
	}
	return out
}
