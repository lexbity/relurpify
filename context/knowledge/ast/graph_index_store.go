package ast

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/context/knowledge/graphdb"
)

const (
	AstFile_graph_index_store = "ast_file"
	AstNode_graph_index_store = "ast_node"
	Cat_graph_index_store = "cat:"
	EdgeId_graph_index_store = "edge_id"
	File_graph_index_store = "file:"
	Storeisclosed_graph_index_store = "store is closed"
	Type_graph_index_store = "type"
	Type_graph_index_store_2 = "type:"
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
	if s.g.IsClosed() {
		return errors.New(Storeisclosed_graph_index_store)
	}
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
		"indexed_at":     metadata.IndexedAt,
	})
	labels := []string{
		"lang:" + metadata.Language,
		Cat_graph_index_store + string(metadata.Category),
		File_graph_index_store + metadata.Path,
	}
	if metadata.ContentHash != "" {
		labels = append(labels, "hash:"+metadata.ContentHash)
	}

	return s.g.UpsertNode(context.TODO(), graphdb.NodeRecord{
		ID:       metadata.ID,
		Kind:     AstFile_graph_index_store,
		SourceID: metadata.Path,
		StableID: "path:" + metadata.Path,
		Labels:   labels,
		Props:    props,
	})
}

func (s *GraphIndexStore) GetFile(fileID string) (*FileMetadata, error) {
	if s.g.IsClosed() {
		return nil, errors.New(Storeisclosed_graph_index_store)
	}
	node, ok := s.g.GetNode(fileID)
	if !ok || node.Kind != AstFile_graph_index_store {
		return nil, os.ErrNotExist
	}
	return nodeToFileMetadata(node)
}

func (s *GraphIndexStore) GetFileByPath(path string) (*FileMetadata, error) {
	if s.g.IsClosed() {
		return nil, errors.New(Storeisclosed_graph_index_store)
	}
	// Use the stable ID which is "path:{path}".
	nodes := s.g.ListNodesByLabel(AstFile_graph_index_store, File_graph_index_store+path)
	if len(nodes) == 0 {
		return nil, os.ErrNotExist
	}
	return nodeToFileMetadata(nodes[0])
}

func (s *GraphIndexStore) ListFiles(category Category) ([]*FileMetadata, error) {
	if s.g.IsClosed() {
		return nil, errors.New(Storeisclosed_graph_index_store)
	}
	var nodes []graphdb.NodeRecord
	if category == "" {
		nodes = s.g.ListNodes(AstFile_graph_index_store)
	} else {
		nodes = s.g.ListNodesByLabel(AstFile_graph_index_store, Cat_graph_index_store+string(category))
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
	if s.g.IsClosed() {
		return errors.New(Storeisclosed_graph_index_store)
	}
	return s.g.DeleteNode(context.TODO(), fileID)
}

// ────────────────────────────────────────────────────────────────────
// Node operations
// ────────────────────────────────────────────────────────────────────

func (s *GraphIndexStore) SaveNodes(nodes []*Node) error {
	if s.g.IsClosed() {
		return errors.New(Storeisclosed_graph_index_store)
	}
	records := make([]graphdb.NodeRecord, 0, len(nodes))
	for _, n := range nodes {
		if n == nil {
			continue
		}
		records = append(records, nodeToRecord(n))
	}
	if len(records) == 0 {
		return nil
	}
	return s.g.UpsertNodes(context.TODO(), records)
}

func nodeToRecord(n *Node) graphdb.NodeRecord {
	props := mustMarshal(map[string]any{
		"parent_id":    n.ParentID,
		"file_id":      n.FileID,
		Type_graph_index_store:         string(n.Type),
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
		Type_graph_index_store_2 + string(n.Type),
		File_graph_index_store + n.FileID,
	}
	if n.Category != "" {
		labels = append(labels, Cat_graph_index_store+string(n.Category))
	}
	if n.Name != "" {
		labels = append(labels, "name:"+n.Name)
	}
	return graphdb.NodeRecord{
		ID:     n.ID,
		Kind:   AstNode_graph_index_store,
		Labels: labels,
		Props:  props,
	}
}

func (s *GraphIndexStore) GetNode(nodeID string) (*Node, error) {
	if s.g.IsClosed() {
		return nil, errors.New(Storeisclosed_graph_index_store)
	}
	node, ok := s.g.GetNode(nodeID)
	if !ok || node.Kind != AstNode_graph_index_store {
		return nil, os.ErrNotExist
	}
	return nodeToASTNode(node)
}

func (s *GraphIndexStore) GetNodesByFile(fileID string) ([]*Node, error) {
	if s.g.IsClosed() {
		return nil, errors.New(Storeisclosed_graph_index_store)
	}
	nodes := s.g.ListNodesByLabel(AstNode_graph_index_store, File_graph_index_store+fileID)
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
	if s.g.IsClosed() {
		return nil, errors.New(Storeisclosed_graph_index_store)
	}
	nodes := s.g.ListNodesByLabel(AstNode_graph_index_store, Type_graph_index_store_2+string(nodeType))
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
	if s.g.IsClosed() {
		return nil, errors.New(Storeisclosed_graph_index_store)
	}
	nodes := s.g.ListNodesByLabel(AstNode_graph_index_store, "name:"+name)
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
	if s.g.IsClosed() {
		return nil, errors.New(Storeisclosed_graph_index_store)
	}
	// Use the index to narrow results.
	var candidates []graphdb.NodeRecord
	if len(query.Types) > 0 {
		for _, t := range query.Types {
			candidates = append(candidates, s.g.ListNodesByLabel(AstNode_graph_index_store, Type_graph_index_store_2+string(t))...)
		}
	} else {
		candidates = s.g.ListNodes(AstNode_graph_index_store)
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

	// Deterministic sorting
	sort.Slice(out, func(i, j int) bool {
		return out[i].ID < out[j].ID
	})

	if query.Offset > 0 {
		if query.Offset >= len(out) {
			return nil, nil
		}
		out = out[query.Offset:]
	}
	if query.Limit > 0 && query.Limit < len(out) {
		out = out[:query.Limit]
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
		if !slices.Contains(q.Languages, n.Language) {
			return false
		}
	}
	if q.NamePattern != "" {
		if !matchName(n.Name, q.NamePattern) {
			return false
		}
	}
	if len(q.FileIDs) > 0 {
		if !slices.Contains(q.FileIDs, n.FileID) {
			return false
		}
	}
	if q.IsExported != nil && n.IsExported != *q.IsExported {
		return false
	}
	return true
}

func (s *GraphIndexStore) DeleteNode(nodeID string) error {
	return s.g.DeleteNode(context.TODO(), nodeID)
}

// ────────────────────────────────────────────────────────────────────
// Edge operations
// ────────────────────────────────────────────────────────────────────

func (s *GraphIndexStore) SaveEdges(edges []*Edge) error {
	if s.g.IsClosed() {
		return errors.New(Storeisclosed_graph_index_store)
	}
	records := make([]graphdb.EdgeRecord, 0, len(edges))
	for _, e := range edges {
		if e == nil {
			continue
		}
		records = append(records, edgeToRecord(e))
	}
	if len(records) == 0 {
		return nil
	}
	return s.g.LinkEdges(context.TODO(), records)
}

func edgeToRecord(e *Edge) graphdb.EdgeRecord {
	props := mustMarshal(e.Attributes)
	now := time.Now().UnixNano()
	return graphdb.EdgeRecord{
		SourceID:  e.SourceID,
		TargetID:  e.TargetID,
		Kind:      graphdb.EdgeKind(e.Type),
		Weight:    1,
		Props:     mustMarshal(map[string]any{EdgeId_graph_index_store: e.ID, Type_graph_index_store: string(e.Type), "props": string(props)}),
		CreatedAt: now,
	}
}

func (s *GraphIndexStore) GetEdge(edgeID string) (*Edge, error) {
	if s.g.IsClosed() {
		return nil, errors.New(Storeisclosed_graph_index_store)
	}
	// Find the edge by searching through connected nodes.
	// Edge IDs are stored in edge props, so we need to search.
	// For simplicity, iterate out edges from known nodes.
	// This is O(n)
	nodes := s.g.ListNodes("")
	for _, n := range nodes {
		edges := s.g.GetOutEdges(n.ID)
		for _, e := range edges {
			props := edgePropsToMap(e.Props)
			if id, _ := props[EdgeId_graph_index_store].(string); id == edgeID {
				return &Edge{
					ID:       edgeID,
					SourceID: e.SourceID,
					TargetID: e.TargetID,
					Type:     EdgeType(props[Type_graph_index_store].(string)),
				}, nil
			}
		}
	}
	return nil, os.ErrNotExist
}

func (s *GraphIndexStore) GetEdgesBySource(sourceID string) ([]*Edge, error) {
	if s.g.IsClosed() {
		return nil, errors.New(Storeisclosed_graph_index_store)
	}
	gedges := s.g.GetOutEdges(sourceID)
	return collectEdges(gedges)
}

func (s *GraphIndexStore) GetEdgesByTarget(targetID string) ([]*Edge, error) {
	if s.g.IsClosed() {
		return nil, errors.New(Storeisclosed_graph_index_store)
	}
	gedges := s.g.GetInEdges(targetID)
	return collectEdges(gedges)
}

func (s *GraphIndexStore) GetEdgesByType(edgeType EdgeType) ([]*Edge, error) {
	if s.g.IsClosed() {
		return nil, errors.New(Storeisclosed_graph_index_store)
	}
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
	if s.g.IsClosed() {
		return nil, errors.New(Storeisclosed_graph_index_store)
	}
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
	out, err := collectEdges(candidates)
	if err != nil {
		return nil, err
	}

	// Deterministic sorting
	sort.Slice(out, func(i, j int) bool {
		return out[i].ID < out[j].ID
	})

	if query.Offset > 0 {
		if query.Offset >= len(out) {
			return nil, nil
		}
		out = out[query.Offset:]
	}
	if query.Limit > 0 && query.Limit < len(out) {
		out = out[:query.Limit]
	}

	return out, nil
}

func (s *GraphIndexStore) DeleteEdge(edgeID string) error {
	if s.g.IsClosed() {
		return errors.New(Storeisclosed_graph_index_store)
	}
	edge, err := s.GetEdge(edgeID)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	if edge == nil {
		return nil
	}
	return s.g.Unlink(context.TODO(), edge.SourceID, edge.TargetID, graphdb.EdgeKind(edge.Type), true)
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
	return s.traverseNodesRecursive(nodeID, graphdb.DirectionOut)
}

func (s *GraphIndexStore) GetDependents(nodeID string) ([]*Node, error) {
	return s.traverseNodesRecursive(nodeID, graphdb.DirectionIn)
}

func (s *GraphIndexStore) traverseNodesRecursive(startNodeID string, dir graphdb.Direction) ([]*Node, error) {
	if s.g.IsClosed() {
		return nil, errors.New(Storeisclosed_graph_index_store)
	}
	visited := make(map[string]bool)
	var queue []string

	// Collect first-level neighbors of allowed types
	var firstLevel []string
	allowedKinds := []graphdb.EdgeKind{
		graphdb.EdgeKind(EdgeTypeImports),
		graphdb.EdgeKind(EdgeTypeDependsOn),
		graphdb.EdgeKind(EdgeTypeReferences),
	}
	for _, k := range allowedKinds {
		firstLevel = append(firstLevel, s.g.Neighbors(startNodeID, dir, k)...)
	}
	for _, id := range firstLevel {
		if !visited[id] {
			visited[id] = true
			queue = append(queue, id)
		}
	}

	// BFS
	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]

		var neighbors []string
		for _, k := range allowedKinds {
			neighbors = append(neighbors, s.g.Neighbors(curr, dir, k)...)
		}
		for _, nid := range neighbors {
			if !visited[nid] {
				visited[nid] = true
				queue = append(queue, nid)
			}
		}
	}

	// Convert IDs to Nodes
	out := make([]*Node, 0, len(visited))
	for id := range visited {
		n, err := s.GetNode(id)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, err
		}
		if n != nil {
			out = append(out, n)
		}
	}

	// Deterministic sorting to ensure stable tests
	sort.Slice(out, func(i, j int) bool {
		return out[i].ID < out[j].ID
	})

	return out, nil
}

func (s *GraphIndexStore) traverseNodes(nodeID string, edgeType EdgeType, dir graphdb.Direction) ([]*Node, error) {
	if s.g.IsClosed() {
		return nil, errors.New(Storeisclosed_graph_index_store)
	}
	neighborIDs := s.g.Neighbors(nodeID, dir, graphdb.EdgeKind(edgeType))
	out := make([]*Node, 0, len(neighborIDs))
	for _, nid := range neighborIDs {
		n, err := s.GetNode(nid)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
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
	if s.g.IsClosed() {
		return nil, errors.New(Storeisclosed_graph_index_store)
	}
	return &graphTransaction{store: s}, nil
}

func (s *GraphIndexStore) Vacuum() error {
	if s.g.IsClosed() {
		return errors.New(Storeisclosed_graph_index_store)
	}
	return nil
}

func (s *GraphIndexStore) GetStats() (*IndexStats, error) {
	if s.g.IsClosed() {
		return nil, errors.New(Storeisclosed_graph_index_store)
	}
	files := s.g.ListNodes(AstFile_graph_index_store)
	nodes := s.g.ListNodes(AstNode_graph_index_store)
	stats := &IndexStats{
		TotalFiles:      len(files),
		TotalNodes:      len(nodes),
		NodesByType:     make(map[NodeType]int),
		EdgesByType:     make(map[EdgeType]int),
		FilesByCategory: make(map[Category]int),
	}
	for _, n := range nodes {
		an, err := nodeToASTNode(n)
		if err == nil && an != nil {
			stats.NodesByType[an.Type]++
		}
	}
	for _, f := range files {
		fm, err := nodeToFileMetadata(f)
		if err == nil && fm != nil {
			stats.FilesByCategory[fm.Category]++
		}
	}

	var totalEdges int
	allNodes := s.g.ListNodes("")
	for _, n := range allNodes {
		gedges := s.g.GetOutEdges(n.ID)
		for _, e := range gedges {
			props := edgePropsToMap(e.Props)
			if et, ok := props[Type_graph_index_store].(string); ok {
				stats.EdgesByType[EdgeType(et)]++
				totalEdges++
			}
		}
	}
	stats.TotalEdges = totalEdges

	return stats, nil
}

// ────────────────────────────────────────────────────────────────────
// Internal helpers
// ────────────────────────────────────────────────────────────────────

type graphTransaction struct {
	store *GraphIndexStore
	nodes []*Node
	edges []*Edge
	files []string
}

func (t *graphTransaction) SaveNodes(nodes []*Node) error {
	t.nodes = append(t.nodes, nodes...)
	return nil
}

func (t *graphTransaction) SaveEdges(edges []*Edge) error {
	t.edges = append(t.edges, edges...)
	return nil
}

func (t *graphTransaction) DeleteFile(fileID string) error {
	t.files = append(t.files, fileID)
	return nil
}

func (t *graphTransaction) Commit() error {
	// Delete files first (separate from node/edge batched commits).
	for _, fileID := range t.files {
		if err := t.store.DeleteFile(fileID); err != nil {
			return err
		}
	}
	// Batch nodes and edges into single API calls.
	if len(t.nodes) > 0 {
		if err := t.store.SaveNodes(t.nodes); err != nil {
			return err
		}
	}
	if len(t.edges) > 0 {
		if err := t.store.SaveEdges(t.edges); err != nil {
			return err
		}
	}
	return nil
}

func (t *graphTransaction) Rollback() error {
	return nil
}

func mustMarshal(v any) json.RawMessage {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return data
}

func nodeToFileMetadata(n graphdb.NodeRecord) (*FileMetadata, error) {
	var props struct {
		Path         string    `json:"path"`
		RelativePath string    `json:"relative_path"`
		Language     string    `json:"language"`
		Category     string    `json:"category"`
		LineCount    int       `json:"line_count"`
		LOC          int       `json:"loc"`
		TokenCount   int       `json:"token_count"`
		Complexity   int       `json:"complexity"`
		ContentHash  string    `json:"content_hash"`
		Hash         string    `json:"hash"`
		RootNodeID   string    `json:"root_node_id"`
		NodeCount    int       `json:"node_count"`
		EdgeCount    int       `json:"edge_count"`
		ParserVer    string    `json:"parser_version"`
		Summary      string    `json:"summary"`
		SummaryHash  string    `json:"summary_hash"`
		Size         int64     `json:"size"`
		IndexedAt    time.Time `json:"indexed_at"`
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
		IndexedAt:     props.IndexedAt,
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
		ID:          n.ID,
		ParentID:    props.ParentID,
		FileID:      props.FileID,
		Type:        NodeType(props.NodeType),
		Category:    Category(props.Category),
		Language:    props.Language,
		StartLine:   props.StartLine,
		EndLine:     props.EndLine,
		StartCol:    props.StartCol,
		EndCol:      props.EndCol,
		Name:        props.Name,
		Signature:   props.Signature,
		DocString:   props.DocString,
		IsExported:  props.IsExported,
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
		edgeID, _ := props[EdgeId_graph_index_store].(string)
		et, _ := props[Type_graph_index_store].(string)
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
	// Convert wildcard pattern (e.g. "Hel%") to a Go regexp pattern.
	var regexStr strings.Builder
	regexStr.WriteString("(?i)^")
	for _, char := range pattern {
		switch char {
		case '%':
			regexStr.WriteString(".*")
		case '_':
			regexStr.WriteString(".")
		case '.', '+', '*', '?', '^', '$', '(', ')', '[', ']', '{', '}', '|', '\\':
			regexStr.WriteRune('\\')
			regexStr.WriteRune(char)
		default:
			regexStr.WriteRune(char)
		}
	}
	regexStr.WriteString("$")
	r, err := regexp.Compile(regexStr.String())
	if err != nil {
		return strings.Contains(strings.ToLower(name), strings.ToLower(pattern))
	}
	return r.MatchString(name)
}
