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
// Cursor — encodes the full BFS frontier (queue + visited state) so
// each page resumes from where the prior page stopped, eliminating the
// O(P²) re-traversal from the roots.  Memory per page is O(frontier).
// ────────────────────────────────────────────────────────────────────

type cursorState struct {
	Queue         []stateEntry   `json:"q"`
	Visited       map[string]int `json:"v"`
	EdgeSet       map[string]int `json:"e"`
	NodeSet       map[string]int `json:"n"`
	EdgeCount     int            `json:"ec"`
	ResumeNode    string         `json:"rn,omitempty"`
	ResumeEdgeIdx int            `json:"re,omitempty"`
}

type stateEntry struct {
	ID    string `json:"i"`
	Depth int    `json:"d"`
}

func encodeCursor(c cursorState) PageToken {
	raw, err := json.Marshal(c)
	if err != nil {
		return PageToken("")
	}
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

// SubgraphPage returns a cursor-paginated subgraph.  The cursor encodes
// the BFS frontier so each page resumes in-place — O(P · frontier) over
// P pages, never O(P²).  Memory per page is O(frontier + pageSize).
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

	allowed := kindSet(q.EdgeKinds)
	var elements []GraphElement
	limitReached := false

	// ── Restore frontier from cursor, or initialise from roots ──
	var state cursorState
	if tok := q.After; tok != "" {
		var err error
		state, err = decodeCursor(tok)
		if err != nil {
			return Page[GraphElement]{}, err
		}
	} else {
		for _, root := range q.RootIDs {
			state.Queue = append(state.Queue, stateEntry{ID: root, Depth: 0})
		}
		state.Visited = make(map[string]int)
		state.EdgeSet = make(map[string]int)
		state.NodeSet = make(map[string]int)
	}

	// Preload edges for all nodes in the frontier so the traversal
	// under RLock can read them without backend calls.
	for _, item := range state.Queue {
		e.preloadEdges(item.ID)
	}
	if state.ResumeNode != "" {
		e.preloadEdges(state.ResumeNode)
	}

	e.store.mu.RLock()

	// If we have a resume node with unfinished edge iteration, push it to
	// the front so its remaining edges are processed before new nodes.
	if state.ResumeNode != "" {
		depth := state.Visited[state.ResumeNode]
		state.Queue = append([]stateEntry{{ID: state.ResumeNode, Depth: depth}}, state.Queue...)
	}

	// Emit queued nodes as GraphElements (first time they appear).
	for _, item := range state.Queue {
		if _, ok := state.NodeSet[item.ID]; !ok {
			if node := e.getNodeMaybe(item.ID); node != nil {
				state.NodeSet[item.ID] = 1
				elements = append(elements, GraphElement{Node: cloneNodeForQuery(node, q.IncludeProps)})
			}
		}
	}
	if len(elements) >= pageSize {
		e.store.mu.RUnlock()
		return pageResult(elements, state, true), nil
	}

	// ── Resume BFS from the frontier ──
traverse:
	for len(state.Queue) > 0 && len(elements) < pageSize {
		select {
		case <-ctx.Done():
			e.store.mu.RUnlock()
			e.emitEvent(Event{Kind: EventTraversalCancelled, ErrorClass: ctx.Err().Error()})
			return Page[GraphElement]{}, ctx.Err()
		default:
		}

		item := state.Queue[0]
		state.Queue = state.Queue[1:]

		// When this is the resume node, process its remaining edges even
		// if it was already visited (the resume was saved because the
		// previous page ran out of space mid-edge-iteration).
		isResume := state.ResumeNode == item.ID
		if !isResume {
			if d, ok := state.Visited[item.ID]; ok && d <= item.Depth {
				continue
			}
		}
		state.Visited[item.ID] = item.Depth
		if item.Depth >= q.MaxDepth {
			continue
		}
		edges := e.edgesForDirectionRaw(item.ID, q.Direction, allowed)
		startEdge := 0
		if isResume {
			startEdge = state.ResumeEdgeIdx
			state.ResumeNode = ""
			state.ResumeEdgeIdx = 0
		}
		for ei := startEdge; ei < len(edges) && len(elements) < pageSize; ei++ {
			edge := edges[ei]
			key := string(edge.Kind) + "|" + edge.SourceID + "|" + edge.TargetID
			if _, ok := state.EdgeSet[key]; !ok {
				state.EdgeSet[key] = 1
				elements = append(elements, GraphElement{Edge: cloneEdgeForQuery(edge, q.IncludeProps)})
				state.EdgeCount++
				if state.EdgeCount >= q.MaxEdges {
					limitReached = true
					break traverse
				}
			}
			nextDepth := item.Depth + 1
			if nextDepth <= q.MaxDepth {
				nextID := edge.TargetID
				if q.Direction == DirectionIn {
					nextID = edge.SourceID
				}
				state.Queue = append(state.Queue, stateEntry{ID: nextID, Depth: nextDepth})
			}
			for _, nodeID := range []string{edge.SourceID, edge.TargetID} {
				if _, ok := state.NodeSet[nodeID]; !ok {
					if node := e.getNodeMaybe(nodeID); node != nil {
						state.NodeSet[nodeID] = 1
						elements = append(elements, GraphElement{Node: cloneNodeForQuery(node, q.IncludeProps)})
						if len(elements) >= pageSize && state.EdgeCount < q.MaxEdges {
							state.ResumeNode = item.ID
							state.ResumeEdgeIdx = ei + 1
							break traverse
						}
					}
				}
			}
		}
	}

	e.store.mu.RUnlock()

	needMore := len(state.Queue) > 0 || limitReached
	return pageResult(elements, state, needMore), nil
}

func pageResult(elements []GraphElement, state cursorState, needMore bool) Page[GraphElement] {
	var next PageToken
	if needMore {
		next = encodeCursor(state)
	}
	return Page[GraphElement]{Items: elements, Next: next}
}

// GraphElement is a single item in a paginated graph traversal response.
type GraphElement struct {
	Node NodeRecord `json:"node,omitempty"`
	Edge EdgeRecord `json:"edge,omitempty"`
}
