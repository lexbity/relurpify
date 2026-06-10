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

	lruMaxCapacity int

	// O(1) LRU: lruMap stores linked-list nodes keyed by node ID;
	// lruHead/lruTail anchor the doubly-linked access-order list.
	lruMap  map[string]*lruEntry
	lruHead *lruEntry
	lruTail *lruEntry
}

type lruEntry struct {
	id   string
	prev *lruEntry
	next *lruEntry
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

// lruTouch moves id to the front (most recently used). O(1).
func (s *adjacencyStore) lruTouch(id string) {
	if s.lruMaxCapacity <= 0 {
		return
	}
	if s.lruMap == nil {
		s.lruMap = make(map[string]*lruEntry)
	}
	entry := s.lruMap[id]
	if entry == nil {
		entry = &lruEntry{id: id}
		s.lruMap[id] = entry
	}
	// Detach from current position.
	if entry.prev != nil {
		entry.prev.next = entry.next
	}
	if entry.next != nil {
		entry.next.prev = entry.prev
	}
	if s.lruHead == entry {
		s.lruHead = entry.next
	}
	if s.lruTail == entry {
		s.lruTail = entry.prev
	}
	// Move to front (most recently used).
	entry.prev = nil
	entry.next = s.lruHead
	if s.lruHead != nil {
		s.lruHead.prev = entry
	}
	s.lruHead = entry
	if s.lruTail == nil {
		s.lruTail = entry
	}
}

// lruEvict removes the least-recently-used entries until the working set
// is within lruMaxCapacity. O(evicted).
func (s *adjacencyStore) lruEvict() {
	if s.lruMaxCapacity <= 0 {
		return
	}
	for len(s.nodes) > s.lruMaxCapacity && s.lruTail != nil {
		oldest := s.lruTail
		s.lruTail = oldest.prev
		if s.lruTail != nil {
			s.lruTail.next = nil
		}
		if s.lruHead == oldest {
			s.lruHead = nil
		}
		delete(s.lruMap, oldest.id)
		delete(s.nodes, oldest.id)
		// Edge eviction happens lazily — forward/reverse maps are
		// populated on demand from the backend via preloadEdges.
		delete(s.forward, oldest.id)
		delete(s.reverse, oldest.id)
		// Label/source indexes are authoritative and built for all
		// nodes at boot.  They must NOT be touched on eviction —
		// removing entries would corrupt the index, causing silent
		// missing results in ListNodesByLabel/NodesBySource after
		// cache churn.
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
