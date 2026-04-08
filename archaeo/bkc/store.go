package bkc

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/lexcodex/relurpify/framework/graphdb"
)

const nodeKindChunk graphdb.NodeKind = "bkc_chunk"

type edgeEnvelope struct {
	ID         EdgeID          `json:"id"`
	Meta       map[string]any  `json:"meta,omitempty"`
	Provenance ChunkProvenance `json:"provenance"`
	CreatedAt  time.Time       `json:"created_at"`
}

// ChunkStore persists chunks and relationships in graphdb.
type ChunkStore struct {
	Graph *graphdb.Engine
}

func (s *ChunkStore) Save(chunk KnowledgeChunk) (*KnowledgeChunk, error) {
	if s == nil || s.Graph == nil {
		return nil, errors.New("bkc: chunk store graph is required")
	}
	if chunk.ID == "" {
		return nil, errors.New("bkc: chunk id is required")
	}
	now := time.Now().UTC()
	if chunk.Freshness == "" {
		chunk.Freshness = FreshnessValid
	}
	if chunk.CreatedAt.IsZero() {
		chunk.CreatedAt = now
	}
	chunk.UpdatedAt = now
	if existing, ok, err := s.Load(chunk.ID); err != nil {
		return nil, err
	} else if ok {
		if chunk.Version <= existing.Version {
			chunk.Version = existing.Version + 1
		}
		if chunk.CreatedAt.IsZero() || chunk.CreatedAt.Before(existing.CreatedAt) {
			chunk.CreatedAt = existing.CreatedAt
		}
	} else if chunk.Version <= 0 {
		chunk.Version = 1
	}
	props, err := json.Marshal(chunk)
	if err != nil {
		return nil, err
	}
	if err := s.Graph.UpsertNode(graphdb.NodeRecord{
		ID:       string(chunk.ID),
		Kind:     nodeKindChunk,
		SourceID: chunk.WorkspaceID,
		Labels:   []string{fmt.Sprintf("freshness:%s", chunk.Freshness)},
		Props:    props,
	}); err != nil {
		return nil, err
	}
	return &chunk, nil
}

func (s *ChunkStore) Load(id ChunkID) (*KnowledgeChunk, bool, error) {
	if s == nil || s.Graph == nil || id == "" {
		return nil, false, nil
	}
	node, ok := s.Graph.GetNode(string(id))
	if !ok {
		return nil, false, nil
	}
	var chunk KnowledgeChunk
	if err := json.Unmarshal(node.Props, &chunk); err != nil {
		return nil, false, err
	}
	return &chunk, true, nil
}

func (s *ChunkStore) LoadMany(ids []ChunkID) ([]KnowledgeChunk, error) {
	if s == nil || s.Graph == nil || len(ids) == 0 {
		return nil, nil
	}
	out := make([]KnowledgeChunk, 0, len(ids))
	for _, id := range ids {
		chunk, ok, err := s.Load(id)
		if err != nil {
			return nil, err
		}
		if ok && chunk != nil {
			out = append(out, *chunk)
		}
	}
	return out, nil
}

func (s *ChunkStore) Delete(id ChunkID) error {
	if s == nil || s.Graph == nil || id == "" {
		return nil
	}
	return s.Graph.DeleteNode(string(id))
}

func (s *ChunkStore) SaveEdge(edge ChunkEdge) (*ChunkEdge, error) {
	if s == nil || s.Graph == nil {
		return nil, errors.New("bkc: chunk store graph is required")
	}
	if edge.FromChunk == "" || edge.Kind == "" {
		return nil, errors.New("bkc: from chunk and edge kind are required")
	}
	if edge.CreatedAt.IsZero() {
		edge.CreatedAt = time.Now().UTC()
	}
	if edge.ID == "" {
		edge.ID = defaultEdgeID(edge)
	}
	props, err := json.Marshal(edgeEnvelope{
		ID:         edge.ID,
		Meta:       cloneMap(edge.Meta),
		Provenance: edge.Provenance,
		CreatedAt:  edge.CreatedAt,
	})
	if err != nil {
		return nil, err
	}
	if err := s.Graph.LinkEdges([]graphdb.EdgeRecord{{
		SourceID:  string(edge.FromChunk),
		TargetID:  string(edge.ToChunk),
		Kind:      graphdb.EdgeKind(edge.Kind),
		Weight:    float32(edge.Weight),
		Props:     props,
		CreatedAt: edge.CreatedAt.UnixNano(),
	}}); err != nil {
		return nil, err
	}
	return &edge, nil
}

func (s *ChunkStore) LoadEdge(from, to ChunkID, kind EdgeKind) (*ChunkEdge, bool, error) {
	if s == nil || s.Graph == nil || from == "" || kind == "" {
		return nil, false, nil
	}
	for _, record := range s.Graph.GetOutEdges(string(from), graphdb.EdgeKind(kind)) {
		if ChunkID(record.TargetID) != to {
			continue
		}
		edge, err := decodeEdge(record)
		if err != nil {
			return nil, false, err
		}
		return &edge, true, nil
	}
	return nil, false, nil
}

func (s *ChunkStore) LoadEdgesFrom(from ChunkID, kinds ...EdgeKind) ([]ChunkEdge, error) {
	if s == nil || s.Graph == nil || from == "" {
		return nil, nil
	}
	graphKinds := make([]graphdb.EdgeKind, 0, len(kinds))
	for _, kind := range kinds {
		graphKinds = append(graphKinds, graphdb.EdgeKind(kind))
	}
	records := s.Graph.GetOutEdges(string(from), graphKinds...)
	out := make([]ChunkEdge, 0, len(records))
	for _, record := range records {
		edge, err := decodeEdge(record)
		if err != nil {
			return nil, err
		}
		out = append(out, edge)
	}
	return out, nil
}

func (s *ChunkStore) FindByCodeStateRef(codeStateRef string) ([]KnowledgeChunk, error) {
	return s.findMatching(func(chunk KnowledgeChunk) bool {
		return chunk.Provenance.CodeStateRef == codeStateRef
	})
}

func (s *ChunkStore) FindByWorkspace(workspaceID string) ([]KnowledgeChunk, error) {
	return s.findMatching(func(chunk KnowledgeChunk) bool {
		return chunk.WorkspaceID == workspaceID
	})
}

func (s *ChunkStore) FindAll() ([]KnowledgeChunk, error) {
	return s.findMatching(nil)
}

func (s *ChunkStore) findMatching(match func(KnowledgeChunk) bool) ([]KnowledgeChunk, error) {
	if s == nil || s.Graph == nil {
		return nil, nil
	}
	nodes := s.Graph.ListNodes(nodeKindChunk)
	out := make([]KnowledgeChunk, 0, len(nodes))
	for _, node := range nodes {
		var chunk KnowledgeChunk
		if err := json.Unmarshal(node.Props, &chunk); err != nil {
			return nil, err
		}
		if match == nil || match(chunk) {
			out = append(out, chunk)
		}
	}
	return out, nil
}

func decodeEdge(record graphdb.EdgeRecord) (ChunkEdge, error) {
	edge := ChunkEdge{
		FromChunk: ChunkID(record.SourceID),
		ToChunk:   ChunkID(record.TargetID),
		Kind:      EdgeKind(record.Kind),
		Weight:    float64(record.Weight),
	}
	var env edgeEnvelope
	if len(record.Props) > 0 {
		if err := json.Unmarshal(record.Props, &env); err != nil {
			return ChunkEdge{}, err
		}
		edge.ID = env.ID
		edge.Meta = cloneMap(env.Meta)
		edge.Provenance = env.Provenance
		edge.CreatedAt = env.CreatedAt
	}
	if edge.ID == "" {
		edge.ID = defaultEdgeID(edge)
	}
	if edge.CreatedAt.IsZero() && record.CreatedAt > 0 {
		edge.CreatedAt = time.Unix(0, record.CreatedAt).UTC()
	}
	return edge, nil
}

func defaultEdgeID(edge ChunkEdge) EdgeID {
	return EdgeID(fmt.Sprintf("%s:%s:%s", edge.FromChunk, edge.Kind, edge.ToChunk))
}

func cloneMap(in map[string]any) map[string]any {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
