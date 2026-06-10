package graphdb

// Neighbors returns depth-1 adjacent nodes in the requested direction.
func (e *Engine) Neighbors(nodeID string, direction Direction, kinds ...EdgeKind) []string {
	e.preloadEdges(nodeID)
	seen := make(map[string]struct{})
	out := make([]string, 0)
	allowed := kindSet(kinds)
	add := func(id string) {
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	e.store.mu.RLock()
	defer e.store.mu.RUnlock()
	if direction == "" || direction == DirectionOut || direction == DirectionBoth {
		for _, edge := range e.store.forward[nodeID] {
			if !edge.IsActive() || !matchKinds(edge.Kind, allowed) {
				continue
			}
			add(edge.TargetID)
		}
	}
	if direction == DirectionIn || direction == DirectionBoth {
		for _, edge := range e.store.reverse[nodeID] {
			if !edge.IsActive() || !matchKinds(edge.Kind, allowed) {
				continue
			}
			add(edge.SourceID)
		}
	}
	return out
}

func (e *Engine) edgesForDirectionRaw(nodeID string, direction Direction, allowed map[EdgeKind]struct{}) []EdgeRecord {
	if direction == "" || direction == DirectionOut {
		return filteredRawEdges(e.store.forward[nodeID], allowed)
	}
	if direction == DirectionIn {
		return filteredRawEdges(e.store.reverse[nodeID], allowed)
	}
	var edges []EdgeRecord
	edges = append(edges, filteredRawEdges(e.store.forward[nodeID], allowed)...)
	edges = append(edges, filteredRawEdges(e.store.reverse[nodeID], allowed)...)
	return edges
}

func filteredRawEdges(edges []EdgeRecord, allowed map[EdgeKind]struct{}) []EdgeRecord {
	if len(edges) == 0 {
		return nil
	}
	out := make([]EdgeRecord, 0, len(edges))
	for _, edge := range edges {
		if !edge.IsActive() || !matchKinds(edge.Kind, allowed) {
			continue
		}
		out = append(out, edge)
	}
	return out
}

func activeNode(node *NodeRecord) *NodeRecord {
	if node == nil || node.DeletedAt != 0 {
		return nil
	}
	return node
}

func cloneNodeForQuery(node *NodeRecord, includeProps bool) NodeRecord {
	out := cloneNode(node)
	if !includeProps {
		out.Props = nil
	}
	return out
}

func cloneEdgeForQuery(edge EdgeRecord, includeProps bool) EdgeRecord {
	out := cloneEdge(edge)
	if !includeProps {
		out.Props = nil
	}
	return out
}
