package retrieval

import (
	"context"
	"path/filepath"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/ast"
	"codeburg.org/lexbit/relurpify/framework/knowledge"
)

// ASTProximityRanker prefers chunks that are close to the active file in the AST index.
type ASTProximityRanker struct {
	Index *ast.IndexManager
}

func (r *ASTProximityRanker) Name() string { return "ast_proximity" }

func (r *ASTProximityRanker) Rank(ctx context.Context, query RetrievalQuery, store *knowledge.ChunkStore) ([]knowledge.ChunkID, error) {
	_ = ctx
	if r == nil || r.Index == nil || store == nil {
		return nil, nil
	}
	scope := strings.TrimSpace(query.Scope)
	if scope == "" {
		return nil, nil
	}
	chunks, err := loadRankerChunks(store)
	if err != nil || len(chunks) == 0 {
		return nil, err
	}

	activeFiles, importedFiles := r.relatedFiles(scope)
	if len(activeFiles) == 0 {
		activeFiles[normalizeFileScope(scope)] = struct{}{}
	}

	scores := make(map[knowledge.ChunkID]float64, len(chunks))
	for _, chunk := range chunks {
		path := chunkFilePath(chunk)
		if path == "" {
			continue
		}
		switch {
		case hasFile(activeFiles, path):
			scores[chunk.ID] = 1.0
		case hasFile(importedFiles, path):
			scores[chunk.ID] = 0.8
		case sameFileDirectory(path, scope):
			scores[chunk.ID] = 0.6
		default:
			scores[chunk.ID] = 0.1
		}
	}

	ids := sortRankedIDs(scores, func(a, b knowledge.ChunkID) bool { return a < b })
	return ids, nil
}

func (r *ASTProximityRanker) relatedFiles(scope string) (map[string]struct{}, map[string]struct{}) {
	active := make(map[string]struct{})
	imported := make(map[string]struct{})
	scope = normalizeFileScope(scope)
	if scope == "" || r == nil || r.Index == nil {
		return active, imported
	}
	nodes, err := r.Index.SearchNodes(ast.NodeQuery{FileIDs: []string{scope}, Limit: 1000})
	if err != nil {
		return active, imported
	}
	for _, node := range nodes {
		if node == nil {
			continue
		}
		if node.FileID != "" {
			active[normalizeFileScope(node.FileID)] = struct{}{}
		}
		store := r.Index.Store()
		if store == nil {
			continue
		}
		imports, err := store.GetImports(node.ID)
		if err != nil {
			continue
		}
		for _, importedNode := range imports {
			if importedNode == nil || importedNode.FileID == "" {
				continue
			}
			imported[normalizeFileScope(importedNode.FileID)] = struct{}{}
		}
	}
	return active, imported
}

func chunkFilePath(chunk knowledge.KnowledgeChunk) string {
	if chunk.Body.Fields == nil {
		return ""
	}
	if path, ok := chunk.Body.Fields["file_path"].(string); ok {
		return normalizeFileScope(path)
	}
	return ""
}

func normalizeFileScope(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	return filepath.ToSlash(filepath.Clean(path))
}

func hasFile(set map[string]struct{}, path string) bool {
	_, ok := set[normalizeFileScope(path)]
	return ok
}

func sameFileDirectory(path, scope string) bool {
	path = normalizeFileScope(path)
	scope = normalizeFileScope(scope)
	if path == "" || scope == "" {
		return false
	}
	return filepath.Dir(path) == filepath.Dir(scope)
}
