package testsuite

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/ast"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/search"
)

func TestIndexManagerSearchIntegration(t *testing.T) {
	temp := t.TempDir()
	goFile := filepath.Join(temp, "service.go")
	goSource := `package service

func HighlightFeature() string {
    return "integration"
}
`
	if err := os.WriteFile(goFile, []byte(goSource), 0o644); err != nil {
		t.Fatalf("write go file: %v", err)
	}
	mdFile := filepath.Join(temp, "README.md")
	mdSource := "# Notes\n\nDocumenting HighlightFeature."
	if err := os.WriteFile(mdFile, []byte(mdSource), 0o644); err != nil {
		t.Fatalf("write markdown: %v", err)
	}

	store, err := ast.NewSQLiteStore(filepath.Join(temp, "index.db"))
	if err != nil {
		t.Fatalf("sqlite init failed: %v", err)
	}
	defer store.Close()
	manager := ast.NewIndexManager(store, ast.IndexConfig{WorkspacePath: temp})
	if err := manager.IndexWorkspace(); err != nil {
		t.Fatalf("IndexWorkspace failed: %v", err)
	}

	codeIndex := &astCodeIndex{store: store}
	engine := search.NewSearchEngine(nil, codeIndex)
	results, err := engine.Search(search.SearchQuery{Text: "highlight", Mode: search.SearchHybrid, MaxResults: 3})
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected AST-backed search results")
	}

	shared := contextdata.NewEnvelope("index-search", "index-search")
	target := results[0]
	data, err := os.ReadFile(target.File)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	shared.SetWorkingValue(target.File, string(data), contextdata.MemoryClassTask)
	if val, ok := shared.GetWorkingValue(target.File); !ok || val != string(data) {
		t.Fatalf("shared context missing %s", target.File)
	}
}

type astCodeIndex struct {
	store *ast.SQLiteStore
}

func (a *astCodeIndex) GetFileMetadata(string) (any, bool)                     { return nil, false }
func (a *astCodeIndex) ListFiles() []string                                    { return nil }
func (a *astCodeIndex) GetSymbolsByName(string) ([]search.SymbolLocation, error) { return nil, nil }
func (a *astCodeIndex) GetSymbolDefinition(string) (*search.SymbolLocation, error) {
	return nil, nil
}
func (a *astCodeIndex) GetSymbolReferences(string) ([]search.SymbolLocation, error) {
	return nil, nil
}
func (a *astCodeIndex) GetFileDependencies(string) []string                         { return nil }
func (a *astCodeIndex) GetDependents(string) []string                               { return nil }
func (a *astCodeIndex) GetChunksForFile(string) []*search.CodeChunk                 { return nil }
func (a *astCodeIndex) GetChunkByID(string) (*search.CodeChunk, bool)               { return nil, false }
func (a *astCodeIndex) FindChunksByName(string) []*search.CodeChunk                 { return nil }
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
