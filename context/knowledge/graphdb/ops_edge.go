package graphdb

import (
	"encoding/json"
	"time"

	"slices"
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
	if err := e.checkDirty(); err != nil {
		return err
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
	if err := e.applyHook(); err != nil {
		e.markDirty(err)
		return err
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
	if err := e.checkDirty(); err != nil {
		return err
	}
	if err := e.persist("unlink_edge", unlinkOp{SourceID: sourceID, TargetID: targetID, Kind: kind, Hard: hard}); err != nil {
		return err
	}
	if err := e.applyHook(); err != nil {
		e.markDirty(err)
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
	if existing := e.store.forward[edge.SourceID]; len(existing) > 0 {
		for _, current := range existing {
			if current.SourceID == edge.SourceID && current.TargetID == edge.TargetID && current.Kind == edge.Kind && current.DeletedAt == 0 {
				if edgeRecordEqual(current, edge) {
					return
				}
				e.store.edgeHistory[edgeHistoryKey(edge.SourceID, edge.TargetID, edge.Kind)] = append(
					e.store.edgeHistory[edgeHistoryKey(edge.SourceID, edge.TargetID, edge.Kind)],
					cloneEdge(current),
				)
				break
			}
		}
	}
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
	key := edgeHistoryKey(sourceID, targetID, kind)
	for _, edge := range e.store.forward[sourceID] {
		if edge.SourceID == sourceID && edge.TargetID == targetID && edge.Kind == kind {
			e.store.edgeHistory[key] = append(e.store.edgeHistory[key], cloneEdge(edge))
			break
		}
	}
	e.store.forward[sourceID] = mutateEdgeSlice(e.store.forward[sourceID], sourceID, targetID, kind, hard, deletedAt)
	e.store.reverse[targetID] = mutateEdgeSlice(e.store.reverse[targetID], sourceID, targetID, kind, hard, deletedAt)
}

// AnnotateEdge merges JSON properties into an edge while preserving history.
func (e *Engine) AnnotateEdge(sourceID, targetID string, kind EdgeKind, props map[string]any) error {
	if sourceID == "" || targetID == "" || kind == "" || len(props) == 0 {
		return nil
	}
	if err := e.checkDirty(); err != nil {
		return err
	}
	if err := e.persist("annotate_edge", annotateEdgeOp{
		SourceID: sourceID,
		TargetID: targetID,
		Kind:     kind,
		Props:    props,
	}); err != nil {
		return err
	}
	e.store.mu.Lock()
	defer e.store.mu.Unlock()
	if err := e.annotateEdgeLocked(sourceID, targetID, kind, props); err != nil {
		e.markDirty(err)
		return err
	}
	return nil
}

func (e *Engine) annotateEdgeLocked(sourceID, targetID string, kind EdgeKind, props map[string]any) error {
	edges := e.store.forward[sourceID]
	for i := range edges {
		if edges[i].SourceID != sourceID || edges[i].TargetID != targetID || edges[i].Kind != kind || edges[i].DeletedAt != 0 {
			continue
		}
		merged, err := mergeJSONProps(edges[i].Props, props)
		if err != nil {
			return err
		}
		if slices.Equal(edges[i].Props, merged) {
			return nil
		}
		key := edgeHistoryKey(sourceID, targetID, kind)
		e.store.edgeHistory[key] = append(e.store.edgeHistory[key], cloneEdge(edges[i]))
		edges[i].Props = merged
		e.store.forward[sourceID][i] = edges[i]
		for j := range e.store.reverse[targetID] {
			if e.store.reverse[targetID][j].SourceID == sourceID && e.store.reverse[targetID][j].TargetID == targetID && e.store.reverse[targetID][j].Kind == kind {
				e.store.reverse[targetID][j] = edges[i]
			}
		}
		return nil
	}
	return nil
}

// ReplaceEdge supersedes an existing edge with a new revision.
func (e *Engine) ReplaceEdge(oldSourceID, oldTargetID string, oldKind EdgeKind, replacement EdgeRecord) error {
	if err := e.Unlink(oldSourceID, oldTargetID, oldKind, false); err != nil {
		return err
	}
	return e.LinkEdges([]EdgeRecord{replacement})
}

// EdgeRevisions returns the revision history for an edge, oldest first.
func (e *Engine) EdgeRevisions(sourceID, targetID string, kind EdgeKind) []EdgeRecord {
	e.store.mu.RLock()
	defer e.store.mu.RUnlock()
	return cloneEdgeHistory(e.store.edgeHistory[edgeHistoryKey(sourceID, targetID, kind)])
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
