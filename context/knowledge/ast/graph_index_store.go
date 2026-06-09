package ast

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/context/knowledge/graphdb"
)

// GraphIndexStore implements IndexStore using a graphdb.Engine as the
// durable backend.  AST concepts are mapped to graph records:
//
//	FileMetadata  → node  kind "ast_file"
//	Node          → node  kind "ast_node"
//	Edge          → edge with kind matching the AST edge type
type GraphIndexStore struct {
	g *graphdb.Engine
}

// NewGraphIndexStore wraps an existing graphdb engine into an IndexStore.
func NewGraphIndexStore(g *graphdb.Engine) *GraphIndexStore {
	return &GraphIndexStore{g: g}
}

// ────────────────────────────────────────────────────────────────────
// File operations
// ────────────────────────────────────────────────────────────────────

func (s *GraphIndexStore) SaveFile(metadata *FileMetadata) error {
	if metadata == nil {
		return errors.New("metadata required")
	}
	props := mustMarshal(map[string]any{
		"path":           metadata.Path,
		"relative_path":  metadata.RelativePath,
		"language":       metadata.Language,
		"category":       string(metadata.Category),
		"line_count":     metadata.LineCount,
		"loc":            metadata.LOC,
		"token_count":    metadata.TokenCount,
		"complexity":     metadata.Complexity,
		"content_hash":   metadata.ContentHash,
		"hash":           metadata.Hash,
		"root_node_id":   metadata.RootNodeID,
		"node_count":     metadata.NodeCount,
		"edge_count":     metadata.EdgeCount,
		"parser_version": metadata.ParserVersion,
		"summary":        metadata.Summary,
		"summary_hash":   metadata.SummaryHash,
		"size":           metadata.Size,
	})
	labels := []string{
		"lang:" + metadata.Language,
		"cat:" + string(metadata.Category),
		"file:" + metadata.Path,
	}
	if metadata.ContentHash != "" {
		labels = append(labels, "hash:"+metadata.ContentHash)
	}

	return s.g.UpsertNode(graphdb.NodeRecord{
		ID:       metadata.ID,
		Kind:     "ast_file",
		SourceID: metadata.Path,
		StableID: "path:" + metadata.Path,
		Labels:   labels,
		Props:    props,
	})
}

func (s *GraphIndexStore) GetFile(fileID string) (*FileMetadata, error) {
	node, ok := s.g.GetNode(fileID)
	if !ok || node.Kind != "ast_file" {
		return nil, nil
	}
	return nodeToFileMetadata(node)
}

func (s *GraphIndexStore) GetFileByPath(path string) (*FileMetadata, error) {
	// Use the stable ID which is "path:{path}".
	nodes := s.g.ListNodesByLabel("ast_file", "file:"+path)
	if len(nodes) == 0 {
		return nil, nil
	}
	return nodeToFileMetadata(nodes[0])
}

func (s *GraphIndexStore) ListFiles(category Category) ([]*FileMetadata, error) {
	var nodes []graphdb.NodeRecord
	if category == "" {
		nodes = s.g.ListNodes("ast_file")
	} else {
		nodes = s.g.ListNodesByLabel("ast_file", "cat:"+string(category))
	}
	out := make([]*FileMetadata, 0, len(nodes))
	for _, n := range nodes {
		m, err := nodeToFileMetadata(n)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, nil
}

func (s *GraphIndexStore) DeleteFile(fileID string) error {
	return s.g.DeleteNode(fileID)
}

// ────────────────────────────────────────────────────────────────────
// Node operations
// ────────────────────────────────────────────────────────────────────

func (s *GraphIndexStore) SaveNodes(nodes []*Node) error {
	for _, n := range nodes {
		if err := s.saveNode(n); err != nil {
			return err
		}
	}
	return nil
}

func (s *GraphIndexStore) saveNode(n *Node) error {
	props := mustMarshal(map[string]any{
		"parent_id":    n.ParentID,
		"file_id":      n.FileID,
		"type":         string(n.Type),
		"category":     string(n.Category),
		"language":     n.Language,
		"start_line":   n.StartLine,
		"end_line":     n.EndLine,
		"start_col":    n.StartCol,
		"end_col":      n.EndCol,
		"name":         n.Name,
		"signature":    n.Signature,
		"doc_string":   n.DocString,
		"is_exported":  n.IsExported,
		"content_hash": n.ContentHash,
	})
	labels := []string{
		"type:" + string(n.Type),
		"file:" + n.FileID,
	}
	if n.Category != "" {
		labels = append(labels, "cat:"+string(n.Category))
	}
	if n.Name != "" {
		labels = append(labels, "name:"+n.Name)
	}

	return s.g.UpsertNode(graphdb.NodeRecord{
		ID:     n.ID,
		Kind:   "ast_node",
		Labels: labels,
		Props:  props,
	})
}

func (s *GraphIndexStore) GetNode(nodeID string) (*Node, error) {
	node, ok := s.g.GetNode(nodeID)
	if !ok || node.Kind != "ast_node" {
		return nil, nil
	}
	return nodeToASTNode(node)
}

func (s *GraphIndexStore) GetNodesByFile(fileID string) ([]*Node, error) {
	nodes := s.g.ListNodesByLabel("ast_node", "file:"+fileID)
	out := make([]*Node, 0, len(nodes))
	for _, n := range nodes {
		an, err := nodeToASTNode(n)
		if err != nil {
			return nil, err
		}
		out = append(out, an)
	}
	return out, nil
}

func (s *GraphIndexStore) GetNodesByType(nodeType NodeType) ([]*Node, error) {
	nodes := s.g.ListNodesByLabel("ast_node", "type:"+string(nodeType))
	out := make([]*Node, 0, len(nodes))
	for _, n := range nodes {
		an, err := nodeToASTNode(n)
		if err != nil {
			return nil, err
		}
		out = append(out, an)
	}
	return out, nil
}

func (s *GraphIndexStore) GetNodesByName(name string) ([]*Node, error) {
	nodes := s.g.ListNodesByLabel("ast_node", "name:"+name)
	out := make([]*Node, 0, len(nodes))
	for _, n := range nodes {
		an, err := nodeToASTNode(n)
		if err != nil {
			return nil, err
		}
		out = append(out, an)
	}
	return out, nil
}

func (s *GraphIndexStore) SearchNodes(query NodeQuery) ([]*Node, error) {
	// Use the index to narrow results.
	var candidates []graphdb.NodeRecord
	if len(query.Types) > 0 {
		for _, t := range query.Types {
			candidates = append(candidates, s.g.ListNodesByLabel("ast_node", "type:"+string(t))...)
		}
	} else {
		candidates = s.g.ListNodes("ast_node")
	}

	out := make([]*Node, 0, len(candidates))
	for _, n := range candidates {
		an, err := nodeToASTNode(n)
		if err != nil {
			return nil, err
		}
		if queryMatches(an, query) {
			out = append(out, an)
		}
	}
	return out, nil
}

func queryMatches(n *Node, q NodeQuery) bool {
	if len(q.Types) > 0 {
		if !containsNodeType(q.Types, n.Type) {
			return false
		}
	}
	if len(q.Categories) > 0 {
		if !containsCategory(q.Categories, n.Category) {
			return false
		}
	}
	if len(q.Languages) > 0 {
		if !containsString(q.Languages, n.Language) {
			return false
		}
	}
	if q.NamePattern != "" {
		if !matchName(n.Name, q.NamePattern) {
			return false
		}
	}
	if len(q.FileIDs) > 0 {
		if !containsString(q.FileIDs, n.FileID) {
			return false
		}
	}
	if q.IsExported != nil && n.IsExported != *q.IsExported {
		return false
	}
	return true
}

func (s *GraphIndexStore) DeleteNode(nodeID string) error {
	return s.g.DeleteNode(nodeID)
}

// ────────────────────────────────────────────────────────────────────
// Edge operations
// ────────────────────────────────────────────────────────────────────

func (s *GraphIndexStore) SaveEdges(edges []*Edge) error {
	for _, e := range edges {
		if err := s.saveEdge(e); err != nil {
			return err
		}
	}
	return nil
}

func (s *GraphIndexStore) saveEdge(e *Edge) error {
	props := mustMarshal(e.Attributes)
	return s.g.Link(e.SourceID, e.TargetID, graphdb.EdgeKind(e.Type), "", 1, map[string]any{
		"edge_id": e.ID,
		"type":    string(e.Type),
		"props":   string(props),
	})
}

func (s *GraphIndexStore) GetEdge(edgeID string) (*Edge, error) {
	// Find the edge by searching through connected nodes.
	// Edge IDs are stored in edge props, so we need to search.
	// For simplicity, iterate out edges from known nodes.
	// This is O(n) but the SQLite version is also O(1) via PK lookup.
	nodes := s.g.ListNodes("")
	for _, n := range nodes {
		edges := s.g.GetOutEdges(n.ID)
		for _, e := range edges {
			props := edgePropsToMap(e.Props)
			if id, _ := props["edge_id"].(string); id == edgeID {
				return &Edge{
					ID:       edgeID,
					SourceID: e.SourceID,
					TargetID: e.TargetID,
					Type:     EdgeType(props["type"].(string)),
				}, nil
			}
		}
	}
	return nil, nil
}

func (s *GraphIndexStore) GetEdgesBySource(sourceID string) ([]*Edge, error) {
	gedges := s.g.GetOutEdges(sourceID)
	return collectEdges(gedges)
}

func (s *GraphIndexStore) GetEdgesByTarget(targetID string) ([]*Edge, error) {
	gedges := s.g.GetInEdges(targetID)
	return collectEdges(gedges)
}

func (s *GraphIndexStore) GetEdgesByType(edgeType EdgeType) ([]*Edge, error) {
	// Walk all nodes and collect edges matching the type.
	var out []*Edge
	nodes := s.g.ListNodes("")
	for _, n := range nodes {
		gedges := s.g.GetOutEdges(n.ID, graphdb.EdgeKind(edgeType))
		ee, err := collectEdges(gedges)
		if err != nil {
			return nil, err
		}
		out = append(out, ee...)
	}
	return out, nil
}

func (s *GraphIndexStore) SearchEdges(query EdgeQuery) ([]*Edge, error) {
	var candidates []graphdb.EdgeRecord
	egk := toEdgeKinds(query.Types)
	if len(query.SourceIDs) > 0 {
		for _, sid := range query.SourceIDs {
			candidates = append(candidates, s.g.GetOutEdges(sid, egk...)...)
		}
	} else if len(query.TargetIDs) > 0 {
		for _, tid := range query.TargetIDs {
			candidates = append(candidates, s.g.GetInEdges(tid, egk...)...)
		}
	} else if len(query.Types) > 0 {
		nodes := s.g.ListNodes("")
		for _, n := range nodes {
			candidates = append(candidates, s.g.GetOutEdges(n.ID, egk...)...)
		}
	} else {
		nodes := s.g.ListNodes("")
		for _, n := range nodes {
			candidates = append(candidates, s.g.GetOutEdges(n.ID)...)
		}
	}
	return collectEdges(candidates)
}

func (s *GraphIndexStore) DeleteEdge(edgeID string) error {
	edge, err := s.GetEdge(edgeID)
	if err != nil || edge == nil {
		return err
	}
	return s.g.Unlink(edge.SourceID, edge.TargetID, graphdb.EdgeKind(edge.Type), true)
}

// ────────────────────────────────────────────────────────────────────
// Traversal helpers
// ────────────────────────────────────────────────────────────────────

func (s *GraphIndexStore) GetCallees(nodeID string) ([]*Node, error) {
	return s.traverseNodes(nodeID, EdgeTypeCalls, graphdb.DirectionOut)
}

func (s *GraphIndexStore) GetCallers(nodeID string) ([]*Node, error) {
	return s.traverseNodes(nodeID, EdgeTypeCalls, graphdb.DirectionIn)
}

func (s *GraphIndexStore) GetImports(nodeID string) ([]*Node, error) {
	return s.traverseNodes(nodeID, EdgeTypeImports, graphdb.DirectionOut)
}

func (s *GraphIndexStore) GetImportedBy(nodeID string) ([]*Node, error) {
	return s.traverseNodes(nodeID, EdgeTypeImports, graphdb.DirectionIn)
}

func (s *GraphIndexStore) GetReferences(nodeID string) ([]*Node, error) {
	return s.traverseNodes(nodeID, EdgeTypeReferences, graphdb.DirectionOut)
}

func (s *GraphIndexStore) GetReferencedBy(nodeID string) ([]*Node, error) {
	return s.traverseNodes(nodeID, EdgeTypeReferences, graphdb.DirectionIn)
}

func (s *GraphIndexStore) GetDependencies(nodeID string) ([]*Node, error) {
	return s.traverseNodes(nodeID, EdgeTypeImports, graphdb.DirectionOut)
}

func (s *GraphIndexStore) GetDependents(nodeID string) ([]*Node, error) {
	return s.traverseNodes(nodeID, EdgeTypeImports, graphdb.DirectionIn)
}

func (s *GraphIndexStore) traverseNodes(nodeID string, edgeType EdgeType, dir graphdb.Direction) ([]*Node, error) {
	neighborIDs := s.g.Neighbors(nodeID, dir, graphdb.EdgeKind(edgeType))
	out := make([]*Node, 0, len(neighborIDs))
	for _, nid := range neighborIDs {
		n, err := s.GetNode(nid)
		if err != nil {
			return nil, err
		}
		if n != nil {
			out = append(out, n)
		}
	}
	return out, nil
}

// ────────────────────────────────────────────────────────────────────
// Transaction / lifecycle
// ────────────────────────────────────────────────────────────────────

func (s *GraphIndexStore) BeginTransaction() (Transaction, error) {
	return noopTransaction{}, nil
}

func (s *GraphIndexStore) Vacuum() error {
	return nil
}

func (s *GraphIndexStore) GetStats() (*IndexStats, error) {
	files := s.g.ListNodes("ast_file")
	nodes := s.g.ListNodes("ast_node")
	stats := &IndexStats{
		TotalFiles: len(files),
		TotalNodes: len(nodes),
	}
	return stats, nil
}

// ────────────────────────────────────────────────────────────────────
// Internal helpers
// ────────────────────────────────────────────────────────────────────

type noopTransaction struct{}

func (noopTransaction) SaveNodes(_ []*Node) error    { return nil }
func (noopTransaction) SaveEdges(_ []*Edge) error     { return nil }
func (noopTransaction) DeleteFile(_ string) error     { return nil }
func (noopTransaction) Commit() error                 { return nil }
func (noopTransaction) Rollback() error               { return nil }

func mustMarshal(v any) json.RawMessage {
	data, _ := json.Marshal(v)
	return data
}

func nodeToFileMetadata(n graphdb.NodeRecord) (*FileMetadata, error) {
	var props struct {
		Path         string `json:"path"`
		RelativePath string `json:"relative_path"`
		Language     string `json:"language"`
		Category     string `json:"category"`
		LineCount    int    `json:"line_count"`
		LOC          int    `json:"loc"`
		TokenCount   int    `json:"token_count"`
		Complexity   int    `json:"complexity"`
		ContentHash  string `json:"content_hash"`
		Hash         string `json:"hash"`
		RootNodeID   string `json:"root_node_id"`
		NodeCount    int    `json:"node_count"`
		EdgeCount    int    `json:"edge_count"`
		ParserVer    string `json:"parser_version"`
		Summary      string `json:"summary"`
		SummaryHash  string `json:"summary_hash"`
		Size         int64  `json:"size"`
	}
	if err := json.Unmarshal(n.Props, &props); err != nil {
		return nil, fmt.Errorf("unmarshal file metadata: %w", err)
	}
	return &FileMetadata{
		ID:            n.ID,
		Path:          props.Path,
		RelativePath:  props.RelativePath,
		Language:      props.Language,
		Category:      Category(props.Category),
		LineCount:     props.LineCount,
		LOC:           props.LOC,
		TokenCount:    props.TokenCount,
		Complexity:    props.Complexity,
		ContentHash:   props.ContentHash,
		Hash:          props.Hash,
		RootNodeID:    props.RootNodeID,
		NodeCount:     props.NodeCount,
		EdgeCount:     props.EdgeCount,
		ParserVersion: props.ParserVer,
		Summary:       props.Summary,
		SummaryHash:   props.SummaryHash,
		Size:          props.Size,
	}, nil
}

func nodeToASTNode(n graphdb.NodeRecord) (*Node, error) {
	var props struct {
		ParentID    string `json:"parent_id"`
		FileID      string `json:"file_id"`
		NodeType    string `json:"type"`
		Category    string `json:"category"`
		Language    string `json:"language"`
		StartLine   int    `json:"start_line"`
		EndLine     int    `json:"end_line"`
		StartCol    int    `json:"start_col"`
		EndCol      int    `json:"end_col"`
		Name        string `json:"name"`
		Signature   string `json:"signature"`
		DocString   string `json:"doc_string"`
		IsExported  bool   `json:"is_exported"`
		ContentHash string `json:"content_hash"`
	}
	if err := json.Unmarshal(n.Props, &props); err != nil {
		return nil, fmt.Errorf("unmarshal ast node: %w", err)
	}
	return &Node{
		ID:         n.ID,
		ParentID:   props.ParentID,
		FileID:     props.FileID,
		Type:       NodeType(props.NodeType),
		Category:   Category(props.Category),
		Language:   props.Language,
		StartLine:  props.StartLine,
		EndLine:    props.EndLine,
		StartCol:   props.StartCol,
		EndCol:     props.EndCol,
		Name:       props.Name,
		Signature:  props.Signature,
		DocString:  props.DocString,
		IsExported: props.IsExported,
		ContentHash: props.ContentHash,
	}, nil
}

func edgePropsToMap(raw json.RawMessage) map[string]any {
	m := make(map[string]any)
	_ = json.Unmarshal(raw, &m)
	return m
}

func collectEdges(gedges []graphdb.EdgeRecord) ([]*Edge, error) {
	out := make([]*Edge, 0, len(gedges))
	for _, e := range gedges {
		props := edgePropsToMap(e.Props)
		edgeID, _ := props["edge_id"].(string)
		et, _ := props["type"].(string)
		out = append(out, &Edge{
			ID:       edgeID,
			SourceID: e.SourceID,
			TargetID: e.TargetID,
			Type:     EdgeType(et),
		})
	}
	return out, nil
}

func containsNodeType(slice []NodeType, val NodeType) bool {
	for _, s := range slice {
		if s == val {
			return true
		}
	}
	return false
}

func containsCategory(slice []Category, val Category) bool {
	for _, s := range slice {
		if s == val {
			return true
		}
	}
	return false
}

func containsString(slice []string, val string) bool {
	for _, s := range slice {
		if s == val {
			return true
		}
	}
	return false
}

func toEdgeKinds(ets []EdgeType) []graphdb.EdgeKind {
	out := make([]graphdb.EdgeKind, len(ets))
	for i, et := range ets {
		out[i] = graphdb.EdgeKind(et)
	}
	return out
}

func matchName(name, pattern string) bool {
	if pattern == "" || name == "" {
		return false
	}
	return strings.Contains(name, pattern) || strings.HasPrefix(name, pattern)
}
