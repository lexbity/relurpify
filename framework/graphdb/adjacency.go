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
