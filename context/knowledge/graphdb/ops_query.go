package graphdb

import (
	"context"
	"errors"
	"math"
	"slices"
)

// ImpactSetContext returns breadth-first reachability from the origin IDs,
// bounded by maxDepth and limit. It returns ErrQueryLimitExceeded if the
// traversal hit limit before exhausting the graph.
func (e *Engine) ImpactSetContext(ctx context.Context, originIDs []string, edgeKinds []EdgeKind, maxDepth int, limit int) (ImpactResult, error) {
	if maxDepth < 0 {
		return ImpactResult{}, errors.New("graphdb: maxDepth must be >= 0")
	}
	if limit <= 0 {
		return ImpactResult{}, errors.New("graphdb: limit must be > 0")
	}
	if len(originIDs) == 0 {
		return ImpactResult{OriginIDs: originIDs}, nil
	}
	if err := ctx.Err(); err != nil {
		e.emitEvent(Event{Kind: EventTraversalCancelled, ErrorClass: err.Error()})
		return ImpactResult{}, err
	}

	allowed := kindSet(edgeKinds)
	visited := make(map[string]struct{}, len(originIDs))
	byDepth := make(map[int][]string)
	type queueEntry struct {
		id    string
		depth int
	}
	queue := make([]queueEntry, 0, len(originIDs))
	var affectedCount int
	limitReached := false

	for _, id := range originIDs {
		if _, ok := visited[id]; ok {
			continue
		}
		visited[id] = struct{}{}
		queue = append(queue, queueEntry{id: id, depth: 0})
		byDepth[0] = append(byDepth[0], id)
	}

	e.store.mu.RLock()
	defer e.store.mu.RUnlock()

traverse:
	for len(queue) > 0 {
		select {
		case <-ctx.Done():
			e.emitEvent(Event{Kind: EventTraversalCancelled, ErrorClass: ctx.Err().Error()})
			return ImpactResult{}, ctx.Err()
		default:
		}

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
			byDepth[nextDepth] = append(byDepth[nextDepth], edge.TargetID)
			affectedCount++
			if affectedCount >= limit {
				limitReached = true
				break traverse
			}
			queue = append(queue, queueEntry{id: edge.TargetID, depth: nextDepth})
		}
	}

	affected := make([]string, 0, affectedCount)
	for depth := 1; depth <= maxDepth; depth++ {
		affected = append(affected, byDepth[depth]...)
	}

	result := ImpactResult{
		OriginIDs: slices.Clone(originIDs),
		Affected:  affected,
		ByDepth:   byDepth,
	}
	if limitReached {
		e.emitEvent(Event{Kind: EventTraversalComplete, NodeCount: len(affected)})
		return result, ErrQueryLimitExceeded
	}
	e.emitEvent(Event{Kind: EventTraversalComplete, NodeCount: len(affected)})
	return result, nil
}

// ImpactSet is the legacy convenience wrapper. It uses context.Background
// and a large limit to preserve existing call‑site behaviour.
func (e *Engine) ImpactSet(originIDs []string, edgeKinds []EdgeKind, maxDepth int) ImpactResult {
	if maxDepth < 0 {
		maxDepth = 0
	}
	result, _ := e.ImpactSetContext(context.Background(), originIDs, edgeKinds, maxDepth, math.MaxInt32)
	return result
}

const ctxCheckInterval = 100

// FindPathContext returns the shortest path within maxDepth if one exists.
// It respects context cancellation during traversal.
func (e *Engine) FindPathContext(ctx context.Context, sourceID, targetID string, kinds []EdgeKind, maxDepth int) (*PathResult, error) {
	if sourceID == "" || targetID == "" {
		return nil, errors.New("graphdb: source and target are required")
	}
	if maxDepth < 0 {
		return nil, errors.New("graphdb: maxDepth must be >= 0")
	}
	if err := ctx.Err(); err != nil {
		e.emitEvent(Event{Kind: EventTraversalCancelled, ErrorClass: err.Error()})
		return nil, err
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
	steps := 0

	for len(frontierF) > 0 && len(frontierB) > 0 {
		steps++
		if steps%ctxCheckInterval == 0 {
			select {
			case <-ctx.Done():
				e.emitEvent(Event{Kind: EventTraversalCancelled, ErrorClass: ctx.Err().Error()})
				return nil, ctx.Err()
			default:
			}
		}

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

// FindPath is the legacy convenience wrapper.
func (e *Engine) FindPath(sourceID, targetID string, kinds []EdgeKind, maxDepth int) (*PathResult, error) {
	return e.FindPathContext(context.Background(), sourceID, targetID, kinds, maxDepth)
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

// SubgraphContext returns nodes and edges reachable within maxDepth, bounded
// by Limit and MaxEdges. It returns ErrQueryLimitExceeded if a budget was
// exhausted before the full graph was traversed.
func (e *Engine) SubgraphContext(ctx context.Context, query GraphQuery) ([]NodeRecord, []EdgeRecord, string, error) {
	if query.MaxDepth < 0 {
		return nil, nil, "", errors.New("graphdb: MaxDepth must be >= 0")
	}
	if query.Limit <= 0 {
		return nil, nil, "", errors.New("graphdb: Limit must be > 0")
	}
	if query.MaxEdges <= 0 {
		query.MaxEdges = defaultMaxEdges(query.Limit)
	}
	if err := ctx.Err(); err != nil {
		e.emitEvent(Event{Kind: EventTraversalCancelled, ErrorClass: err.Error()})
		return nil, nil, "", err
	}

	allowed := kindSet(query.EdgeKinds)
	nodeSet := make(map[string]struct{})
	edgeSet := make(map[string]struct{})
	nodes := make([]NodeRecord, 0)
	edges := make([]EdgeRecord, 0)
	edgeCount := 0
	limitReached := false

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
				if len(nodes) < query.Limit {
					nodes = append(nodes, cloneNodeForQuery(node, query.IncludeProps))
				}
			}
		}
	}
	visitedDepth := make(map[string]int)

traverse:
	for len(queue) > 0 {
		select {
		case <-ctx.Done():
			e.emitEvent(Event{Kind: EventTraversalCancelled, ErrorClass: ctx.Err().Error()})
			return nil, nil, "", ctx.Err()
		default:
		}

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
				edges = append(edges, cloneEdgeForQuery(edge, query.IncludeProps))
				edgeCount++
				if edgeCount >= query.MaxEdges {
					limitReached = true
					break traverse
				}
			}
			for _, nodeID := range []string{edge.SourceID, edge.TargetID} {
				if _, ok := nodeSet[nodeID]; !ok {
					if node := activeNode(e.store.nodes[nodeID]); node != nil {
						nodeSet[nodeID] = struct{}{}
						if len(nodes) < query.Limit {
							nodes = append(nodes, cloneNodeForQuery(node, query.IncludeProps))
						}
						if len(nodes) >= query.Limit {
							limitReached = true
							break traverse
						}
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

	if limitReached {
		e.emitEvent(Event{Kind: EventTraversalComplete, NodeCount: len(nodes), EdgeCount: len(edges)})
		return nodes, edges, "", ErrQueryLimitExceeded
	}
	e.emitEvent(Event{Kind: EventTraversalComplete, NodeCount: len(nodes), EdgeCount: len(edges)})
	return nodes, edges, "", nil
}

// Subgraph is the legacy convenience wrapper. It supplies a large default
// Limit when the caller leaves it at its zero value.
func (e *Engine) Subgraph(query GraphQuery) ([]NodeRecord, []EdgeRecord) {
	if query.Limit <= 0 {
		query.Limit = math.MaxInt32
	}
	nodes, edges, _, _ := e.SubgraphContext(context.Background(), query)
	return nodes, edges
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
