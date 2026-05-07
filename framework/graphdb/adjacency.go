package graphdb

import (
	"slices"
	"sync"
)

type adjacencyStore struct {
	mu              sync.RWMutex
	nodes           map[string]*NodeRecord
	nodeHistory     map[string][]NodeRecord
	bySource        map[string]map[string]struct{}
	forward         map[string][]EdgeRecord
	reverse         map[string][]EdgeRecord
	edgeHistory     map[string][]EdgeRecord
	mutationResults map[string]MutationResult
	labels          *LabelIndex
}

func newAdjacencyStore() *adjacencyStore {
	return &adjacencyStore{
		nodes:           make(map[string]*NodeRecord),
		nodeHistory:     make(map[string][]NodeRecord),
		bySource:        make(map[string]map[string]struct{}),
		forward:         make(map[string][]EdgeRecord),
		reverse:         make(map[string][]EdgeRecord),
		edgeHistory:     make(map[string][]EdgeRecord),
		mutationResults: make(map[string]MutationResult),
		labels:          NewLabelIndex(),
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

func cloneNodeHistory(nodes []NodeRecord) []NodeRecord {
	if len(nodes) == 0 {
		return nil
	}
	out := make([]NodeRecord, len(nodes))
	for i := range nodes {
		out[i] = cloneNode(&nodes[i])
	}
	return out
}

func cloneEdgeHistory(edges []EdgeRecord) []EdgeRecord {
	if len(edges) == 0 {
		return nil
	}
	out := make([]EdgeRecord, len(edges))
	for i := range edges {
		out[i] = cloneEdge(edges[i])
	}
	return out
}

func cloneMutationResult(result MutationResult) MutationResult {
	out := result
	out.RecordIDs = slices.Clone(result.RecordIDs)
	out.CreatedIDs = slices.Clone(result.CreatedIDs)
	out.UpdatedIDs = slices.Clone(result.UpdatedIDs)
	out.AnnotatedIDs = slices.Clone(result.AnnotatedIDs)
	out.SupersededIDs = slices.Clone(result.SupersededIDs)
	out.MatchedIDs = slices.Clone(result.MatchedIDs)
	out.RejectedIDs = slices.Clone(result.RejectedIDs)
	out.ConflictIDs = slices.Clone(result.ConflictIDs)
	if result.Details != nil {
		out.Details = make(map[string]any, len(result.Details))
		for k, v := range result.Details {
			out.Details[k] = v
		}
	}
	return out
}

func cloneMutationResults(results map[string]MutationResult) map[string]MutationResult {
	if len(results) == 0 {
		return nil
	}
	out := make(map[string]MutationResult, len(results))
	for key, result := range results {
		out[key] = cloneMutationResult(result)
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

func edgeHistoryKey(sourceID, targetID string, kind EdgeKind) string {
	return string(kind) + "|" + sourceID + "|" + targetID
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
