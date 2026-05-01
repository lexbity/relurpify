package graphdb

import "time"

// UpsertNode inserts or updates a node.
func (e *Engine) UpsertNode(node NodeRecord) error {
	return e.UpsertNodes([]NodeRecord{node})
}

// UpsertNodes inserts or updates nodes in one durable batch.
func (e *Engine) UpsertNodes(nodes []NodeRecord) error {
	if len(nodes) == 0 {
		return nil
	}
	now := time.Now().UnixNano()
	batch := make([]NodeRecord, 0, len(nodes))
	for _, node := range nodes {
		if node.CreatedAt == 0 {
			node.CreatedAt = now
		}
		node.UpdatedAt = now
		batch = append(batch, node)
	}
	if len(batch) == 1 {
		if err := e.persist("upsert_node", nodeOp{Node: batch[0]}); err != nil {
			return err
		}
	} else if err := e.persist("upsert_nodes", nodeBatchOp{Nodes: batch}); err != nil {
		return err
	}
	e.store.mu.Lock()
	defer e.store.mu.Unlock()
	for _, node := range batch {
		e.applyUpsertNode(node)
	}
	return nil
}

// DeleteNode soft-deletes a node and all connected edges.
func (e *Engine) DeleteNode(id string) error {
	return e.DeleteNodes([]string{id})
}

// DeleteNodes soft-deletes nodes and connected edges in one durable batch.
func (e *Engine) DeleteNodes(ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	if len(ids) == 1 {
		if err := e.persist("delete_node", deleteNodeOp{ID: ids[0]}); err != nil {
			return err
		}
	} else if err := e.persist("delete_nodes", deleteNodesOp{IDs: ids}); err != nil {
		return err
	}
	deletedAt := time.Now().UnixNano()
	e.store.mu.Lock()
	defer e.store.mu.Unlock()
	for _, id := range ids {
		e.applyDeleteNode(id, deletedAt)
	}
	return nil
}

// GetNode returns a node by ID.
func (e *Engine) GetNode(id string) (NodeRecord, bool) {
	e.store.mu.RLock()
	defer e.store.mu.RUnlock()
	node, ok := e.store.nodes[id]
	if !ok || node.DeletedAt != 0 {
		return NodeRecord{}, false
	}
	out := cloneNode(node)
	return out, true
}

// ListNodes returns active nodes of the given kind.
func (e *Engine) ListNodes(kind NodeKind) []NodeRecord {
	e.store.mu.RLock()
	defer e.store.mu.RUnlock()
	out := make([]NodeRecord, 0)
	for _, node := range e.store.nodes {
		if node.DeletedAt != 0 {
			continue
		}
		if kind != "" && node.Kind != kind {
			continue
		}
		out = append(out, cloneNode(node))
	}
	return out
}

// ListNodesByLabel returns active nodes matching the given label and optional kind.
func (e *Engine) ListNodesByLabel(kind NodeKind, label string) []NodeRecord {
	e.store.mu.RLock()
	defer e.store.mu.RUnlock()
	ids := e.store.labels.Lookup(label)
	out := make([]NodeRecord, 0, len(ids))
	for _, id := range ids {
		node := e.store.nodes[id]
		if node == nil || node.DeletedAt != 0 {
			continue
		}
		if kind != "" && node.Kind != kind {
			continue
		}
		out = append(out, cloneNode(node))
	}
	return out
}

// ListNodesByLabelPrefix returns active nodes matching any label with the given prefix.
func (e *Engine) ListNodesByLabelPrefix(kind NodeKind, labelPrefix string) []NodeRecord {
	e.store.mu.RLock()
	defer e.store.mu.RUnlock()
	ids := e.store.labels.LookupPrefix(labelPrefix)
	out := make([]NodeRecord, 0, len(ids))
	for _, id := range ids {
		node := e.store.nodes[id]
		if node == nil || node.DeletedAt != 0 {
			continue
		}
		if kind != "" && node.Kind != kind {
			continue
		}
		out = append(out, cloneNode(node))
	}
	return out
}

// NodesBySource returns active nodes for a source ID.
func (e *Engine) NodesBySource(sourceID string) []NodeRecord {
	e.store.mu.RLock()
	defer e.store.mu.RUnlock()
	nodeIDs := e.store.bySource[sourceID]
	out := make([]NodeRecord, 0, len(nodeIDs))
	for nodeID := range nodeIDs {
		node := e.store.nodes[nodeID]
		if node == nil || node.DeletedAt != 0 || node.SourceID != sourceID {
			continue
		}
		out = append(out, cloneNode(node))
	}
	return out
}

func (e *Engine) applyUpsertNode(node NodeRecord) {
	existing, ok := e.store.nodes[node.ID]
	if ok && node.CreatedAt == 0 {
		node.CreatedAt = existing.CreatedAt
	}
	if ok && existing != nil && existing.SourceID != "" && existing.SourceID != node.SourceID {
		e.store.removeNodeSourceIndex(existing.ID, existing.SourceID)
	}
	if ok && existing != nil {
		e.store.removeNodeLabels(*existing)
	}
	n := node
	e.store.nodes[node.ID] = &n
	e.store.addNodeSourceIndex(node)
	e.store.addNodeLabels(node)
}

func (e *Engine) applyDeleteNode(id string, deletedAt int64) {
	if deletedAt == 0 {
		deletedAt = time.Now().UnixNano()
	}
	node, ok := e.store.nodes[id]
	if ok {
		e.store.removeNodeLabels(*node)
		node.DeletedAt = deletedAt
		node.UpdatedAt = deletedAt
		e.store.removeNodeSourceIndex(node.ID, node.SourceID)
	}
	outbound := cloneEdges(e.store.forward[id])
	inbound := cloneEdges(e.store.reverse[id])
	e.store.forward[id] = markEdgesDeleted(e.store.forward[id], deletedAt)
	e.store.reverse[id] = markEdgesDeleted(e.store.reverse[id], deletedAt)
	for _, edge := range outbound {
		e.store.reverse[edge.TargetID] = markSpecificEdgeDeleted(e.store.reverse[edge.TargetID], edge.SourceID, edge.TargetID, edge.Kind, deletedAt)
	}
	for _, edge := range inbound {
		e.store.forward[edge.SourceID] = markSpecificEdgeDeleted(e.store.forward[edge.SourceID], edge.SourceID, edge.TargetID, edge.Kind, deletedAt)
	}
}

func markEdgesDeleted(edges []EdgeRecord, deletedAt int64) []EdgeRecord {
	if len(edges) == 0 {
		return edges
	}
	out := edges[:0]
	for _, edge := range edges {
		edge.DeletedAt = deletedAt
		out = append(out, edge)
	}
	return out
}

func markSpecificEdgeDeleted(edges []EdgeRecord, sourceID, targetID string, kind EdgeKind, deletedAt int64) []EdgeRecord {
	if len(edges) == 0 {
		return edges
	}
	out := edges[:0]
	for _, edge := range edges {
		if edge.SourceID == sourceID && edge.TargetID == targetID && edge.Kind == kind {
			edge.DeletedAt = deletedAt
		}
		out = append(out, edge)
	}
	return out
}
