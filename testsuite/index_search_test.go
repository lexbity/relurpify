package testsuite

import (
	"errors"
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/context/knowledge/ast"
	"codeburg.org/lexbit/relurpify/context/knowledge/search"
)

type astCodeIndex struct {
	store ast.IndexStore
}

func (a *astCodeIndex) GetFileMetadata(string) (any, bool)                       { return nil, false }
func (a *astCodeIndex) ListFiles() []string                                      { return nil }
func (a *astCodeIndex) GetSymbolsByName(string) ([]search.SymbolLocation, error) { return nil, nil }
func (a *astCodeIndex) GetSymbolDefinition(string) (*search.SymbolLocation, error) {
	return nil, errors.New("mock not found")
}
func (a *astCodeIndex) GetSymbolReferences(string) ([]search.SymbolLocation, error) {
	return nil, nil
}
func (a *astCodeIndex) GetFileDependencies(string) []string                           { return nil }
func (a *astCodeIndex) GetDependents(string) []string                                 { return nil }
func (a *astCodeIndex) GetChunksForFile(string) []*search.CodeChunk                   { return nil }
func (a *astCodeIndex) GetChunkByID(string) (*search.CodeChunk, bool)                 { return nil, false }
func (a *astCodeIndex) FindChunksByName(string) []*search.CodeChunk                   { return nil }
func (a *astCodeIndex) FindChunksByFileAndRange(string, int, int) []*search.CodeChunk { return nil }
func (a *astCodeIndex) SearchChunks(query string, limit int) []*search.CodeChunk {
	nodes, err := a.store.SearchNodes(ast.NodeQuery{})
	if err != nil {
		return nil
	}
	query = strings.ToLower(query)
	results := make([]*search.CodeChunk, 0, len(nodes))
	seen := make(map[string]struct{})
	for _, node := range nodes {
		if node.Name == "" || !strings.Contains(strings.ToLower(node.Name), query) {
			continue
		}
		meta, err := a.store.GetFile(node.FileID)
		if err != nil || meta == nil {
			continue
		}
		chunkID := fmt.Sprintf("%s:%s:%d", node.FileID, node.Name, node.StartLine)
		if _, exists := seen[chunkID]; exists {
			continue
		}
		seen[chunkID] = struct{}{}
		lineSpan := node.EndLine - node.StartLine + 1
		if lineSpan <= 0 {
			lineSpan = 1
		}
		results = append(results, &search.CodeChunk{
			ID:         chunkID,
			File:       meta.Path,
			Kind:       search.ChunkFunction,
			Name:       node.Name,
			StartLine:  node.StartLine,
			EndLine:    node.EndLine,
			Summary:    node.DocString,
			Preview:    node.Name,
			TokenCount: lineSpan,
		})
		if limit > 0 && len(results) >= limit {
			break
		}
	}
	return results
}
