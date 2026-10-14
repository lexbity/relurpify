package lsp

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/capability/ports"
)

// fakeLSPClient records calls and returns canned results so proxy tools can
// be exercised without a language server process.
type fakeLSPClient struct {
	mu sync.Mutex

	definitionCalls int
	definitionErr   error

	referencesCalls int
	referencesErr   error

	hoverCalls int
	hoverErr   error

	diagnosticsCalls int
	diagnosticsErr   error

	symbolsCalls int
	symbolsErr   error

	docSymbolsCalls int
	docSymbolsErr   error

	formatCalls int
	formatErr   error
	formatOut   string

	lastDefinition  DefinitionRequest
	lastReferences  ReferencesRequest
	lastHover       HoverRequest
	lastDiagnostics string
	lastSymbols     string
	lastDocSymbols  string
	lastFormat      FormatRequest
}

func (c *fakeLSPClient) GetDefinition(_ context.Context, req DefinitionRequest) (DefinitionResult, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.definitionCalls++
	c.lastDefinition = req
	if c.definitionErr != nil {
		return DefinitionResult{}, c.definitionErr
	}
	return DefinitionResult{
		Location:  Location{URI: "file:///def.go", Range: [2]int64{3, 4}},
		Snippet:   "func Snip()",
		Signature: "func Snip()",
	}, nil
}

func (c *fakeLSPClient) GetReferences(_ context.Context, req ReferencesRequest) ([]Location, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.referencesCalls++
	c.lastReferences = req
	if c.referencesErr != nil {
		return nil, c.referencesErr
	}
	return []Location{{URI: "file:///use.go", Range: [2]int64{8, 8}}}, nil
}

func (c *fakeLSPClient) GetHover(_ context.Context, req HoverRequest) (HoverResult, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.hoverCalls++
	c.lastHover = req
	if c.hoverErr != nil {
		return HoverResult{}, c.hoverErr
	}
	return HoverResult{TypeInfo: "func(a int) int", Docs: "docs"}, nil
}

func (c *fakeLSPClient) GetDiagnostics(_ context.Context, file string) ([]Diagnostic, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.diagnosticsCalls++
	c.lastDiagnostics = file
	if c.diagnosticsErr != nil {
		return nil, c.diagnosticsErr
	}
	return []Diagnostic{{Severity: "1", Message: "boom", Source: "fake", Line: 2}}, nil
}

func (c *fakeLSPClient) SearchSymbols(_ context.Context, query string) ([]SymbolInformation, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.symbolsCalls++
	c.lastSymbols = query
	if c.symbolsErr != nil {
		return nil, c.symbolsErr
	}
	return []SymbolInformation{{Name: "Sym", Kind: "5", Location: "a.go:1"}}, nil
}

func (c *fakeLSPClient) GetDocumentSymbols(_ context.Context, file string) ([]SymbolInformation, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.docSymbolsCalls++
	c.lastDocSymbols = file
	if c.docSymbolsErr != nil {
		return nil, c.docSymbolsErr
	}
	return []SymbolInformation{{Name: "DocSym", Kind: "6", Location: "f.go:0"}}, nil
}

func (c *fakeLSPClient) Format(_ context.Context, req FormatRequest) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.formatCalls++
	c.lastFormat = req
	if c.formatErr != nil {
		return "", c.formatErr
	}
	return "formatted:" + req.Code, nil
}

func newProxyWithClient(t *testing.T) (*Proxy, *fakeLSPClient) {
	t.Helper()
	proxy := NewProxy(time.Minute, 64)
	client := &fakeLSPClient{}
	proxy.Register("go", client)
	return proxy, client
}

func TestDefinitionTool_RoutesAndCaches(t *testing.T) {
	proxy, client := newProxyWithClient(t)
	tool := &DefinitionTool{Proxy: proxy}

	res, err := tool.Execute(context.Background(), map[string]any{
		"file":      "main.go",
		"symbol":    "Foo",
		"line":      float64(3), // JSON numbers arrive as float64
		"character": int64(7),
	})
	require.NoError(t, err)
	require.True(t, res.Success)
	assert.Equal(t, Location{URI: "file:///def.go", Range: [2]int64{3, 4}}, res.Data["location"])
	assert.Equal(t, "func Snip()", res.Data["snippet"])
	assert.Equal(t, "func Snip()", res.Data["signature"])

	client.mu.Lock()
	require.Equal(t, "main.go", client.lastDefinition.File)
	require.Equal(t, "Foo", client.lastDefinition.Symbol)
	require.Equal(t, 3, client.lastDefinition.Position.Line)
	require.Equal(t, 7, client.lastDefinition.Position.Character)
	client.mu.Unlock()

	_, err = tool.Execute(context.Background(), map[string]any{
		"file": "main.go", "symbol": "Foo", "line": 3, "character": 7,
	})
	require.NoError(t, err)
	client.mu.Lock()
	require.Equal(t, 1, client.definitionCalls, "same position must be served from cache")
	client.mu.Unlock()
}

func TestDefinitionTool_NoClientForExtension(t *testing.T) {
	proxy, _ := newProxyWithClient(t)
	tool := &DefinitionTool{Proxy: proxy}
	_, err := tool.Execute(context.Background(), map[string]any{"file": "note.txt"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "no LSP client for extension txt")
}

func TestDefinitionTool_PropagatesClientError(t *testing.T) {
	proxy, client := newProxyWithClient(t)
	client.definitionErr = errors.New("server exploded")
	tool := &DefinitionTool{Proxy: proxy}
	_, err := tool.Execute(context.Background(), map[string]any{"file": "main.go"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "server exploded")
}

func TestReferencesTool_RoutesAndCachesPerSymbol(t *testing.T) {
	proxy, client := newProxyWithClient(t)
	tool := &ReferencesTool{Proxy: proxy}

	res, err := tool.Execute(context.Background(), map[string]any{
		"file": "main.go", "symbol": "Foo", "line": 1, "character": 2,
	})
	require.NoError(t, err)
	require.True(t, res.Success)
	locs := res.Data["locations"].([]Location)
	require.Len(t, locs, 1)
	require.Equal(t, "file:///use.go", locs[0].URI)

	client.mu.Lock()
	require.Equal(t, "Foo", client.lastReferences.Symbol)
	client.mu.Unlock()

	// Same file+symbol is cached; a different symbol is a fresh fetch.
	_, err = tool.Execute(context.Background(), map[string]any{
		"file": "main.go", "symbol": "Foo", "line": 1, "character": 2,
	})
	require.NoError(t, err)
	_, err = tool.Execute(context.Background(), map[string]any{
		"file": "main.go", "symbol": "Bar", "line": 1, "character": 2,
	})
	require.NoError(t, err)
	client.mu.Lock()
	require.Equal(t, 2, client.referencesCalls, "symbol must be part of the cache key")
	client.mu.Unlock()
}

func TestHoverTool_ReturnsTypeAndDocs(t *testing.T) {
	proxy, client := newProxyWithClient(t)
	tool := &HoverTool{Proxy: proxy}

	res, err := tool.Execute(context.Background(), map[string]any{
		"file": "main.go", "line": 4, "character": 9,
	})
	require.NoError(t, err)
	require.True(t, res.Success)
	assert.Equal(t, "func(a int) int", res.Data["type"])
	assert.Equal(t, "docs", res.Data["docs"])

	client.mu.Lock()
	require.Equal(t, 4, client.lastHover.Position.Line)
	client.mu.Unlock()
}

func TestDiagnosticsTool_ReturnsDiagnostics(t *testing.T) {
	proxy, client := newProxyWithClient(t)
	tool := &DiagnosticsTool{Proxy: proxy}

	res, err := tool.Execute(context.Background(), map[string]any{"file": "main.go"})
	require.NoError(t, err)
	require.True(t, res.Success)
	diags := res.Data["diagnostics"].([]Diagnostic)
	require.Len(t, diags, 1)
	require.Equal(t, "boom", diags[0].Message)

	client.mu.Lock()
	require.Equal(t, 1, client.diagnosticsCalls)
	client.mu.Unlock()

	// Cached second call.
	_, err = tool.Execute(context.Background(), map[string]any{"file": "main.go"})
	require.NoError(t, err)
	client.mu.Lock()
	require.Equal(t, 1, client.diagnosticsCalls)
	client.mu.Unlock()
}

func TestSearchSymbolsTool_CombinesAllRegisteredClients(t *testing.T) {
	proxy := NewProxy(time.Minute, 16)
	first := &fakeLSPClient{}
	second := &fakeLSPClient{}
	proxy.Register("go", first)
	proxy.Register("py", second)

	tool := &SearchSymbolsTool{Proxy: proxy}
	res, err := tool.Execute(context.Background(), map[string]any{"query": "Sym"})
	require.NoError(t, err)
	require.True(t, res.Success)
	symbols := res.Data["symbols"].([]SymbolInformation)
	require.Len(t, symbols, 2, "symbols from every registered client are combined")

	first.mu.Lock()
	require.Equal(t, "Sym", first.lastSymbols)
	first.mu.Unlock()
}

func TestSearchSymbolsTool_PropagatesClientError(t *testing.T) {
	proxy, client := newProxyWithClient(t)
	client.symbolsErr = errors.New("symbol lookup failed")
	tool := &SearchSymbolsTool{Proxy: proxy}
	_, err := tool.Execute(context.Background(), map[string]any{"query": "x"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "symbol lookup failed")
}

func TestDocumentSymbolsTool_RoutesAndCaches(t *testing.T) {
	proxy, client := newProxyWithClient(t)
	tool := &DocumentSymbolsTool{Proxy: proxy}

	res, err := tool.Execute(context.Background(), map[string]any{"file": "main.go"})
	require.NoError(t, err)
	require.True(t, res.Success)
	symbols := res.Data["symbols"].([]SymbolInformation)
	require.Len(t, symbols, 1)
	require.Equal(t, "DocSym", symbols[0].Name)

	_, err = tool.Execute(context.Background(), map[string]any{"file": "main.go"})
	require.NoError(t, err)
	client.mu.Lock()
	require.Equal(t, 1, client.docSymbolsCalls)
	client.mu.Unlock()
}

func TestFormatTool_SendsCodeAndReturnsFormatted(t *testing.T) {
	proxy, client := newProxyWithClient(t)
	tool := &FormatTool{Proxy: proxy}

	res, err := tool.Execute(context.Background(), map[string]any{
		"file": "main.go", "code": "x:=1",
	})
	require.NoError(t, err)
	require.True(t, res.Success)
	require.Equal(t, "formatted:x:=1", res.Data["code"])

	client.mu.Lock()
	require.Equal(t, "x:=1", client.lastFormat.Code)
	require.Equal(t, "main.go", client.lastFormat.File)
	client.mu.Unlock()

	_, err = tool.Execute(context.Background(), map[string]any{"file": "main.go", "code": "y:=2"})
	require.NoError(t, err)
	client.mu.Lock()
	require.Equal(t, 2, client.formatCalls, "format results must not be cached")
	client.mu.Unlock()
}

// TestToolsRejectFilesWithoutRegisteredClient drives the clientForFile error
// branch of every file-based tool.
func TestToolsRejectFilesWithoutRegisteredClient(t *testing.T) {
	proxy, _ := newProxyWithClient(t) // only "go" is registered

	cases := []struct {
		name string
		run  func() error
	}{
		{"definition", func() error {
			_, err := (&DefinitionTool{Proxy: proxy}).Execute(context.Background(), map[string]any{"file": "readme.md"})
			return err
		}},
		{"references", func() error {
			_, err := (&ReferencesTool{Proxy: proxy}).Execute(context.Background(), map[string]any{"file": "readme.md"})
			return err
		}},
		{"hover", func() error {
			_, err := (&HoverTool{Proxy: proxy}).Execute(context.Background(), map[string]any{"file": "readme.md"})
			return err
		}},
		{"diagnostics", func() error {
			_, err := (&DiagnosticsTool{Proxy: proxy}).Execute(context.Background(), map[string]any{"file": "readme.md"})
			return err
		}},
		{"document_symbols", func() error {
			_, err := (&DocumentSymbolsTool{Proxy: proxy}).Execute(context.Background(), map[string]any{"file": "readme.md"})
			return err
		}},
		{"format", func() error {
			_, err := (&FormatTool{Proxy: proxy}).Execute(context.Background(), map[string]any{"file": "readme.md"})
			return err
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.run()
			require.Error(t, err)
			require.Contains(t, err.Error(), "no LSP client for extension md")
		})
	}
}

func TestToolMetadataAndAvailability(t *testing.T) {
	proxy := NewProxy(time.Minute, 16)

	allTools := []ports.Tool{
		&DefinitionTool{Proxy: proxy},
		&ReferencesTool{Proxy: proxy},
		&HoverTool{Proxy: proxy},
		&DiagnosticsTool{Proxy: proxy},
		&SearchSymbolsTool{Proxy: proxy},
		&DocumentSymbolsTool{Proxy: proxy},
		&FormatTool{Proxy: proxy},
	}
	for _, tool := range allTools {
		require.NotEmpty(t, tool.Name())
		require.NotEmpty(t, tool.Description())
		require.Equal(t, "lsp", tool.Category())
		require.NotNil(t, tool.Parameters(), "%s parameters", tool.Name())
		require.NotNil(t, tool.Permissions().Permissions, "%s permissions", tool.Name())
		require.NotEmpty(t, tool.Tags(), "%s tags", tool.Name())
		require.True(t, tool.IsAvailable(context.Background()), "%s with proxy must be available", tool.Name())
	}

	withoutProxy := []ports.Tool{
		&DefinitionTool{},
		&ReferencesTool{},
		&HoverTool{},
		&DiagnosticsTool{},
		&SearchSymbolsTool{},
		&DocumentSymbolsTool{},
		&FormatTool{},
	}
	for _, tool := range withoutProxy {
		require.False(t, tool.IsAvailable(context.Background()), "nil proxy must be unavailable")
	}

	def := &DefinitionTool{Proxy: proxy}
	require.Len(t, def.Parameters(), 4)
	fmtTool := &FormatTool{Proxy: proxy}
	require.Len(t, fmtTool.Parameters(), 2)
	require.Equal(t, []string{ports.TagDestructive}, fmtTool.Tags())
	require.Equal(t, []string{ports.TagReadOnly}, def.Tags())
}

func TestToIntCoercesNumericTypes(t *testing.T) {
	require.Equal(t, 1, toInt(1))
	require.Equal(t, 2, toInt(int32(2)))
	require.Equal(t, 3, toInt(int64(3)))
	require.Equal(t, 4, toInt(float64(4.9)))
	require.Equal(t, 5, toInt(float32(5.9)))
	require.Equal(t, 0, toInt("6"))
	require.Equal(t, 0, toInt(nil))
}
