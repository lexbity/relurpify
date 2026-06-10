package graphdb

import (
	"slices"
	"sync"
)

type adjacencyStore struct {
	mu       sync.RWMutex
	nodes    map[string]*NodeRecord
	bySource map[string]map[string]struct{}
	forward  map[string][]EdgeRecord
	reverse  map[string][]EdgeRecord
	labels   *LabelIndex

	// lruMaxCapacity limits the number of nodes kept in RAM. 0 means
	// unbounded (legacy full-hydration behaviour).
	lruMaxCapacity int
	// lruAccessOrder tracks node IDs from oldest-access to newest-access.
	// When len(nodes) exceeds lruMaxCapacity the oldest entries are
	// evicted. Only used when lruMaxCapacity > 0.
	lruAccessOrder []string
}

func newAdjacencyStore() *adjacencyStore {
	return &adjacencyStore{
		nodes:    make(map[string]*NodeRecord),
		bySource: make(map[string]map[string]struct{}),
		forward:  make(map[string][]EdgeRecord),
		reverse:  make(map[string][]EdgeRecord),
		labels:   NewLabelIndex(),
	}
}

// lruTouch records a node access. Must be called with mu write-locked.
func (s *adjacencyStore) lruTouch(id string) {
	if s.lruMaxCapacity <= 0 {
		return
	}
	// Move id to end (most recently used).
	for i, v := range s.lruAccessOrder {
		if v == id {
			s.lruAccessOrder = append(s.lruAccessOrder[:i], s.lruAccessOrder[i+1:]...)
			break
		}
	}
	s.lruAccessOrder = append(s.lruAccessOrder, id)
}

// lruEvict removes excess entries from the in-memory cache when the
// working set exceeds lruMaxCapacity. Must be called with mu write-locked.
func (s *adjacencyStore) lruEvict() {
	if s.lruMaxCapacity <= 0 {
		return
	}
	for len(s.nodes) > s.lruMaxCapacity && len(s.lruAccessOrder) > 0 {
		oldest := s.lruAccessOrder[0]
		s.lruAccessOrder = s.lruAccessOrder[1:]
		if n, ok := s.nodes[oldest]; ok && n != nil {
			if n.SourceID != "" {
				s.removeNodeSourceIndex(n.ID, n.SourceID)
			}
			s.removeNodeLabels(*n)
		}
		delete(s.nodes, oldest)
	}
}

func cloneNode(node *NodeRecord) NodeRecord {
	if node == nil {
		return NodeRecord{}
	}
	out := *node
	out.Labels = slices.Clone(node.Labels)
	out.Props = slices.Clone(node.Props)
	return out
}

func cloneEdge(edge EdgeRecord) EdgeRecord {
	edge.Props = slices.Clone(edge.Props)
	return edge
}

func cloneEdges(edges []EdgeRecord) []EdgeRecord {
	if len(edges) == 0 {
		return nil
	}
	out := make([]EdgeRecord, 0, len(edges))
	for _, edge := range edges {
		out = append(out, cloneEdge(edge))
	}
	return out
}

func nodeRecordEqual(a, b NodeRecord) bool {
	return a.Kind == b.Kind &&
		a.SourceID == b.SourceID &&
		a.StableID == b.StableID &&
		a.RevisionRootID == b.RevisionRootID &&
		a.RevisionOf == b.RevisionOf &&
		a.IdempotencyKey == b.IdempotencyKey &&
		a.TaskID == b.TaskID &&
		a.SessionID == b.SessionID &&
		a.TurnID == b.TurnID &&
		a.StateVersion == b.StateVersion &&
		slices.Equal(a.Labels, b.Labels) &&
		slices.Equal(a.Props, b.Props)
}

func edgeRecordEqual(a, b EdgeRecord) bool {
	return a.SourceID == b.SourceID &&
		a.TargetID == b.TargetID &&
		a.Kind == b.Kind &&
		a.StableID == b.StableID &&
		a.RevisionRootID == b.RevisionRootID &&
		a.RevisionOf == b.RevisionOf &&
		a.IdempotencyKey == b.IdempotencyKey &&
		a.TaskID == b.TaskID &&
		a.SessionID == b.SessionID &&
		a.TurnID == b.TurnID &&
		a.StateVersion == b.StateVersion &&
		a.Weight == b.Weight &&
		slices.Equal(a.Props, b.Props)
}

func (s *adjacencyStore) addNodeSourceIndex(node NodeRecord) {
	if s == nil || node.SourceID == "" || node.ID == "" {
		return
	}
	ids := s.bySource[node.SourceID]
	if ids == nil {
		ids = make(map[string]struct{})
		s.bySource[node.SourceID] = ids
	}
	ids[node.ID] = struct{}{}
}

func (s *adjacencyStore) removeNodeSourceIndex(nodeID, sourceID string) {
	if s == nil || sourceID == "" || nodeID == "" {
		return
	}
	ids := s.bySource[sourceID]
	if len(ids) == 0 {
		return
	}
	delete(ids, nodeID)
	if len(ids) == 0 {
		delete(s.bySource, sourceID)
	}
}

func (s *adjacencyStore) addNodeLabels(node NodeRecord) {
	if s == nil || s.labels == nil || node.ID == "" || node.DeletedAt != 0 {
		return
	}
	for _, label := range uniqueLabels(node.Labels) {
		s.labels.Add(label, node.ID)
	}
}

func (s *adjacencyStore) removeNodeLabels(node NodeRecord) {
	if s == nil || s.labels == nil || node.ID == "" {
		return
	}
	for _, label := range uniqueLabels(node.Labels) {
		s.labels.Remove(label, node.ID)
	}
}

func matchKinds(kind EdgeKind, allowed map[EdgeKind]struct{}) bool {
	return len(allowed) == 0 || hasKind(allowed, kind)
}

func hasKind(allowed map[EdgeKind]struct{}, kind EdgeKind) bool {
	_, ok := allowed[kind]
	return ok
}

func kindSet(kinds []EdgeKind) map[EdgeKind]struct{} {
	if len(kinds) == 0 {
		return nil
	}
	out := make(map[EdgeKind]struct{}, len(kinds))
	for _, kind := range kinds {
		out[kind] = struct{}{}
	}
	return out
}
