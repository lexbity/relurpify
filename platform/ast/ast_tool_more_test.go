package ast

import (
	"context"
	"errors"
	"testing"

	frameworkast "github.com/lexcodex/relurpify/framework/ast"
	"github.com/lexcodex/relurpify/framework/core"
	platformlsp "github.com/lexcodex/relurpify/platform/lsp"
	"github.com/stretchr/testify/require"
)

type fakeSymbolTool struct {
	result *core.ToolResult
	err    error
}

func (f fakeSymbolTool) Name() string                     { return "lsp_document_symbols" }
func (f fakeSymbolTool) Description() string              { return "fake" }
func (f fakeSymbolTool) Category() string                 { return "lsp" }
func (f fakeSymbolTool) Parameters() []core.ToolParameter { return nil }
func (f fakeSymbolTool) Execute(context.Context, *core.Context, map[string]interface{}) (*core.ToolResult, error) {
	return f.result, f.err
}
func (f fakeSymbolTool) IsAvailable(context.Context, *core.Context) bool { return true }
func (f fakeSymbolTool) Permissions() core.ToolPermissions               { return core.ToolPermissions{} }
func (f fakeSymbolTool) Tags() []string                                  { return nil }

func TestDocumentSymbolProviderHelpers(t *testing.T) {
	require.Nil(t, NewDocumentSymbolToolProvider(nil))

	provider := NewDocumentSymbolToolProvider(fakeSymbolTool{
		result: &core.ToolResult{
			Success: true,
			Data: map[string]interface{}{
				"symbols": []interface{}{
					map[string]interface{}{"name": "Hello", "kind": "12", "location": "file.go:9"},
				},
			},
		},
	})
	require.NotNil(t, provider)

	symbols, err := provider.DocumentSymbols(context.Background(), "file.go")
	require.NoError(t, err)
	require.Len(t, symbols, 1)
	require.Equal(t, "Hello", symbols[0].Name)
	require.Equal(t, 10, symbols[0].StartLine)

	_, err = NewDocumentSymbolToolProvider(fakeSymbolTool{result: &core.ToolResult{Success: true, Data: map[string]interface{}{}}}).DocumentSymbols(context.Background(), "file.go")
	require.Error(t, err)
}

func TestASTToolSymbolHelpers(t *testing.T) {
	require.Equal(t, 1, extractLine("file.go"))
	require.Equal(t, 10, extractLine("file.go:9"))
	require.Equal(t, frameworkast.NodeTypeClass, mapSymbolKind("5"))
	require.Equal(t, frameworkast.NodeTypeFunction, mapSymbolKind("12"))
	require.Equal(t, frameworkast.NodeTypeSection, mapSymbolKind("unknown"))

	symbols := convertSymbolInformation([]platformlsp.SymbolInformation{
		{Name: "Hello", Kind: "12", Location: "file.go:2"},
	})
	require.Len(t, symbols, 1)
	require.Equal(t, "Hello", symbols[0].Name)
	require.Equal(t, 3, symbols[0].StartLine)

	summary := summarizeNodes([]*frameworkast.Node{
		nil,
		{Name: "Hello", Type: frameworkast.NodeTypeFunction, Signature: "func Hello()", FileID: "file", StartLine: 2, IsExported: true},
	})
	require.Len(t, summary, 1)
	require.Equal(t, "Hello", summary[0]["name"])
}

func TestASTToolMetadataAndNilManager(t *testing.T) {
	tool := NewASTTool(nil)
	require.Equal(t, "query_ast", tool.Name())
	require.Equal(t, "search", tool.Category())
	require.False(t, tool.IsAvailable(context.Background(), core.NewContext()))

	_, err := tool.Execute(context.Background(), core.NewContext(), map[string]interface{}{"action": "list_symbols"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "ast index unavailable")

	tool = NewASTTool(frameworkast.NewIndexManager(nil, frameworkast.IndexConfig{}))
	_, err = tool.Execute(context.Background(), core.NewContext(), map[string]interface{}{"action": "unknown"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown action")

	_, err = NewDocumentSymbolToolProvider(fakeSymbolTool{err: errors.New("boom")}).DocumentSymbols(context.Background(), "file.go")
	require.Error(t, err)
}
