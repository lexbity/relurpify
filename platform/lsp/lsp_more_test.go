package lsp

import (
	"context"
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/stretchr/testify/require"
)

type fakeLSPClient struct {
	definitionCalls int
	definition      DefinitionResult
}

func (f *fakeLSPClient) GetDefinition(context.Context, DefinitionRequest) (DefinitionResult, error) {
	f.definitionCalls++
	return f.definition, nil
}
func (f *fakeLSPClient) GetReferences(context.Context, ReferencesRequest) ([]Location, error) {
	return nil, nil
}
func (f *fakeLSPClient) GetHover(context.Context, HoverRequest) (HoverResult, error) {
	return HoverResult{}, nil
}
func (f *fakeLSPClient) GetDiagnostics(context.Context, string) ([]Diagnostic, error) {
	return nil, nil
}
func (f *fakeLSPClient) SearchSymbols(context.Context, string) ([]SymbolInformation, error) {
	return nil, nil
}
func (f *fakeLSPClient) GetDocumentSymbols(context.Context, string) ([]SymbolInformation, error) {
	return nil, nil
}
func (f *fakeLSPClient) Format(context.Context, FormatRequest) (string, error) { return "", nil }

func TestLSPHelpersAndProxy(t *testing.T) {
	require.Equal(t, "file:///tmp/example.go", pathToURI("/tmp/example.go"))
	require.Equal(t, "/tmp/example.go", uriToPath("file:///tmp/example.go"))
	require.Equal(t, 7, toInt(int64(7)))
	require.Equal(t, 0, toInt("abc"))

	proxy := NewProxy(0)
	client := &fakeLSPClient{definition: DefinitionResult{Snippet: "snippet", Signature: "func Hello()"}}
	proxy.Register("go", client)
	got, err := proxy.clientForFile("main.go")
	require.NoError(t, err)
	require.NotNil(t, got)

	defTool := &DefinitionTool{Proxy: proxy}
	res, err := defTool.Execute(context.Background(), core.NewContext(), map[string]interface{}{
		"file":      "main.go",
		"symbol":    "Hello",
		"line":      1,
		"character": 0,
	})
	require.NoError(t, err)
	require.True(t, res.Success)
	require.Equal(t, 1, client.definitionCalls)

	cacheCalls := 0
	val, err := proxy.cached("key", func() (interface{}, error) {
		cacheCalls++
		return "value", nil
	})
	require.NoError(t, err)
	require.Equal(t, "value", val)
	val, err = proxy.cached("key", func() (interface{}, error) {
		cacheCalls++
		return "other", nil
	})
	require.NoError(t, err)
	require.Equal(t, "value", val)
	require.Equal(t, 1, cacheCalls)
}
