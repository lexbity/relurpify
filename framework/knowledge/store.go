package knowledge

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/framework/graphdb"
)

// nodeKindChunk keeps the legacy persisted node kind during the transition so
// new framework-native readers can work against existing graph data.
const nodeKindChunk graphdb.NodeKind = "bkc_chunk"

type edgeEnvelope struct {
	ID         EdgeID          `json:"id"`
	Meta       map[string]any  `json:"meta,omitempty"`
	Provenance ChunkProvenance `json:"provenance"`
	CreatedAt  time.Time       `json:"created_at"`
}

// ChunkStore persists artifacts and relationships in graphdb.
type ChunkStore struct {
	Graph *graphdb.Engine
}

func (s *ChunkStore) Save(chunk KnowledgeChunk) (*KnowledgeChunk, error) {
	if s == nil || s.Graph == nil {
		return nil, errors.New("knowledge: chunk store graph is required")
	}
	if chunk.ID == "" {
		return nil, errors.New("knowledge: chunk id is required")
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
		Labels:   chunkLabels(chunk),
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
		return nil, errors.New("knowledge: chunk store graph is required")
	}
	if edge.FromChunk == "" || edge.Kind == "" {
		return nil, errors.New("knowledge: from chunk and edge kind are required")
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

func (s *ChunkStore) FindByContentHash(hash string) ([]KnowledgeChunk, error) {
	if s == nil || s.Graph == nil || hash == "" {
		return nil, nil
	}
	nodes := s.Graph.ListNodesByLabel(nodeKindChunk, contentHashLabel(hash))
	return decodeChunks(nodes)
}

func (s *ChunkStore) FindByCoverageHash(hash string) ([]KnowledgeChunk, error) {
	if s == nil || s.Graph == nil || hash == "" {
		return nil, nil
	}
	nodes := s.Graph.ListNodesByLabel(nodeKindChunk, coverageHashLabel(hash))
	return decodeChunks(nodes)
}

func (s *ChunkStore) FindByFilePath(path string) ([]KnowledgeChunk, error) {
	if s == nil || s.Graph == nil || path == "" {
		return nil, nil
	}
	nodes := s.Graph.ListNodesByLabel(nodeKindChunk, filePathLabel(path))
	return decodeChunks(nodes)
}

func (s *ChunkStore) FindByFilePathPrefix(prefix string) ([]KnowledgeChunk, error) {
	if s == nil || s.Graph == nil || prefix == "" {
		return nil, nil
	}
	normalized := normalizeFilePath(prefix)
	if normalized == "" {
		return nil, nil
	}
	nodes := s.Graph.ListNodesByLabelPrefix(nodeKindChunk, filePathLabelPrefix(normalized))
	return decodeChunks(nodes)
}

func (s *ChunkStore) FindFreshByFilePath(path string) ([]KnowledgeChunk, error) {
	chunks, err := s.FindByFilePath(path)
	if err != nil || len(chunks) == 0 {
		return chunks, err
	}
	out := make([]KnowledgeChunk, 0, len(chunks))
	for _, chunk := range chunks {
		if chunk.Freshness == FreshnessValid {
			out = append(out, chunk)
		}
	}
	return out, nil
}

// NOTE: full scan — only use in migration or tooling.
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

// Tombstone marks a chunk as tombstoned and records the superseding chunk ID.
func (s *ChunkStore) Tombstone(id ChunkID, supersededBy ChunkID) error {
	if s == nil || s.Graph == nil {
		return errors.New("knowledge: chunk store graph is required")
	}
	if id == "" {
		return errors.New("knowledge: chunk id is required")
	}

	chunk, ok, err := s.LoadIncludingTombstoned(id)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("knowledge: chunk %q not found", id)
	}

	chunk.Tombstoned = true
	chunk.SupersededBy = supersededBy
	chunk.UpdatedAt = time.Now().UTC()

	_, err = s.Save(*chunk)
	return err
}

// LoadIncludingTombstoned loads a chunk regardless of tombstone status.
func (s *ChunkStore) LoadIncludingTombstoned(id ChunkID) (*KnowledgeChunk, bool, error) {
	// For now, this is equivalent to Load since we don't filter by tombstone status yet
	return s.Load(id)
}

// MarkStale marks chunks as stale with a reason.
func (s *ChunkStore) MarkStale(ids []ChunkID, reason string) error {
	if s == nil || s.Graph == nil {
		return errors.New("knowledge: chunk store graph is required")
	}
	if len(ids) == 0 {
		return nil
	}

	for _, id := range ids {
		chunk, ok, err := s.Load(id)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}

		chunk.Freshness = FreshnessStale
		chunk.UpdatedAt = time.Now().UTC()
		if chunk.Body.Fields == nil {
			chunk.Body.Fields = make(map[string]any)
		}
		chunk.Body.Fields["stale_reason"] = reason

		_, err = s.Save(*chunk)
		if err != nil {
			return err
		}
	}
	return nil
}

// MarkStaleByCoverageHash marks all chunks with the given coverage hash as stale.
func (s *ChunkStore) MarkStaleByCoverageHash(coverageHash string) error {
	if s == nil || s.Graph == nil {
		return errors.New("knowledge: chunk store graph is required")
	}
	if coverageHash == "" {
		return nil
	}

	chunks, err := s.FindByCoverageHash(coverageHash)
	if err != nil {
		return err
	}

	var ids []ChunkID
	for _, chunk := range chunks {
		ids = append(ids, chunk.ID)
	}

	return s.MarkStale(ids, "coverage_hash_changed")
}

func chunkLabels(chunk KnowledgeChunk) []string {
	labels := make([]string, 0, 3)
	if chunk.Freshness != "" {
		labels = append(labels, fmt.Sprintf("freshness:%s", chunk.Freshness))
	}
	if label := coverageHashLabel(chunk.CoverageHash); label != "" {
		labels = append(labels, label)
	}
	if label := contentHashLabel(chunk.ContentHash); label != "" {
		labels = append(labels, label)
	}
	if label := filePathLabelFromChunk(chunk); label != "" {
		labels = append(labels, label)
	}
	return uniqueChunkLabels(labels)
}

func coverageHashLabel(hash string) string {
	hash = strings.TrimSpace(hash)
	if hash == "" {
		return ""
	}
	return "coverage_hash:" + hash
}

func contentHashLabel(hash string) string {
	hash = strings.TrimSpace(hash)
	if hash == "" {
		return ""
	}
	return "content_hash:" + hash
}

func filePathLabelFromChunk(chunk KnowledgeChunk) string {
	if chunk.Body.Fields == nil {
		return ""
	}
	path, _ := chunk.Body.Fields["file_path"].(string)
	return filePathLabel(path)
}

func filePathLabel(path string) string {
	path = normalizeFilePath(path)
	if path == "" {
		return ""
	}
	return "file_path:" + path
}

func filePathLabelPrefix(prefix string) string {
	prefix = normalizeFilePath(prefix)
	if prefix == "" {
		return ""
	}
	return "file_path:" + prefix
}

func normalizeFilePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	return filepath.ToSlash(filepath.Clean(path))
}

func decodeChunks(nodes []graphdb.NodeRecord) ([]KnowledgeChunk, error) {
	if len(nodes) == 0 {
		return nil, nil
	}
	out := make([]KnowledgeChunk, 0, len(nodes))
	for _, node := range nodes {
		var chunk KnowledgeChunk
		if err := json.Unmarshal(node.Props, &chunk); err != nil {
			return nil, err
		}
		out = append(out, chunk)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].ID < out[j].ID
	})
	return out, nil
}

func uniqueChunkLabels(labels []string) []string {
	if len(labels) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(labels))
	out := make([]string, 0, len(labels))
	for _, label := range labels {
		label = strings.TrimSpace(label)
		if label == "" {
			continue
		}
		if _, ok := seen[label]; ok {
			continue
		}
		seen[label] = struct{}{}
		out = append(out, label)
	}
	return out
}
