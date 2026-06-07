package graphdb

import (
	"errors"
	"slices"
)

// ImpactSet returns breadth-first reachability from the origin IDs.
func (e *Engine) ImpactSet(originIDs []string, edgeKinds []EdgeKind, maxDepth int) ImpactResult {
	if maxDepth < 0 {
		maxDepth = 0
	}
	allowed := kindSet(edgeKinds)
	visited := make(map[string]struct{}, len(originIDs))
	byDepth := make(map[int][]string)
	type queueEntry struct {
		id    string
		depth int
	}
	queue := make([]queueEntry, 0, len(originIDs))
	for _, id := range originIDs {
		if _, ok := visited[id]; ok {
			continue
		}
		visited[id] = struct{}{}
		queue = append(queue, queueEntry{id: id, depth: 0})
		byDepth[0] = append(byDepth[0], id)
	}
	affected := make([]string, 0)

	e.store.mu.RLock()
	defer e.store.mu.RUnlock()

	for len(queue) > 0 {
		item := queue[0]
		queue = queue[1:]
		if item.depth >= maxDepth {
			continue
		}
		for _, edge := range e.store.forward[item.id] {
			if !edge.IsActive() || !matchKinds(edge.Kind, allowed) {
				continue
			}
			if _, ok := visited[edge.TargetID]; ok {
				continue
			}
			nextDepth := item.depth + 1
			visited[edge.TargetID] = struct{}{}
			queue = append(queue, queueEntry{id: edge.TargetID, depth: nextDepth})
			byDepth[nextDepth] = append(byDepth[nextDepth], edge.TargetID)
			affected = append(affected, edge.TargetID)
		}
	}

	return ImpactResult{
		OriginIDs: slices.Clone(originIDs),
		Affected:  affected,
		ByDepth:   byDepth,
	}
}

// FindPath returns the shortest path within maxDepth if one exists.
func (e *Engine) FindPath(sourceID, targetID string, kinds []EdgeKind, maxDepth int) (*PathResult, error) {
	if sourceID == "" || targetID == "" {
		return nil, errors.New("graphdb: source and target are required")
	}
	if sourceID == targetID {
		return &PathResult{Source: sourceID, Target: targetID, Path: []string{sourceID}}, nil
	}
	if maxDepth <= 0 {
		maxDepth = 1
	}
	allowed := kindSet(kinds)

	e.store.mu.RLock()
	defer e.store.mu.RUnlock()

	depthF := map[string]int{sourceID: 0}
	depthB := map[string]int{targetID: 0}
	prevNode := make(map[string]string)
	prevEdge := make(map[string]EdgeRecord)
	nextNode := make(map[string]string)
	nextEdge := make(map[string]EdgeRecord)
	frontierF := []string{sourceID}
	frontierB := []string{targetID}

	for len(frontierF) > 0 && len(frontierB) > 0 {
		if len(frontierF) <= len(frontierB) {
			next, meet, found := expandForwardFrontier(e.store.forward, allowed, frontierF, depthF, depthB, prevNode, prevEdge, maxDepth)
			if found {
				return buildBidirectionalPathResult(sourceID, targetID, meet, prevNode, prevEdge, nextNode, nextEdge), nil
			}
			frontierF = next
		} else {
			next, meet, found := expandBackwardFrontier(e.store.reverse, allowed, frontierB, depthB, depthF, nextNode, nextEdge, maxDepth)
			if found {
				return buildBidirectionalPathResult(sourceID, targetID, meet, prevNode, prevEdge, nextNode, nextEdge), nil
			}
			frontierB = next
		}
	}
	return nil, nil
}

func buildBidirectionalPathResult(sourceID, targetID, meet string, prevNode map[string]string, prevEdge map[string]EdgeRecord, nextNode map[string]string, nextEdge map[string]EdgeRecord) *PathResult {
	path := []string{meet}
	edges := make([]EdgeRecord, 0)

	current := meet
	for current != sourceID {
		edges = append(edges, cloneEdge(prevEdge[current]))
		current = prevNode[current]
		path = append(path, current)
	}
	slices.Reverse(path)
	slices.Reverse(edges)

	current = meet
	for current != targetID {
		edge := cloneEdge(nextEdge[current])
		edges = append(edges, edge)
		current = nextNode[current]
		path = append(path, current)
	}

	return &PathResult{
		Source: sourceID,
		Target: targetID,
		Path:   path,
		Edges:  edges,
	}
}

func expandForwardFrontier(forward map[string][]EdgeRecord, allowed map[EdgeKind]struct{}, frontier []string, depthSelf, depthOther map[string]int, prevNode map[string]string, prevEdge map[string]EdgeRecord, maxDepth int) ([]string, string, bool) {
	currentDepth := depthSelf[frontier[0]]
	if currentDepth >= maxDepth {
		return nil, "", false
	}
	nextFrontier := make([]string, 0, len(frontier)*2)
	for _, nodeID := range frontier {
		for _, edge := range forward[nodeID] {
			if !edge.IsActive() || !matchKinds(edge.Kind, allowed) {
				continue
			}
			nextDepth := depthSelf[nodeID] + 1
			if nextDepth > maxDepth {
				continue
			}
			if priorDepth, ok := depthSelf[edge.TargetID]; ok && priorDepth <= nextDepth {
				continue
			}
			depthSelf[edge.TargetID] = nextDepth
			prevNode[edge.TargetID] = nodeID
			prevEdge[edge.TargetID] = edge
			nextFrontier = append(nextFrontier, edge.TargetID)
			if otherDepth, ok := depthOther[edge.TargetID]; ok && nextDepth+otherDepth <= maxDepth {
				return nextFrontier, edge.TargetID, true
			}
		}
	}
	return nextFrontier, "", false
}

func expandBackwardFrontier(reverse map[string][]EdgeRecord, allowed map[EdgeKind]struct{}, frontier []string, depthSelf, depthOther map[string]int, nextNode map[string]string, nextEdge map[string]EdgeRecord, maxDepth int) ([]string, string, bool) {
	currentDepth := depthSelf[frontier[0]]
	if currentDepth >= maxDepth {
		return nil, "", false
	}
	nextFrontier := make([]string, 0, len(frontier)*2)
	for _, nodeID := range frontier {
		for _, edge := range reverse[nodeID] {
			if !edge.IsActive() || !matchKinds(edge.Kind, allowed) {
				continue
			}
			nextDepth := depthSelf[nodeID] + 1
			if nextDepth > maxDepth {
				continue
			}
			if priorDepth, ok := depthSelf[edge.SourceID]; ok && priorDepth <= nextDepth {
				continue
			}
			depthSelf[edge.SourceID] = nextDepth
			nextNode[edge.SourceID] = nodeID
			nextEdge[edge.SourceID] = edge
			nextFrontier = append(nextFrontier, edge.SourceID)
			if otherDepth, ok := depthOther[edge.SourceID]; ok && nextDepth+otherDepth <= maxDepth {
				return nextFrontier, edge.SourceID, true
			}
		}
	}
	return nextFrontier, "", false
}

// Neighbors returns depth-1 adjacent nodes in the requested direction.
func (e *Engine) Neighbors(nodeID string, direction Direction, kinds ...EdgeKind) []string {
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

// Subgraph returns nodes and edges reachable within maxDepth.
func (e *Engine) Subgraph(query GraphQuery) ([]NodeRecord, []EdgeRecord) {
	if query.MaxDepth < 0 {
		query.MaxDepth = 0
	}
	allowed := kindSet(query.EdgeKinds)
	nodeSet := make(map[string]struct{})
	edgeSet := make(map[string]struct{})
	nodes := make([]NodeRecord, 0)
	edges := make([]EdgeRecord, 0)
	type state struct {
		id    string
		depth int
	}
	queue := make([]state, 0, len(query.RootIDs))
	e.store.mu.RLock()
	defer e.store.mu.RUnlock()
	for _, root := range query.RootIDs {
		queue = append(queue, state{id: root, depth: 0})
		if _, ok := nodeSet[root]; !ok {
			if node := activeNode(e.store.nodes[root]); node != nil {
				nodeSet[root] = struct{}{}
				nodes = append(nodes, cloneNode(node))
			}
		}
	}
	visitedDepth := make(map[string]int)

	for len(queue) > 0 {
		item := queue[0]
		queue = queue[1:]
		if depth, ok := visitedDepth[item.id]; ok && depth <= item.depth {
			continue
		}
		visitedDepth[item.id] = item.depth
		if item.depth >= query.MaxDepth {
			continue
		}
		for _, edge := range e.edgesForDirectionRaw(item.id, query.Direction, allowed) {
			key := string(edge.Kind) + "|" + edge.SourceID + "|" + edge.TargetID
			if _, ok := edgeSet[key]; !ok {
				edgeSet[key] = struct{}{}
				edges = append(edges, cloneEdge(edge))
			}
			for _, nodeID := range []string{edge.SourceID, edge.TargetID} {
				if _, ok := nodeSet[nodeID]; !ok {
					if node := activeNode(e.store.nodes[nodeID]); node != nil {
						nodeSet[nodeID] = struct{}{}
						nodes = append(nodes, cloneNode(node))
					}
				}
			}
			nextID := edge.TargetID
			if query.Direction == DirectionIn {
				nextID = edge.SourceID
			}
			queue = append(queue, state{id: nextID, depth: item.depth + 1})
		}
	}
	return nodes, edges
}

func (e *Engine) edgesForDirectionRaw(nodeID string, direction Direction, allowed map[EdgeKind]struct{}) []EdgeRecord {
	if direction == "" || direction == DirectionOut {
		return filteredRawEdges(e.store.forward[nodeID], allowed)
	}
	if direction == DirectionIn {
		return filteredRawEdges(e.store.reverse[nodeID], allowed)
	}
	out := make([]EdgeRecord, 0, len(e.store.forward[nodeID])+len(e.store.reverse[nodeID]))
	out = append(out, filteredRawEdges(e.store.forward[nodeID], allowed)...)
	out = append(out, filteredRawEdges(e.store.reverse[nodeID], allowed)...)
	return out
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
