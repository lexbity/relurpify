package graphdb

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
)

// PageToken is an opaque, server-defined cursor used for paginated reads.
type PageToken string

// Page is the universal paginated read response. When Next is empty,
// the page is the last page and the result set is exhausted.
type Page[T any] struct {
	Items []T       `json:"items"`
	Next  PageToken `json:"next,omitempty"`
}

// ────────────────────────────────────────────────────────────────────
// Cursor — tracks the total number of elements returned so far.
// Each page runs the full BFS and slices the result window.
// ────────────────────────────────────────────────────────────────────

type cursorState struct {
	Offset int `json:"offset"`
}

func encodeCursor(c cursorState) PageToken {
	raw, _ := json.Marshal(c)
	return PageToken(base64.RawURLEncoding.EncodeToString(raw))
}

func decodeCursor(tok PageToken) (cursorState, error) {
	if tok == "" {
		return cursorState{}, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(string(tok))
	if err != nil {
		return cursorState{}, fmt.Errorf("graphdb: invalid page token: %w", err)
	}
	var c cursorState
	if err := json.Unmarshal(raw, &c); err != nil {
		return cursorState{}, fmt.Errorf("graphdb: invalid page token: %w", err)
	}
	return c, nil
}

// GraphPageQuery extends GraphQuery with pagination support.
type GraphPageQuery struct {
	GraphQuery
	PageSize int
	After    PageToken
}

// SubgraphPage returns a cursor-paginated subgraph.  Each page runs the
// full BFS, collects all reachable elements, and slices the requested
// window.  Memory is O(reachable set) for a single page; consecutive
// pages double memory because the BFS re-runs.  The cursor is a simple
// offset that tracks total emitted elements.
func (e *Engine) SubgraphPage(ctx context.Context, q GraphPageQuery) (Page[GraphElement], error) {
	if q.MaxDepth < 0 {
		return Page[GraphElement]{}, fmt.Errorf("graphdb: MaxDepth must be >= 0")
	}
	if q.Limit <= 0 {
		q.Limit = 100
	}
	pageSize := q.PageSize
	if pageSize <= 0 || pageSize > q.Limit {
		pageSize = q.Limit
	}
	if q.MaxEdges <= 0 {
		q.MaxEdges = defaultMaxEdges(q.Limit)
	}
	if err := ctx.Err(); err != nil {
		e.emitEvent(Event{Kind: EventTraversalCancelled, ErrorClass: err.Error()})
		return Page[GraphElement]{}, err
	}

	// Decode cursor offset.
	var offset int
	if tok := q.After; tok != "" {
		c, err := decodeCursor(tok)
		if err != nil {
			return Page[GraphElement]{}, err
		}
		offset = c.Offset
	}

	// Run the full BFS and collect all elements.
	all, limitReached, err := e.collectReachableElements(ctx, q)
	if err != nil {
		return Page[GraphElement]{}, err
	}

	// Slice the requested window.
	if offset >= len(all) {
		return Page[GraphElement]{}, nil
	}
	end := offset + pageSize
	if end > len(all) {
		end = len(all)
	}
	elements := all[offset:end]

	var next PageToken
	if end < len(all) || limitReached {
		next = encodeCursor(cursorState{Offset: end})
	}
	return Page[GraphElement]{Items: elements, Next: next}, nil
}

// collectReachableElements runs a BFS from the query roots and returns
// all reachable elements in deterministic BFS order.
func (e *Engine) collectReachableElements(ctx context.Context, q GraphPageQuery) ([]GraphElement, bool, error) {
	allowed := kindSet(q.EdgeKinds)
	nodeSet := make(map[string]struct{})
	edgeSet := make(map[string]struct{})
	var elements []GraphElement
	edgeCount := 0
	limitReached := false

	e.store.mu.RLock()
	defer e.store.mu.RUnlock()

	type state struct {
		id    string
		depth int
	}
	queue := make([]state, 0, len(q.RootIDs))
	visitedDepth := make(map[string]int)

	for _, root := range q.RootIDs {
		queue = append(queue, state{id: root, depth: 0})
	}

	// Emit root nodes.
	for _, root := range q.RootIDs {
		if _, ok := nodeSet[root]; !ok {
			if node := e.getNodeMaybe(root); node != nil {
				nodeSet[root] = struct{}{}
				elements = append(elements, GraphElement{Node: cloneNodeForQuery(node, q.IncludeProps)})
			}
		}
	}

	// BFS traversal.
traverse:
	for len(queue) > 0 {
		select {
		case <-ctx.Done():
			e.emitEvent(Event{Kind: EventTraversalCancelled, ErrorClass: ctx.Err().Error()})
			return nil, false, ctx.Err()
		default:
		}

		item := queue[0]
		queue = queue[1:]
		if d, ok := visitedDepth[item.id]; ok && d <= item.depth {
			continue
		}
		visitedDepth[item.id] = item.depth
		if item.depth >= q.MaxDepth {
			continue
		}
		for _, edge := range e.edgesForDirectionRaw(item.id, q.Direction, allowed) {
			key := string(edge.Kind) + "|" + edge.SourceID + "|" + edge.TargetID
			if _, ok := edgeSet[key]; !ok {
				edgeSet[key] = struct{}{}
				elements = append(elements, GraphElement{Edge: cloneEdgeForQuery(edge, q.IncludeProps)})
				edgeCount++
				if edgeCount >= q.MaxEdges {
					limitReached = true
					break traverse
				}
			}
			nextDepth := item.depth + 1
			if nextDepth <= q.MaxDepth {
				nextID := edge.TargetID
				if q.Direction == DirectionIn {
					nextID = edge.SourceID
				}
				queue = append(queue, state{id: nextID, depth: nextDepth})
			}
			for _, nodeID := range []string{edge.SourceID, edge.TargetID} {
				if _, ok := nodeSet[nodeID]; !ok {
					if node := e.getNodeMaybe(nodeID); node != nil {
						nodeSet[nodeID] = struct{}{}
						elements = append(elements, GraphElement{Node: cloneNodeForQuery(node, q.IncludeProps)})
					}
				}
			}
		}
	}

	return elements, limitReached, nil
}

// GraphElement is a single item in a paginated graph traversal response.
type GraphElement struct {
	Node NodeRecord `json:"node,omitempty"`
	Edge EdgeRecord `json:"edge,omitempty"`
}
